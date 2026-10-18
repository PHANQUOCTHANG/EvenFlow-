"""Xu ly mot message: ack / retry / DLQ / requeue va metric (AC-2, AC-5, AC-8).

Dung message gia de khong can broker; hanh vi tren broker that nam o tests/integration.
"""

from __future__ import annotations

import json
import time
from datetime import UTC, datetime, timedelta

import pytest
from prometheus_client import REGISTRY

from app.mq.protocol import ATTEMPT_HEADER, ERROR_HEADER, PermanentError
from app.mq.runtime import DLX_EXCHANGE, RETRY_EXCHANGE, Delivery, PoolSpec, process, queue_name


class FakeMessage:
    def __init__(self, body: bytes = b"{}", headers=None, timestamp=None, message_id="m-1") -> None:
        self.body = body
        self.headers = headers or {}
        self.timestamp = timestamp
        self.message_id = message_id
        self.routing_key = "chat"
        self.acked = False
        self.nacked: bool | None = None  # gia tri requeue

    async def ack(self) -> None:
        self.acked = True

    async def nack(self, requeue: bool = True) -> None:
        self.nacked = requeue


class FakeForward:
    def __init__(self, fail: bool = False) -> None:
        self.calls: list[tuple[str, str, dict]] = []
        self._fail = fail

    async def __call__(self, msg, exchange: str, routing_key: str, headers: dict) -> None:
        if self._fail:
            raise ConnectionError("broker nack")
        self.calls.append((exchange, routing_key, headers))


def _spec(handler, key: str = "chat") -> PoolSpec:
    return PoolSpec(key=key, concurrency=1, handler=handler)


def _count(queue: str, outcome: str) -> float:
    return REGISTRY.get_sample_value("ai_jobs_total", {"queue": queue, "outcome": outcome}) or 0.0


async def _ok(d: Delivery) -> None:
    return None


async def _boom(d: Delivery) -> None:
    raise RuntimeError("gemini tam thoi loi")


async def _permanent(d: Delivery) -> None:
    raise PermanentError("json hong")


def _never_stopping() -> bool:
    return False


async def test_ThanhCong_Ack() -> None:
    before = _count("ai.chat.q", "ack")
    msg, fwd = FakeMessage(), FakeForward()

    outcome = await process(msg, _spec(_ok), fwd, _never_stopping)

    assert outcome == "ack"
    assert msg.acked and msg.nacked is None
    assert fwd.calls == []
    assert _count("ai.chat.q", "ack") == before + 1


async def test_LoiTamLanDau_ChuyenSangRetryVoiAttempt2() -> None:
    msg = FakeMessage(headers={"traceparent": "00-t-s-01", "x-death": [{}]})
    fwd = FakeForward()

    outcome = await process(msg, _spec(_boom), fwd, _never_stopping)

    assert outcome == "retry"
    assert msg.acked  # ban goc ack SAU khi ban sao da duoc broker xac nhan
    assert fwd.calls == [(RETRY_EXCHANGE, "chat", {"traceparent": "00-t-s-01", ATTEMPT_HEADER: 2})]


async def test_LoiTamLanThu3_VaoDLQGiuAttemptVaLyDo() -> None:
    msg = FakeMessage(headers={ATTEMPT_HEADER: 3})
    fwd = FakeForward()

    outcome = await process(msg, _spec(_boom, key="report"), fwd, _never_stopping)

    assert outcome == "dead_letter"
    assert msg.acked
    exchange, key, headers = fwd.calls[0]
    assert (exchange, key) == (DLX_EXCHANGE, "report")
    assert headers[ATTEMPT_HEADER] == 3
    assert "RuntimeError" in headers[ERROR_HEADER]


async def test_LoiVinhVien_VaoDLQNgayLanDau() -> None:
    fwd = FakeForward()
    outcome = await process(FakeMessage(), _spec(_permanent), fwd, _never_stopping)

    assert outcome == "dead_letter"
    assert fwd.calls[0][0] == DLX_EXCHANGE
    assert fwd.calls[0][2][ATTEMPT_HEADER] == 1


async def test_ChuyenTiepLoi_NackRequeueKhongAck() -> None:
    # Publish retry/DLQ hong ma van ack ban goc = mat message. Phai tra lai hang.
    msg = FakeMessage()
    outcome = await process(msg, _spec(_boom), FakeForward(fail=True), _never_stopping)

    assert outcome == "requeue"
    assert not msg.acked
    assert msg.nacked is True


async def test_DangTat_LoiThiTraLaiHangKhongChuyenTiep() -> None:
    msg, fwd = FakeMessage(), FakeForward()
    outcome = await process(msg, _spec(_boom), fwd, lambda: True)

    assert outcome == "requeue"
    assert msg.nacked is True
    assert fwd.calls == []


async def test_DangTat_ChuaXuLyThiTraLaiHangKhongGoiHandler() -> None:
    called = []

    async def handler(d: Delivery) -> None:
        called.append(d)

    msg = FakeMessage()
    outcome = await process(msg, _spec(handler), FakeForward(), lambda: True)

    assert outcome == "requeue"
    assert called == []
    assert msg.nacked is True


async def test_HandlerTraNhanKetQua_DungLamOutcome() -> None:
    async def expired(d: Delivery) -> str:
        return "expired"

    before = _count("ai.chat.q", "expired")
    msg = FakeMessage()
    outcome = await process(msg, _spec(expired), FakeForward(), _never_stopping)

    assert outcome == "expired"
    assert msg.acked
    assert _count("ai.chat.q", "expired") == before + 1


async def test_Lag_LayTuTimestampAMQP() -> None:
    ts = datetime.now(UTC) - timedelta(seconds=7)
    await process(FakeMessage(timestamp=ts), _spec(_ok, key="anomaly"), FakeForward(), _never_stopping)

    lag = REGISTRY.get_sample_value("ai_queue_lag_seconds", {"queue": "ai.anomaly.q"})
    assert 6.5 <= lag <= 9


async def test_Lag_KhongCoTimestampThiDungEnqueuedAtTrongBody() -> None:
    body = json.dumps({"job_id": "j", "enqueued_at": time.time() - 3}).encode()
    await process(FakeMessage(body=body), _spec(_ok, key="moderation"), FakeForward(), _never_stopping)

    lag = REGISTRY.get_sample_value("ai_queue_lag_seconds", {"queue": "ai.moderation.q"})
    assert 2.5 <= lag <= 5


async def test_Delivery_MangAttemptVaPayload() -> None:
    seen: list[Delivery] = []

    async def handler(d: Delivery) -> None:
        seen.append(d)

    body = json.dumps({"job_id": "j-9", "event_id": "e-1"}).encode()
    await process(FakeMessage(body=body, headers={ATTEMPT_HEADER: 2}), _spec(handler), FakeForward(),
                  _never_stopping)

    d = seen[0]
    assert d.attempt == 2
    assert d.body == body
    assert d.payload == {"job_id": "j-9", "event_id": "e-1"}
    # Handler sua header khong duoc lam hong ban chuyen tiep.
    d.headers["x"] = 1


async def test_Delivery_BodyKhongPhaiJsonThiPayloadRong() -> None:
    seen: list[Delivery] = []

    async def handler(d: Delivery) -> None:
        seen.append(d)

    await process(FakeMessage(body=b"\xff\xfe"), _spec(handler), FakeForward(), _never_stopping)
    assert seen[0].payload == {}


@pytest.mark.parametrize(("key", "want"), [("chat", "ai.chat.q"), ("report", "ai.report.q")])
def test_queue_name(key, want) -> None:
    assert queue_name(key) == want


class _SampleChannel:
    def __init__(self, fail: bool) -> None:
        self.fail = fail
        self.is_closed = False

    async def declare_queue(self, name, passive):
        if self.fail:
            self.is_closed = True  # broker dong channel khi 404
            raise RuntimeError("NOT_FOUND")
        from types import SimpleNamespace

        return SimpleNamespace(declaration_result=SimpleNamespace(message_count=0))


class _Conn:
    def __init__(self, fail_first: bool) -> None:
        self.opened: list[_SampleChannel] = []
        self._fail_first = fail_first

    async def channel(self, **kw):
        ch = _SampleChannel(fail=self._fail_first and not self.opened)
        self.opened.append(ch)
        return ch


async def test_Sampler_LoiDungChannelRiengKhongDungPool() -> None:
    from app.mq.runtime import ConsumerRuntime

    conn = _Conn(fail_first=True)
    rt = ConsumerRuntime(conn, [PoolSpec("chat", 1, _ok)], grace_seconds=1)
    pool_channel = object()
    rt._pools[0].channel = pool_channel  # channel cua pool: sampler khong duoc cham vao

    await rt.sample_once()  # lan dau: channel lay mau bi broker dong
    await rt.sample_once()  # lan sau: mo channel moi va doc duoc

    assert rt._pools[0].channel is pool_channel
    assert len(conn.opened) == 2
    assert REGISTRY.get_sample_value("ai_queue_depth", {"queue": "ai.chat.q"}) == 0


# ---- EV-159: dem message vao DLQ theo ly do va do sau ai.dlq ----

def _dead(queue: str, reason: str) -> float:
    return REGISTRY.get_sample_value("ai_dead_lettered_total", {"queue": queue, "reason": reason}) or 0.0


async def test_DLQ_DemTheoLyDo_VinhVien() -> None:
    before = _dead("ai.moderation.q", "permanent")
    await process(FakeMessage(), _spec(_permanent, key="moderation"), FakeForward(), _never_stopping)
    assert _dead("ai.moderation.q", "permanent") == before + 1


async def test_DLQ_DemTheoLyDo_HetLuot() -> None:
    before = _dead("ai.chat.q", "exhausted")
    await process(FakeMessage(headers={ATTEMPT_HEADER: 3}), _spec(_boom), FakeForward(), _never_stopping)
    assert _dead("ai.chat.q", "exhausted") == before + 1


async def test_DLQ_ChuyenTiepLoi_KhongDem() -> None:
    # Message chua vao DLQ (broker tu choi) -> khong duoc dem, neu khong alert bao sai.
    before = _dead("ai.report.q", "permanent")
    await process(FakeMessage(), _spec(_permanent, key="report"), FakeForward(fail=True), _never_stopping)
    assert _dead("ai.report.q", "permanent") == before


async def test_DLQ_GhiLoaiLoiVaoHeader() -> None:
    fwd = FakeForward()
    await process(FakeMessage(), _spec(_permanent, key="moderation"), fwd, _never_stopping)
    assert fwd.calls[0][2][ERROR_HEADER].startswith("permanent: PermanentError")


async def test_Sampler_DoSauDLQ() -> None:
    from types import SimpleNamespace

    from app.mq.runtime import ConsumerRuntime

    class _Ch:
        is_closed = False

        async def declare_queue(self, name, passive):
            n = 4 if name == "ai.dlq" else 0
            return SimpleNamespace(declaration_result=SimpleNamespace(message_count=n))

    class _C:
        async def channel(self, **kw):
            return _Ch()

    rt = ConsumerRuntime(_C(), [PoolSpec("chat", 1, _ok)], grace_seconds=1)
    rt._pools[0].channel = object()
    await rt.sample_once()
    assert REGISTRY.get_sample_value("ai_dlq_depth") == 4
