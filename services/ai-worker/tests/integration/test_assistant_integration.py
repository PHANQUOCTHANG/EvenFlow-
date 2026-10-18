"""API tro ly tren Redis 7 + RabbitMQ 3.13 that (EV-171)."""

from __future__ import annotations

import asyncio
import json
import statistics
import time

import httpx
import pytest
from fastapi import FastAPI

from app.api.assistant import AssistantDeps, router
from app.api.quota import AssistantQuota
from app.consumers import chat
from app.gemini.client import GeminiClient
from app.gemini.limiter import GlobalRateLimiter
from app.mq.publisher import JobPublisher
from app.mq.runtime import ConsumerRuntime, PoolSpec
from app.pipeline import degradation
from app.pipeline.faq import FaqMatcher
from app.pipeline.results import RedisResultSink
from app.pipeline.semantic_cache import SemanticCache
from app.pipeline.singleflight import SingleFlight

pytestmark = pytest.mark.integration

EVENT = "evt-api"
FAQ_QUESTION = "Tôi tắt trình duyệt có mất chỗ không?"


def _app(redis, publisher, *, stream_timeout_s: float = 10.0) -> FastAPI:
    app = FastAPI()
    app.include_router(router)
    app.state.assistant = AssistantDeps(
        quota=AssistantQuota(redis), publish=publisher.publish, redis=redis,
        mode=lambda: degradation.Mode.NORMAL, stream_timeout_s=stream_timeout_s, stream_ping_s=0.5,
    )
    return app


def _chat_runtime(connection, redis) -> ConsumerRuntime:
    gemini = GeminiClient(mock=True)
    consumer = chat.ChatConsumer(
        limiter=GlobalRateLimiter(redis), cache=SemanticCache(redis, gemini), faq=FaqMatcher(),
        flight=SingleFlight(redis), gemini=gemini, tools=None,
        signals=lambda: degradation.Signals(0.0, 0.0, False),
        publish_result=RedisResultSink(redis).publish,
    )
    return ConsumerRuntime(connection, [PoolSpec("chat", 4, chat.make_handler(consumer))], grace_seconds=5)


def _client(app: FastAPI) -> httpx.AsyncClient:
    return httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://ai")


def _sse_events(text: str) -> list[tuple[str, dict]]:
    out = []
    for block in text.split("\n\n"):
        lines = dict(line.split(": ", 1) for line in block.splitlines() if ": " in line and not line.startswith(":"))
        if "event" in lines:
            out.append((lines["event"], json.loads(lines["data"])))
    return out


# ---------------------------------------------------------------- quota (AC-2, AC-3)


async def test_Quota_10QuaCau11BiChan(redis) -> None:
    q = AssistantQuota(redis)
    now = int(time.time() * 1000)
    results = [await q.take(EVENT, "khach-a", f"j{i}", now + i) for i in range(11)]
    assert [r.allowed for r in results] == [True] * 10 + [False]
    assert results[9].used == 10 and results[10].used == 10
    # Khoa phai tu het han: khong co TTL thi moi khach de lai mot khoa Redis vinh vien.
    from app.api.quota import QUOTA_WINDOW_MS, quota_key

    assert 0 < await redis.pttl(quota_key(EVENT, "khach-a")) <= QUOTA_WINDOW_MS
    # Khach khac va su kien khac khong bi anh huong.
    assert (await q.take(EVENT, "khach-b", "x", now)).allowed
    assert (await q.take("evt-khac", "khach-a", "y", now)).allowed


async def test_Quota_CuaSoTruot(redis) -> None:
    q = AssistantQuota(redis, limit=2, window_ms=1000)
    t0 = int(time.time() * 1000)
    assert (await q.take(EVENT, "k", "a", t0)).allowed
    assert (await q.take(EVENT, "k", "b", t0 + 500)).allowed
    denied = await q.take(EVENT, "k", "c", t0 + 600)
    assert not denied.allowed
    assert denied.reset_at_ms == t0 + 1000  # cau cu nhat roi khoi cua so
    assert (await q.take(EVENT, "k", "d", t0 + 1001)).allowed  # "a" da ra khoi cua so


async def test_Quota_50YeuCauSongSong_Chi10Qua(redis) -> None:
    q = AssistantQuota(redis)
    now = int(time.time() * 1000)
    results = await asyncio.gather(*(q.take(EVENT, "khach-dong-thoi", f"j{i}", now) for i in range(50)))
    assert sum(r.allowed for r in results) == 10


# ---------------------------------------------------------------- dau-cuoi (AC-1, AC-4, AC-5)


async def test_Post202Duoi50ms_p95(connection, redis) -> None:
    publisher = JobPublisher(connection)
    try:
        async with _client(_app(redis, publisher)) as c:
            await c.post(f"/v1/events/{EVENT}/assistant/messages", json={"question": "khoi dong"},
                         headers={"X-Identity-Id": "warm"})  # mo channel publish lan dau
            times = []
            for i in range(50):
                t0 = time.perf_counter()
                r = await c.post(f"/v1/events/{EVENT}/assistant/messages", json={"question": f"cau {i}"},
                                 headers={"X-Identity-Id": f"khach-{i}"})
                times.append(time.perf_counter() - t0)
                assert r.status_code == 202
    finally:
        await publisher.close()
    p95 = statistics.quantiles(times, n=20)[18]
    print(f"p95 POST 202 = {p95 * 1000:.1f} ms, median = {statistics.median(times) * 1000:.1f} ms")
    assert p95 < 0.050, f"p95 = {p95 * 1000:.1f} ms"


async def test_DauCuoi_PostRoiGetRoiStream(connection, redis) -> None:
    publisher = JobPublisher(connection)
    rt = _chat_runtime(connection, redis)
    await rt.start()
    try:
        async with _client(_app(redis, publisher)) as c:
            r = await c.post(f"/v1/events/{EVENT}/assistant/messages", json={"question": FAQ_QUESTION},
                             headers={"X-Identity-Id": "khach-1"})
            assert r.status_code == 202
            job_id = r.json()["job_id"]

            deadline = time.monotonic() + 10
            while True:
                g = await c.get(f"/v1/assistant/messages/{job_id}", headers={"X-Identity-Id": "khach-1"})
                if g.status_code == 200:
                    break
                assert g.status_code == 202 and time.monotonic() < deadline
                await asyncio.sleep(0.1)
            assert g.json()["status"] == "done" and g.json()["source"] == "faq"

            # Ket qua da co truoc khi mo stream: van nhan duoc (SET truoc PUBLISH, EV-157).
            s = await c.get(f"/v1/assistant/messages/{job_id}/stream", headers={"X-Identity-Id": "khach-1"})
            assert s.headers["content-type"].startswith("text/event-stream")
            events = _sse_events(s.text)
            assert events[0][0] == "answer" and events[0][1]["job_id"] == job_id

            # Nguoi khac khong doc duoc.
            other = await c.get(f"/v1/assistant/messages/{job_id}", headers={"X-Identity-Id": "khach-2"})
            assert other.status_code == 404
    finally:
        await rt.stop()
        await publisher.close()


async def test_Stream_MoTruocKhiCoKetQua(connection, redis) -> None:
    publisher = JobPublisher(connection)
    rt = _chat_runtime(connection, redis)
    try:
        async with _client(_app(redis, publisher)) as c:
            r = await c.post(f"/v1/events/{EVENT}/assistant/messages", json={"question": FAQ_QUESTION},
                             headers={"X-Identity-Id": "khach-3"})
            job_id = r.json()["job_id"]
            # Stream mo khi worker CHUA chay: phai doi qua PUBLISH roi moi tra.
            stream = asyncio.create_task(
                c.get(f"/v1/assistant/messages/{job_id}/stream", headers={"X-Identity-Id": "khach-3"}))
            await asyncio.sleep(1.2)
            assert not stream.done()
            await rt.start()
            s = await asyncio.wait_for(stream, timeout=10)
    finally:
        await rt.stop()
        await publisher.close()
    events = _sse_events(s.text)
    assert [e for e, _ in events] == ["answer"]
    assert ": ping" in s.text  # heartbeat trong luc cho


async def test_Stream_QuaHan_TraLoiDuPhong(connection, redis) -> None:
    publisher = JobPublisher(connection)
    try:
        async with _client(_app(redis, publisher, stream_timeout_s=1.5)) as c:
            r = await c.post(f"/v1/events/{EVENT}/assistant/messages", json={"question": "khong ai xu ly"},
                             headers={"X-Identity-Id": "khach-4"})
            s = await c.get(f"/v1/assistant/messages/{r.json()['job_id']}/stream",
                            headers={"X-Identity-Id": "khach-4"})
    finally:
        await publisher.close()
    events = _sse_events(s.text)
    assert events[-1][0] == "timeout"
    assert events[-1][1]["answer"]  # khach van co cau tra loi
    assert "CSKH" in events[-1][1]["answer"]


async def test_Publisher_MessageDungProperty(connection, redis) -> None:
    publisher = JobPublisher(connection)
    try:
        await publisher.publish("chat", {"job_id": "p-1", "enqueued_at": 1.0}, message_id="p-1")
    finally:
        await publisher.close()
    async with connection.channel() as ch:
        q = await ch.declare_queue("ai.chat.q", passive=True)
        msg = await q.get(no_ack=True, fail=True)
    assert msg.message_id == "p-1"
    assert msg.content_type == "application/json"
    assert msg.timestamp is not None
    assert msg.delivery_mode == 2
