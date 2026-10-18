"""Moi lan goi Gemini that deu phai qua limiter (BR-A1, L6) -- chat lan kiem duyet."""

from __future__ import annotations

import json
from types import SimpleNamespace

import pytest

from app.consumers.moderation import ModerationConsumer, make_handler
from app.domain.moderation import AiVerdict
from app.gemini.client import GeminiClient, Generation, RateLimited
from app.gemini.guarded import PermitDenied, guarded_generate
from app.gemini.limiter import Permit
from app.mq.protocol import MAX_ATTEMPTS
from app.mq.runtime import Delivery


class FakeLimiter:
    def __init__(self, granted: bool = True) -> None:
        self.granted = granted
        self.acquired: list[int] = []
        self.refunds: list[tuple[int, int]] = []

    async def acquire(self, est: int) -> Permit:
        self.acquired.append(est)
        return Permit(True, 0, est) if self.granted else Permit(False, 2500)

    async def refund(self, reserved: int, actual: int) -> None:
        self.refunds.append((reserved, actual))


class FakeGen:
    def __init__(self, *, total: int | None = 40, exc: Exception | None = None, text: str = "ok") -> None:
        self.calls = 0
        self._total, self._exc, self._text = total, exc, text

    async def generate(self, prompt, *, max_output_tokens):
        self.calls += 1
        if self._exc is not None:
            raise self._exc
        return Generation(text=self._text, total_tokens=self._total, model="fake")


# ---------------------------------------------------------------- guarded_generate


async def test_ChuaDuocPhep_KhongGoiGeminiVaKhongHoan() -> None:
    lim, gen = FakeLimiter(granted=False), FakeGen()
    with pytest.raises(PermitDenied) as ei:
        await guarded_generate(lim, gen, "p", max_output_tokens=100)
    assert ei.value.wait_ms == 2500
    assert gen.calls == 0
    assert lim.refunds == []


async def test_DatChoTruocRoiHoanPhanThua() -> None:
    lim, gen = FakeLimiter(), FakeGen(total=40)
    result = await guarded_generate(lim, gen, "p" * 300, max_output_tokens=100)
    assert result.text == "ok"
    reserved = lim.acquired[0]
    assert reserved >= 100 + 100  # uoc luong THUA: prompt/3 + toan bo dau ra + du phong
    assert lim.refunds == [(reserved, 40)]


async def test_429_HoanHetVaNemLai() -> None:
    lim = FakeLimiter()
    with pytest.raises(RateLimited):
        await guarded_generate(lim, FakeGen(exc=RateLimited("429")), "p", max_output_tokens=10)
    assert lim.refunds[0][1] == 0


async def test_LoiKhac_KhongHoan() -> None:
    lim = FakeLimiter()
    with pytest.raises(TimeoutError):
        await guarded_generate(lim, FakeGen(exc=TimeoutError()), "p", max_output_tokens=10)
    reserved, actual = lim.refunds[0]
    assert actual == reserved


async def test_KhongBietTokenThat_KhongHoan() -> None:
    lim = FakeLimiter()
    await guarded_generate(lim, FakeGen(total=None), "p", max_output_tokens=10)
    reserved, actual = lim.refunds[0]
    assert actual == reserved


# ---------------------------------------------------------------- kiem duyet qua limiter


class FakeModels:
    def __init__(self, text: str) -> None:
        self.calls = 0
        self._text = text

    async def generate_content(self, *, model, contents, config):
        self.calls += 1
        return SimpleNamespace(text=self._text, usage_metadata=SimpleNamespace(total_token_count=77))


def _real_gemini(text: str = '{"score": 0.1, "axes": {}, "reasons": ["ok"]}'):
    models = FakeModels(text)
    client = GeminiClient(api_key="k", mock=False, sdk=SimpleNamespace(aio=SimpleNamespace(models=models)))
    return client, models


def _job_bytes() -> bytes:
    return json.dumps({
        "job_id": "m-1", "event_id": "evt-1", "event_title": "Dem nhac",
        "event_description": "Acoustic", "enqueued_at": 1.0,
    }).encode()


async def test_KiemDuyetThat_DiQuaLimiter() -> None:
    gemini, models = _real_gemini()
    lim = FakeLimiter()
    verdicts: list = []

    async def cb(event_id, result):
        verdicts.append((event_id, result))

    consumer = ModerationConsumer(gemini, "http://x", callback_fn=cb, limiter=lim)
    result = await consumer.handle_message(_job_bytes())

    assert result.verdict == AiVerdict.AUTO_APPROVED
    assert models.calls == 1
    assert len(lim.acquired) == 1
    assert lim.refunds[0][1] == 77


async def test_KiemDuyetThat_LimiterTuChoi_KhongGoiGemini() -> None:
    gemini, models = _real_gemini()
    consumer = ModerationConsumer(gemini, "http://x", callback_fn=lambda *a: None,
                                  limiter=FakeLimiter(granted=False))
    with pytest.raises(PermitDenied):
        await consumer.handle_message(_job_bytes())
    assert models.calls == 0


def _delivery(attempt: int) -> Delivery:
    return Delivery(body=_job_bytes(), headers={}, message_id="m-1", routing_key="moderation", attempt=attempt)


async def test_KiemDuyet_ChuaLanCuoi_LimiterTuChoiThiRetry() -> None:
    gemini, _ = _real_gemini()
    consumer = ModerationConsumer(gemini, "http://x", callback_fn=lambda *a: None,
                                  limiter=FakeLimiter(granted=False))
    with pytest.raises(PermitDenied):
        await make_handler(consumer)(_delivery(1))


async def test_KiemDuyet_LanCuoiVanKhongGoiDuoc_ChuyenNguoiDuyet() -> None:
    # AI khong chay duoc thi su kien khong duoc ket o PENDING_REVIEW: chuyen Moderator (BR-E4).
    gemini, models = _real_gemini()
    verdicts: list = []

    async def cb(event_id, result):
        verdicts.append((event_id, result))

    consumer = ModerationConsumer(gemini, "http://x", callback_fn=cb, limiter=FakeLimiter(granted=False))
    outcome = await make_handler(consumer)(_delivery(MAX_ATTEMPTS))

    assert outcome == "needs_human"
    assert models.calls == 0
    event_id, result = verdicts[0]
    assert event_id == "evt-1"
    assert result.verdict == AiVerdict.NEEDS_HUMAN
    assert result.reasons


async def test_KiemDuyet_LanCuoi429_CungChuyenNguoiDuyet() -> None:
    gemini = GeminiClient(api_key="k", mock=False, sdk=SimpleNamespace(aio=SimpleNamespace(models=None)))
    verdicts: list = []

    async def cb(event_id, result):
        verdicts.append(result)

    class _Lim(FakeLimiter):
        pass

    consumer = ModerationConsumer(gemini, "http://x", callback_fn=cb, limiter=_Lim())

    async def boom(*a, **k):
        raise RateLimited("429")

    gemini.generate = boom  # type: ignore[method-assign]
    assert await make_handler(consumer)(_delivery(MAX_ATTEMPTS)) == "needs_human"
    assert verdicts[0].verdict == AiVerdict.NEEDS_HUMAN


async def test_KiemDuyetMock_VanChayKhongCanLimiter() -> None:
    # Hop dong cu (EV-120): mock khong goi Gemini nen khong can xin phep.
    verdicts: list = []

    async def cb(event_id, result):
        verdicts.append(result)

    consumer = ModerationConsumer(GeminiClient(mock=True), "http://x", callback_fn=cb)
    await consumer.handle_message(_job_bytes())
    assert verdicts[0].verdict == AiVerdict.AUTO_APPROVED
