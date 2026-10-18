"""ai-worker: FastAPI (health + metric + dieu khien Ops) va 4 consumer RabbitMQ.

Tai sao gop HTTP va consumer vao mot process: Ops can mot cho de bat/tat che do
suy bien giua luc su co, va metric can duoc scrape tu chinh process dang chay
consumer. Tach ra se phai dong bo trang thai giua hai process — phuc tap hon ma
khong duoc gi.

Thu tu khoi dong la co y: Redis -> khai bao topology -> mo 4 pool. Buoc nao hong
thi process chet ngay (fail fast) thay vi chay "nua song nua chet" toi gio mo ban.
"""

from __future__ import annotations

import contextlib
import logging
from collections.abc import AsyncIterator
from typing import Any

import aio_pika
from fastapi import FastAPI, HTTPException
from prometheus_client import CONTENT_TYPE_LATEST, generate_latest
from redis.asyncio import Redis
from starlette.responses import Response

from app import logx
from app.config import Settings, settings
from app.consumers import chat, moderation, unsupported
from app.gemini.client import GeminiClient
from app.gemini.limiter import GlobalRateLimiter, LimiterConfig
from app.mq.runtime import ConsumerRuntime, Handler, PoolSpec
from app.mq.topology import declare, load_ai_topology
from app.pipeline import degradation
from app.pipeline.faq import FaqMatcher
from app.pipeline.results import RedisResultSink
from app.pipeline.semantic_cache import SemanticCache
from app.pipeline.singleflight import SingleFlight
from app.pipeline.tools import SystemTools

log = logging.getLogger("ai-worker")

_state: dict[str, Any] = {
    "redis": None,
    "limiter": None,
    "manual_override": None,
    "connection": None,
    "runtime": None,
}


def build_specs(s: Settings, *, chat: Handler, moderation: Handler) -> list[PoolSpec]:
    """Moi hang doi mot pool rieng (BR-A3). Pool co concurrency 0 bi runtime bo qua."""
    return [
        PoolSpec("chat", s.chat_consumers, chat),
        PoolSpec("moderation", s.moderation_consumers, moderation),
        PoolSpec("anomaly", s.anomaly_consumers, unsupported.make_handler("anomaly", "EV-167")),
        PoolSpec("report", s.report_consumers, unsupported.make_handler("report", "EV-168")),
    ]


@contextlib.asynccontextmanager
async def lifespan(_: FastAPI) -> AsyncIterator[None]:
    logx.configure()
    async with contextlib.AsyncExitStack() as stack:
        redis = Redis.from_url(settings.redis_url)
        stack.push_async_callback(redis.aclose)
        await redis.ping()
        limiter = GlobalRateLimiter(redis, LimiterConfig(rpm=settings.gemini_rpm, tpm=settings.gemini_tpm))
        _state.update(redis=redis, limiter=limiter)

        connection = await aio_pika.connect_robust(settings.amqp_url)
        stack.push_async_callback(connection.close)
        async with connection.channel() as ch:
            await declare(ch, load_ai_topology(settings.ai_topology_file))
        _state["connection"] = connection

        gemini = GeminiClient(
            api_key=settings.gemini_api_key,
            model=settings.gemini_model_fast,
            mock=settings.gemini_mock,
            timeout_seconds=settings.gemini_timeout_seconds,
            embed_model=settings.gemini_embed_model,
        )
        tools = SystemTools(settings.internal_api_base)
        mod_consumer = moderation.ModerationConsumer(gemini, settings.internal_api_base, limiter=limiter)
        stack.push_async_callback(mod_consumer.aclose)
        chat_consumer = chat.ChatConsumer(
            limiter=limiter,
            cache=SemanticCache(redis, gemini),
            faq=FaqMatcher(),
            flight=SingleFlight(redis),
            gemini=gemini,
            tools=tools,
            signals=_signals_now,
            publish_result=RedisResultSink(redis).publish,
        )

        runtime = ConsumerRuntime(
            connection,
            build_specs(
                settings,
                chat=chat.make_handler(chat_consumer),
                moderation=moderation.make_handler(mod_consumer),
            ),
            grace_seconds=settings.shutdown_grace_seconds,
        )
        # Dang ky stop TRUOC start: pool thu hai mo loi thi worker cua pool dau van duoc don.
        stack.push_async_callback(runtime.stop)
        await runtime.start()
        _state["runtime"] = runtime
        log.info("ai-worker san sang", extra={"gemini_mock": settings.gemini_mock, "pools": runtime.keys})

        try:
            yield
        finally:
            _state.update(runtime=None, connection=None, redis=None, limiter=None)
            log.info("ai-worker dang tat")


app = FastAPI(title="EventFlow AI Worker", version="0.1.0", lifespan=lifespan)


@app.get("/healthz")
async def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/readyz")
async def readyz() -> dict[str, str]:
    redis, connection = _state["redis"], _state["connection"]
    if redis is None or connection is None or _state["runtime"] is None:
        raise HTTPException(status_code=503, detail="chua khoi dong xong")
    if connection.is_closed:
        raise HTTPException(status_code=503, detail="mat ket noi rabbitmq")
    try:
        await redis.ping()
    except Exception as exc:
        raise HTTPException(status_code=503, detail="redis khong san sang") from exc
    return {"status": "ready"}


@app.get("/metrics")
async def metrics() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


@app.get("/admin/mode")
async def get_mode() -> dict[str, Any]:
    """Nac suy bien hien tai va cac tin hieu dan toi no."""
    signals = _signals_now()
    mode = degradation.decide(signals)
    return {
        "mode": str(mode),
        "manual_override": str(_state["manual_override"]) if _state["manual_override"] else None,
        "signals": {
            "chat_queue_lag_seconds": signals.chat_queue_lag_seconds,
            "budget_used_ratio": signals.budget_used_ratio,
            "gemini_circuit_open": signals.gemini_circuit_open,
        },
        "policy": degradation.policy_for(mode).__dict__,
    }


@app.post("/admin/mode/{mode}")
async def set_mode(mode: str) -> dict[str, str]:
    """Ops ghi de nac suy bien giua luc su co.

    Ghi de thang moi tin hieu tu dong: nguoi truc thuong biet nhung dieu metric
    chua kip phan anh. Dat mode = AUTO de tra lai cho he thong tu quyet.
    """
    if mode.upper() == "AUTO":
        _state["manual_override"] = None
        log.warning("Ops tra che do suy bien ve TU DONG")
        return {"manual_override": "AUTO"}

    try:
        chosen = degradation.Mode(mode.upper())
    except ValueError as exc:
        raise HTTPException(
            status_code=400,
            detail=f"nac khong hop le; chon mot trong {[m.value for m in degradation.Mode]} hoac AUTO",
        ) from exc

    _state["manual_override"] = chosen
    log.warning("Ops ghi de nac suy bien thanh %s", chosen)
    return {"manual_override": str(chosen)}


def _signals_now() -> degradation.Signals:
    # TODO(EVF-87): lay lag that (ai_queue_lag_seconds cua ai.chat.q) va ngan sach tu Redis.
    return degradation.Signals(
        chat_queue_lag_seconds=0.0,
        budget_used_ratio=0.0,
        gemini_circuit_open=False,
        manual_override=_state["manual_override"],
    )
