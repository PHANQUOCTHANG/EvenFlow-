"""Fix X-4 (stop_reason allowlist) va X-6 (kep usage, khong hoan qua phan dat cho)."""

from __future__ import annotations

from types import SimpleNamespace

import anthropic
import httpx2
import pytest
from prometheus_client import REGISTRY

from app.llm.client import LlmIncomplete, Prompt
from app.llm.guarded import guarded_generate
from app.llm.limiter import GlobalRateLimiter

from .conftest import ANTHROPIC_KEY, FakeAnthropic, FakeLimiter, make_client, text_block

PARTIAL = "Phan tra loi bi cat giua ch"


def _incomplete(reason: str) -> float:
    return REGISTRY.get_sample_value("ai_llm_incomplete_total", {"reason": reason}) or 0.0


# ---------------------------------------------------------------- X-4


@pytest.mark.parametrize(
    ("stop", "label"),
    [
        ("model_context_window_exceeded", "other"),
        ("pause_turn", "other"),
        ("tool_use", "other"),
        (None, "other"),
        ("weird_future", "other"),
        ("", "other"),
        ("max_tokens", "max_tokens"),
        ("refusal", "refusal"),
    ],
)
async def test_X4_StopReasonNgoaiAllowlist_LlmIncomplete(stop, label) -> None:
    sdk = FakeAnthropic(content=[text_block(PARTIAL)], stop_reason=stop)
    before = _incomplete(label)
    with pytest.raises(LlmIncomplete) as ei:
        await make_client(sdk).generate(Prompt("s", "u"), max_output_tokens=16)
    assert ei.value.reason == label
    assert PARTIAL not in str(ei.value)
    assert _incomplete(label) == before + 1


async def test_X4_NhanMetricCoDinh_KhongTheoGiaTriLa() -> None:
    sdk = FakeAnthropic(content=[text_block(PARTIAL)], stop_reason="gia_tri_la_xyz_123")
    with pytest.raises(LlmIncomplete):
        await make_client(sdk).generate(Prompt("s", "u"), max_output_tokens=16)
    assert REGISTRY.get_sample_value("ai_llm_incomplete_total", {"reason": "gia_tri_la_xyz_123"}) is None


def _msg(stop: str) -> dict:
    return {"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-haiku-5-5",
            "content": [{"type": "text", "text": "cut off mid"}], "stop_reason": stop, "stop_sequence": None,
            "usage": {"input_tokens": 10, "output_tokens": 5}}


async def test_X4_SdkThat_ContextWindowExceeded_KhongTraVe() -> None:
    def handler(req):
        return httpx2.Response(200, json=_msg("model_context_window_exceeded"))

    sdk = anthropic.AsyncAnthropic(api_key=ANTHROPIC_KEY, max_retries=0, timeout=5,
                                   http_client=httpx2.AsyncClient(transport=httpx2.MockTransport(handler)))
    with pytest.raises(LlmIncomplete):
        await make_client(sdk).generate(Prompt("s", "u"), max_output_tokens=16)


async def test_X4_SdkThat_EndTurn_VanTraVe() -> None:
    def handler(req):
        return httpx2.Response(200, json=_msg("end_turn"))

    sdk = anthropic.AsyncAnthropic(api_key=ANTHROPIC_KEY, max_retries=0, timeout=5,
                                   http_client=httpx2.AsyncClient(transport=httpx2.MockTransport(handler)))
    gen = await make_client(sdk).generate(Prompt("s", "u"), max_output_tokens=16)
    assert gen.text == "cut off mid"
    assert gen.total_tokens == 15


# ---------------------------------------------------------------- X-6


@pytest.mark.parametrize(
    "usage",
    [
        SimpleNamespace(input_tokens=-900000, output_tokens=0),
        SimpleNamespace(input_tokens=10, output_tokens=-1),
        SimpleNamespace(input_tokens=1.0, output_tokens=2),
        SimpleNamespace(input_tokens=True, output_tokens=2),
        SimpleNamespace(input_tokens="10", output_tokens=2),
    ],
)
async def test_X6_UsageAmHoacKhongPhaiSoNguyen_KhongBiet_KhongHoan(usage) -> None:
    sdk = FakeAnthropic(usage=usage)
    c = make_client(sdk)
    gen = await c.generate(Prompt("s", "u"), max_output_tokens=16)
    assert gen.total_tokens is None

    lim = FakeLimiter()
    await guarded_generate(lim, make_client(FakeAnthropic(usage=usage)), Prompt("s", "u"), max_output_tokens=512)
    assert lim.refunds == [(lim.acquired[0], lim.acquired[0])]


async def test_X6_UsageVuotPhanDatCho_KhongHoanAm() -> None:
    lim = FakeLimiter()
    sdk = FakeAnthropic(usage=SimpleNamespace(input_tokens=10**9, output_tokens=1))
    await guarded_generate(lim, make_client(sdk), Prompt("s", "u"), max_output_tokens=512)
    reserved, actual = lim.refunds[0]
    assert actual == reserved


class _FakeTotal:
    """Client gia tra total_tokens tuy y (vd. tu mot nha cung cap khac) -- guarded van phai kep."""

    def __init__(self, total: int) -> None:
        self._total = total

    def sent_max_tokens(self, budget: int) -> int:
        return budget

    async def generate(self, prompt, *, max_output_tokens):
        from app.llm.client import Generation

        return Generation(text="ok", total_tokens=self._total, model="m")


@pytest.mark.parametrize(("total", "expected"), [(-900000, 0), (-1, 0), (0, 0), (50, 50), (10**9, None)])
async def test_X6_Guarded_KepActualTrongKhoangDatCho(total, expected) -> None:
    lim = FakeLimiter()
    await guarded_generate(lim, _FakeTotal(total), Prompt("s", "u"), max_output_tokens=512)
    reserved, actual = lim.refunds[0]
    assert 0 <= actual <= reserved
    assert actual == (reserved if expected is None else expected)


async def test_X6_LimiterThat_KhongBaoGioHoanQuaPhanDatCho() -> None:
    calls: list[tuple] = []

    class Redis:
        async def script_load(self, s):
            return "sha"

        async def evalsha(self, *a):
            calls.append(a)
            return (1, 0)

    real = GlobalRateLimiter(Redis())

    class Lim:
        async def acquire(self, est):
            return await real.acquire(est)

        async def refund(self, reserved, actual):
            await real.refund(reserved, actual)

    await guarded_generate(Lim(), _FakeTotal(-900000), Prompt("s", "u"), max_output_tokens=512)
    reserved = next(a for a in calls if "acquire" in a)[-1]
    refunds = [a for a in calls if "refund" in a]
    assert refunds, calls
    # delta gui cho Lua = phan hoan lai: khong bao gio lon hon phan da dat cho.
    assert 0 <= refunds[-1][-1] <= reserved


@pytest.mark.parametrize(("reserved", "actual", "delta"), [(2000, -900000, 2000), (2000, -1, 2000), (2000, 0, 2000),
                                                           (2000, 500, 1500), (2000, 2000, None), (2000, 9999, None)])
async def test_X6_LimiterThat_RefundTrucTiep_KhongVuotPhanDatCho(reserved, actual, delta) -> None:
    calls: list[tuple] = []

    class Redis:
        async def script_load(self, s):
            return "sha"

        async def evalsha(self, *a):
            calls.append(a)
            return (1, 0)

    await GlobalRateLimiter(Redis()).refund(reserved, actual)
    if delta is None:
        assert calls == []
    else:
        assert calls[-1][-1] == delta
