"""4 pool consumer tren RabbitMQ 3.13 that (AC-1, AC-4, AC-5, AC-6, AC-8)."""

from __future__ import annotations

import asyncio
import json
import time
import uuid
from datetime import UTC, datetime

import aio_pika
import pytest
from prometheus_client import REGISTRY

from app.consumers import chat
from app.gemini.client import GeminiClient
from app.gemini.limiter import GlobalRateLimiter
from app.mq.protocol import ATTEMPT_HEADER, ERROR_HEADER, PermanentError
from app.mq.runtime import ConsumerRuntime, Delivery, PoolSpec
from app.mq.topology import Queue, Topology, declare
from app.pipeline import degradation
from app.pipeline.faq import FaqMatcher
from app.pipeline.results import RedisResultSink, result_key
from app.pipeline.semantic_cache import SemanticCache
from app.pipeline.singleflight import SingleFlight

from .conftest import TOPOLOGY

pytestmark = pytest.mark.integration


async def _publish(conn, key: str, payload: dict, *, headers: dict | None = None) -> None:
    async with conn.channel() as ch:
        ex = await ch.get_exchange("ai.direct")
        await ex.publish(
            aio_pika.Message(
                json.dumps(payload).encode(),
                content_type="application/json",
                delivery_mode=aio_pika.DeliveryMode.PERSISTENT,
                timestamp=datetime.now(UTC),
                message_id=payload.get("job_id"),
                headers=headers or {},
            ),
            routing_key=key,
            mandatory=True,
        )


async def _count(conn, queue: str) -> int:
    async with conn.channel() as ch:
        q = await ch.declare_queue(queue, passive=True)
        return q.declaration_result.message_count


async def _eventually(cond, timeout: float, interval: float = 0.2):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if (value := await cond()) :
            return value
        await asyncio.sleep(interval)
    raise AssertionError(f"dieu kien khong dat sau {timeout}s")


async def _noop(d: Delivery) -> None:
    return None


# ---------------------------------------------------------------- AC-4 topology


async def test_KhaiBaoTopology_HaiLanVanOK(connection) -> None:
    async with connection.channel() as ch:
        await declare(ch, TOPOLOGY)
        await declare(ch, TOPOLOGY)
    # Moi queue ton tai va dung binding: publish theo key khong bi tra ve.
    for key in ("chat", "moderation", "anomaly", "report"):
        await _publish(connection, key, {"job_id": f"t-{key}"})
        assert await _count(connection, f"ai.{key}.q") == 1


async def test_KhaiBaoTopology_LechThamSoThiLoi(connection) -> None:
    lech = Topology(
        exchanges=(),
        queues=(Queue("ai.chat.q", True, {"x-queue-type": "quorum", "x-message-ttl": 1000}),),
        bindings=(),
    )
    async with connection.channel() as ch:
        with pytest.raises(RuntimeError, match="ai.chat.q"):
            await declare(ch, lech)


# ---------------------------------------------------------------- AC-5 retry / DLQ


async def test_LoiTam_TuPhucHoiQuaHangTre(connection) -> None:
    attempts: list[int] = []

    async def flaky(d: Delivery) -> None:
        attempts.append(d.attempt)
        if d.attempt < 3:
            raise RuntimeError("gemini tam thoi")

    rt = ConsumerRuntime(connection, [PoolSpec("chat", 2, flaky)], grace_seconds=5)
    await rt.start()
    try:
        await _publish(connection, "chat", {"job_id": "j-flaky"})
        # Hai lan qua ai.chat.retry (5s moi lan) roi quay lai ai.chat.q.
        await _eventually(lambda: _async(len(attempts) >= 3), timeout=25)
    finally:
        await rt.stop()

    assert attempts == [1, 2, 3]
    assert await _count(connection, "ai.dlq") == 0
    assert await _count(connection, "ai.chat.q") == 0


async def test_HetLuot_VaoDLQGiuBodyVaLyDo(connection) -> None:
    async def always_fail(d: Delivery) -> None:
        raise RuntimeError("van hong")

    rt = ConsumerRuntime(connection, [PoolSpec("chat", 1, always_fail)], grace_seconds=5)
    await rt.start()
    try:
        await _publish(connection, "chat", {"job_id": "j-dlq"}, headers={"traceparent": "00-" + "a" * 32 + "-" + "b" * 16 + "-01"})
        await _eventually(lambda: _count_positive(connection, "ai.dlq"), timeout=25)
    finally:
        await rt.stop()

    msg = await _get(connection, "ai.dlq")
    assert json.loads(msg.body) == {"job_id": "j-dlq"}
    assert msg.headers[ATTEMPT_HEADER] == 3
    assert "RuntimeError" in msg.headers[ERROR_HEADER]
    assert msg.headers["traceparent"].startswith("00-aaaa")  # trace di theo message vao DLQ
    assert msg.message_id == "j-dlq"


async def test_LoiVinhVien_VaoDLQNgay(connection) -> None:
    async def bad(d: Delivery) -> None:
        raise PermanentError("json hong")

    rt = ConsumerRuntime(connection, [PoolSpec("moderation", 1, bad)], grace_seconds=5)
    await rt.start()
    try:
        await _publish(connection, "moderation", {"job_id": "j-bad"})
        await _eventually(lambda: _count_positive(connection, "ai.dlq"), timeout=10)
    finally:
        await rt.stop()

    msg = await _get(connection, "ai.dlq")
    assert msg.headers[ATTEMPT_HEADER] == 1
    assert await _count(connection, "ai.moderation.retry") == 0


async def test_HeaderAttemptKieuGo_DuocDocDung(connection) -> None:
    # Go ghi x-ef-attempt int32; message da o lan 3 tu ben Go thi Python khong duoc retry them.
    seen: list[int] = []

    async def fail(d: Delivery) -> None:
        seen.append(d.attempt)
        raise RuntimeError("x")

    rt = ConsumerRuntime(connection, [PoolSpec("chat", 1, fail)], grace_seconds=5)
    await rt.start()
    try:
        await _publish(connection, "chat", {"job_id": "j-go"}, headers={ATTEMPT_HEADER: 3})
        await _eventually(lambda: _count_positive(connection, "ai.dlq"), timeout=10)
    finally:
        await rt.stop()
    assert seen == [3]


# ---------------------------------------------------------------- AC-1 tach biet


@pytest.mark.parametrize("report_kind", ["async", "thread"])
async def test_ReportNang_KhongLamChatTreQua5Giay(connection, report_kind) -> None:
    """AC Jira: job report nang khong lam tang lag cua ai.chat.q qua 5 giay.

    Report chiem pool cua no 4 giay moi job (ca kieu await lan kieu chan dong bo qua
    to_thread). Trong luc do 150 cau chat duoc publish deu trong 3 giay.
    """
    chat_lags: list[float] = []
    report_started = asyncio.Event()

    async def report(d: Delivery) -> None:
        report_started.set()
        if report_kind == "async":
            await asyncio.sleep(4)
        else:
            await asyncio.to_thread(time.sleep, 4)

    async def chat_handler(d: Delivery) -> None:
        chat_lags.append(time.time() - d.payload["enqueued_at"])
        await asyncio.sleep(0.05)

    rt = ConsumerRuntime(
        connection,
        [PoolSpec("chat", 20, chat_handler), PoolSpec("report", 1, report)],
        grace_seconds=10,
    )
    for i in range(10):
        await _publish(connection, "report", {"job_id": f"r-{i}", "enqueued_at": time.time()})
    await rt.start()
    try:
        await asyncio.wait_for(report_started.wait(), timeout=10)
        for i in range(150):
            await _publish(connection, "chat", {"job_id": f"c-{i}", "enqueued_at": time.time()})
            await asyncio.sleep(0.02)
        await _eventually(lambda: _async(len(chat_lags) >= 150), timeout=20)
    finally:
        await rt.stop()

    assert max(chat_lags) < 5.0, f"lag chat lon nhat {max(chat_lags):.2f}s"
    # Report van con hang: dung la no dang bi nghen trong pool cua chinh no.
    assert await _count(connection, "ai.report.q") >= 7


async def test_PoolChaySongSongDungSoConsumer(connection) -> None:
    """AC-3: prefetch = so consumer va dung so handler chay cung luc."""
    current = 0
    peak = 0

    async def slow(d: Delivery) -> None:
        nonlocal current, peak
        current += 1
        peak = max(peak, current)
        await asyncio.sleep(0.5)
        current -= 1

    for i in range(16):
        await _publish(connection, "chat", {"job_id": f"p-{i}"})
    rt = ConsumerRuntime(connection, [PoolSpec("chat", 8, slow)], grace_seconds=5)
    t0 = time.monotonic()
    await rt.start()
    try:
        await _eventually(lambda: _done_count("ai.chat.q", 16), timeout=10, interval=0.05)
    finally:
        await rt.stop()

    assert peak == 8
    assert time.monotonic() - t0 < 2.5  # 16 viec x 0.5s / 8 luong ~ 1s; noi tiep se la 8s


async def test_LagMetric_BonQueueVaVeKhongKhiTrong(connection) -> None:
    rt = ConsumerRuntime(
        connection,
        [PoolSpec(k, 1, _noop) for k in ("chat", "moderation", "anomaly", "report")],
        grace_seconds=5,
    )
    await rt.start()
    try:
        for k in ("chat", "moderation", "anomaly", "report"):
            await _publish(connection, k, {"job_id": f"l-{k}"})
        await _eventually(lambda: _all_drained(connection), timeout=10)
        await asyncio.sleep(0.3)
        await rt.sample_once()
    finally:
        await rt.stop()

    for k in ("chat", "moderation", "anomaly", "report"):
        labels = {"queue": f"ai.{k}.q"}
        assert REGISTRY.get_sample_value("ai_queue_lag_seconds", labels) == 0
        assert REGISTRY.get_sample_value("ai_queue_depth", labels) == 0
        assert (REGISTRY.get_sample_value("ai_jobs_total", {**labels, "outcome": "ack"}) or 0) >= 1


# ---------------------------------------------------------------- AC-8 tat em


async def test_TatEm_ViecDangChayXongPhanConLaiQuayLaiHang(connection) -> None:
    done: list[str] = []
    started = asyncio.Event()

    async def slow(d: Delivery) -> None:
        started.set()
        await asyncio.sleep(1.5)
        done.append(d.payload["job_id"])

    for i in range(3):
        await _publish(connection, "moderation", {"job_id": f"s-{i}"})
    rt = ConsumerRuntime(connection, [PoolSpec("moderation", 1, slow)], grace_seconds=5)
    await rt.start()
    await asyncio.wait_for(started.wait(), timeout=10)
    await rt.stop()

    assert done == ["s-0"]
    assert await _eventually(lambda: _count_eq(connection, "ai.moderation.q", 2), timeout=10)


async def test_TatEm_QuaHanThiHuyVaMessageQuayLai(connection) -> None:
    started = asyncio.Event()

    async def stuck(d: Delivery) -> None:
        started.set()
        await asyncio.sleep(60)

    await _publish(connection, "chat", {"job_id": "stuck"})
    rt = ConsumerRuntime(connection, [PoolSpec("chat", 1, stuck)], grace_seconds=0.5)
    await rt.start()
    await asyncio.wait_for(started.wait(), timeout=10)
    t0 = time.monotonic()
    await rt.stop()

    assert time.monotonic() - t0 < 5
    assert await _eventually(lambda: _count_eq(connection, "ai.chat.q", 1), timeout=10)


# ---------------------------------------------------------------- AC-6 dau-cuoi chat


async def test_Chat_DauCuoi_KetQuaVaoRedis(connection, redis) -> None:
    gemini = GeminiClient(mock=True)
    consumer = chat.ChatConsumer(
        limiter=GlobalRateLimiter(redis),
        cache=SemanticCache(redis, gemini),
        faq=FaqMatcher(),
        flight=SingleFlight(redis),
        gemini=gemini,
        tools=None,  # cau hoi FAQ tinh, khong can so lieu song
        signals=lambda: degradation.Signals(0.0, 0.0, False),
        publish_result=RedisResultSink(redis).publish,
    )
    rt = ConsumerRuntime(connection, [PoolSpec("chat", 2, chat.make_handler(consumer))], grace_seconds=5)
    job_id = str(uuid.uuid4())
    pubsub = redis.pubsub()
    await pubsub.subscribe(f"ai:job:{{{job_id}}}")

    await rt.start()
    try:
        await _publish(connection, "chat", {
            "job_id": job_id, "event_id": "e-1", "identity_id": "i-1",
            "question": "Tôi tắt trình duyệt có mất chỗ không?", "queue_token": "t",
            "enqueued_at": time.time(),
        })
        raw = await _eventually(lambda: redis.get(result_key(job_id)), timeout=10)
    finally:
        await rt.stop()

    result = json.loads(raw)
    assert result["source"] == "faq"
    assert "khong mat cho" in result["answer"].lower()
    assert 0 < await redis.ttl(result_key(job_id)) <= 300
    msg = await _eventually(lambda: pubsub.get_message(ignore_subscribe_messages=True, timeout=0.1), timeout=5)
    assert json.loads(msg["data"])["job_id"] == job_id
    await pubsub.aclose()


# ---------------------------------------------------------------- tien ich


async def _async(value):
    return value


_baseline: dict[str, float] = {}


async def _done_count(queue: str, n: int) -> bool:
    # ai_jobs_total la counter toan process: so voi gia tri luc test bat dau.
    value = REGISTRY.get_sample_value("ai_jobs_total", {"queue": queue, "outcome": "ack"}) or 0.0
    start = _baseline.setdefault(queue, value)
    return value - start >= n


async def _count_positive(conn, queue: str) -> bool:
    return await _count(conn, queue) > 0


async def _count_eq(conn, queue: str, n: int) -> bool:
    return await _count(conn, queue) == n


async def _all_drained(conn) -> bool:
    for k in ("chat", "moderation", "anomaly", "report"):
        if await _count(conn, f"ai.{k}.q") != 0:
            return False
    return True


async def _get(conn, queue: str):
    async with conn.channel() as ch:
        q = await ch.declare_queue(queue, passive=True)
        msg = await q.get(no_ack=True, fail=True)
        return msg
