"""ai-worker: FastAPI (health + metric + dieu khien Ops) va 4 consumer RabbitMQ.

Tai sao gop HTTP va consumer vao mot process: Ops can mot cho de bat/tat che do
suy bien giua luc su co, va metric can duoc scrape tu chinh process dang chay
consumer. Tach ra se phai dong bo trang thai giua hai process — phuc tap hon ma
khong duoc gi.
"""

from __future__ import annotations

import asyncio
import contextlib
import logging
from typing import Any

from fastapi import FastAPI, HTTPException
from prometheus_client import CONTENT_TYPE_LATEST, Counter, Gauge, generate_latest
from redis.asyncio import Redis
from starlette.responses import Response

from app.config import settings
from app.gemini.limiter import GlobalRateLimiter, LimiterConfig
from app.pipeline import degradation

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")
log = logging.getLogger("ai-worker")

# Metric nghiep vu — day moi la nhung con so Ops nhin trong war room.
QUEUE_LAG = Gauge("ai_queue_lag_seconds", "Do tre hang doi AI", ["queue"])
CACHE_HIT = Gauge("ai_semantic_cache_hit_ratio", "Ti le hit cache ngu nghia")
RATE_LIMITED = Counter("ai_gemini_429_total", "So lan bi Gemini tu choi vi rate-limit")
PERMIT_DENIED = Counter("ai_permit_denied_total", "So lan limiter tu choi cap phep")
MODE = Gauge("ai_degradation_mode", "Nac suy bien (0=NORMAL..3=OFF)")

app = FastAPI(title="EventFlow AI Worker", version="0.1.0")

_state: dict[str, Any] = {
    "redis": None,
    "limiter": None,
    "manual_override": None,
    "consumers": [],
}


@app.on_event("startup")
async def startup() -> None:
    redis = Redis.from_url(settings.redis_url)
    await redis.ping()

    _state["redis"] = redis
    _state["limiter"] = GlobalRateLimiter(
        redis, LimiterConfig(rpm=settings.gemini_rpm, tpm=settings.gemini_tpm)
    )

    # TODO(EVF-80): khoi dong 4 consumer voi so luong theo settings.*_consumers.
    # Moi hang doi mot pool rieng — day la ly do job bao cao nang khong the lam
    # nghen chat cua khach dang cho (BR-A3).
    log.info("ai-worker san sang (gemini_mock=%s)", settings.gemini_mock)


@app.on_event("shutdown")
async def shutdown() -> None:
    for task in _state["consumers"]:
        task.cancel()
        with contextlib.suppress(asyncio.CancelledError):
            await task
    if _state["redis"] is not None:
        await _state["redis"].aclose()


@app.get("/healthz")
async def healthz() -> dict[str, str]:
    return {"status": "ok"}


@app.get("/readyz")
async def readyz() -> dict[str, str]:
    redis = _state["redis"]
    if redis is None:
        raise HTTPException(status_code=503, detail="chua khoi dong xong")
    try:
        await redis.ping()
    except Exception as exc:  # noqa: BLE001
        raise HTTPException(status_code=503, detail="redis khong san sang") from exc
    return {"status": "ready"}


@app.get("/metrics")
async def metrics() -> Response:
    return Response(generate_latest(), media_type=CONTENT_TYPE_LATEST)


@app.get("/admin/mode")
async def get_mode() -> dict[str, Any]:
    """Nac suy bien hien tai va cac tin hieu dan toi no."""
    signals = await _current_signals()
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


async def _current_signals() -> degradation.Signals:
    # TODO(EVF-87): lay lag that tu RabbitMQ management API va ngan sach tu Redis.
    return degradation.Signals(
        chat_queue_lag_seconds=0.0,
        budget_used_ratio=0.0,
        gemini_circuit_open=False,
        manual_override=_state["manual_override"],
    )
