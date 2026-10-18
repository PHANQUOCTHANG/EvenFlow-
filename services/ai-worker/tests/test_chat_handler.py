"""Noi ChatConsumer vao runtime: loi nao retry, loi nao vao DLQ, loi nao khach van co cau tra loi."""

from __future__ import annotations

import json
import time

import pytest

from app.consumers.chat import ChatConsumer, Retry
from app.gemini.client import Generation, RateLimited
from app.gemini.limiter import Permit
from app.mq.protocol import PermanentError
from app.pipeline import degradation


class FakeLimiter:
    def __init__(self, granted: bool = True) -> None:
        self.granted = granted
        self.refunds: list[tuple[int, int]] = []

    async def acquire(self, est: int) -> Permit:
        return Permit(True, 0, est) if self.granted else Permit(False, 1500)

    async def refund(self, reserved: int, actual: int) -> None:
        self.refunds.append((reserved, actual))


class FakeCache:
    async def get(self, event_id, question):
        return None

    async def put(self, event_id, question, answer):
        pass

    def key(self, event_id, question):
        return f"k:{event_id}:{question}"


class FakeFaq:
    async def match(self, *a):
        return None

    def fallback(self, notice):
        return "FALLBACK"


class FakeFlight:
    async def acquire(self, key):
        return True

    async def wait(self, key, timeout):
        return None

    async def publish(self, key, answer):
        pass

    async def release(self, key):
        pass


class FakeGemini:
    def __init__(self, *, exc: Exception | None = None, total: int | None = 50) -> None:
        self._exc, self._total = exc, total

    async def build_prompt(self, job, tools):
        return "prompt"

    async def generate(self, prompt, *, max_output_tokens):
        if self._exc is not None:
            raise self._exc
        return Generation(text="tra loi", total_tokens=self._total, model="fake")


def _consumer(gemini: FakeGemini, limiter: FakeLimiter | None = None, out: list | None = None):
    out = out if out is not None else []

    async def publish(job_id, payload):
        out.append(payload)

    return ChatConsumer(
        limiter=limiter or FakeLimiter(),
        cache=FakeCache(),
        faq=FakeFaq(),
        flight=FakeFlight(),
        gemini=gemini,
        tools=None,
        signals=lambda: degradation.Signals(0.0, 0.0, False),
        publish_result=publish,
    )


def _job(**over) -> bytes:
    job = {
        "job_id": "j-1", "event_id": "e-1", "identity_id": "i-1",
        "question": "cau hoi mo", "queue_token": "t", "enqueued_at": time.time(),
    }
    job.update(over)
    return json.dumps(job).encode()


async def test_429_ThanhRetryKhongTraFallbackChoKhach() -> None:
    out: list = []
    limiter = FakeLimiter()
    c = _consumer(FakeGemini(exc=RateLimited("429")), limiter, out)

    with pytest.raises(Retry):
        await c.handle(_job())

    assert out == []  # khach khong nhan cau tra loi du phong vi job se duoc thu lai
    _reserved, actual = limiter.refunds[0]
    assert actual == 0  # 429: Gemini khong tinh token -> hoan het


async def test_LimiterChuaChoPhep_Retry() -> None:
    with pytest.raises(Retry):
        await _consumer(FakeGemini(), FakeLimiter(granted=False)).handle(_job())


async def test_GeminiLoiKhac_KhachVanNhanFallback_KhongHoanToken() -> None:
    out: list = []
    limiter = FakeLimiter()
    await _consumer(FakeGemini(exc=TimeoutError()), limiter, out).handle(_job())

    assert out[0]["source"] == "fallback"
    reserved, actual = limiter.refunds[0]
    # Timeout: Gemini co the da tinh token. Khong hoan de khong vuot TPM (uoc luong thua).
    assert actual == reserved


async def test_KhongBietTokenThat_KhongHoan() -> None:
    limiter = FakeLimiter()
    await _consumer(FakeGemini(total=None), limiter).handle(_job())
    reserved, actual = limiter.refunds[0]
    assert actual == reserved


async def test_CoTokenThat_HoanPhanThua() -> None:
    limiter = FakeLimiter()
    out: list = []
    await _consumer(FakeGemini(total=50), limiter, out).handle(_job())
    assert limiter.refunds[0][1] == 50
    assert out[0]["source"] == "llm"


async def test_JobQuaHan_TraNhanExpired() -> None:
    out: list = []
    outcome = await _consumer(FakeGemini(), out=out).handle(_job(enqueued_at=time.time() - 61))
    assert outcome == "expired"
    assert out == []


@pytest.mark.parametrize("raw", [b"khong phai json", b"{}", json.dumps({"job_id": "x"}).encode()])
async def test_BodyHong_LaLoiVinhVien(raw) -> None:
    with pytest.raises(PermanentError):
        await _consumer(FakeGemini()).handle(raw)


# ---- Lan thu cuoi: khach van phai co cau tra loi (BR-A1) ----
# Luc mo ban, limiter tu choi la chuyen BINH THUONG (5.000 cau/phut vs 55 RPM). Neu job
# chat het luot roi vao DLQ thi khach doi mai khong co gi -- trai "khach khong bao gio
# thay loi". Lan cuoi phai tra loi du phong va ack.


def _delivery(raw: bytes, attempt: int):
    from app.mq.runtime import Delivery

    return Delivery(body=raw, headers={}, message_id="m", routing_key="chat", attempt=attempt)


async def test_LanCuoiLimiterVanTuChoi_TraLoiDuPhongVaAck() -> None:
    from app.consumers.chat import make_handler
    from app.mq.protocol import MAX_ATTEMPTS

    out: list = []
    handler = make_handler(_consumer(FakeGemini(), FakeLimiter(granted=False), out))

    outcome = await handler(_delivery(_job(), MAX_ATTEMPTS))

    assert outcome == "fallback"
    assert out[0]["source"] == "fallback"
    assert out[0]["job_id"] == "j-1"


async def test_ChuaToiLanCuoi_VanRetry() -> None:
    from app.consumers.chat import make_handler

    handler = make_handler(_consumer(FakeGemini(), FakeLimiter(granted=False)))
    with pytest.raises(Retry):
        await handler(_delivery(_job(), 2))


async def test_LanCuoi429_CungTraLoiDuPhong() -> None:
    from app.consumers.chat import make_handler
    from app.mq.protocol import MAX_ATTEMPTS

    out: list = []
    handler = make_handler(_consumer(FakeGemini(exc=RateLimited("429")), out=out))
    assert await handler(_delivery(_job(), MAX_ATTEMPTS)) == "fallback"
    assert out[0]["source"] == "fallback"
