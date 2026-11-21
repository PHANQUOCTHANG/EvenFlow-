"""AC-3/7/8/10 qua consumer that (chat + kiem duyet) voi LlmClient that va SDK Anthropic gia."""

from __future__ import annotations

import json
import time

import anthropic
import pytest

from app.consumers.chat import ChatConsumer, Retry
from app.consumers.moderation import ModerationConsumer, make_handler
from app.domain.moderation import AiVerdict
from app.llm.client import RateLimited
from app.mq.runtime import Delivery
from app.pipeline import degradation

from .conftest import (
    HEADROOM,
    FakeAnthropic,
    FakeLimiter,
    FakeTools,
    http_request,
    make_client,
    mod_body,
    status_error,
    text_block,
    thinking_block,
)

INCOMPLETE_REASON = "AI không đưa ra kết luận, chuyển người duyệt"


class RecCache:
    def __init__(self) -> None:
        self.puts: list[tuple] = []

    async def get(self, event_id, question):
        return None

    async def put(self, event_id, question, answer):
        self.puts.append((event_id, question, answer))

    def key(self, event_id, question):
        return f"k:{event_id}:{question}"


class RecFlight:
    def __init__(self) -> None:
        self.published: list[tuple] = []
        self.released: list[str] = []

    async def acquire(self, key):
        return True

    async def wait(self, key, timeout):
        return None

    async def publish(self, key, answer):
        self.published.append((key, answer))

    async def release(self, key):
        self.released.append(key)


class Faq:
    async def match(self, *a):
        return None

    def fallback(self, notice):
        return "FALLBACK"


def _normal() -> degradation.Signals:
    return degradation.Signals(chat_queue_lag_seconds=0.0, budget_used_ratio=0.0, gemini_circuit_open=False)


def _raw(question: str = "Con ve khong?") -> bytes:
    return json.dumps({"job_id": "j-1", "event_id": "ev-1", "identity_id": "u-1", "question": question,
                       "queue_token": "qt", "enqueued_at": time.time()}).encode()


def _chat(sdk: FakeAnthropic, **kw):
    out: list[dict] = []
    cache, flight, lim = RecCache(), RecFlight(), FakeLimiter()

    async def publish(job_id, payload):
        out.append(payload)

    # Tham so vi tri: ten tham so client (gemini/llm) khong thuoc hop dong.
    consumer = ChatConsumer(lim, cache, Faq(), flight, make_client(sdk, **kw), FakeTools(), _normal, publish)
    return consumer, out, cache, flight, lim


# ---------------------------------------------------------------- chat


async def test_AC2_AC3_Chat_ThanhCong_GoiAnthropicVoiNganSachNormal() -> None:
    sdk = FakeAnthropic(content=[text_block("Con 120 ve.")])
    consumer, out, _cache, _flight, lim = _chat(sdk)

    await consumer.handle(_raw())

    assert out[-1]["source"] == "llm"
    assert out[-1]["answer"] == "Con 120 ve."
    call = sdk.messages.calls[0]
    assert call["max_tokens"] == 512 + HEADROOM
    assert lim.acquired[0] >= call["max_tokens"] + 256


async def test_AC7_Chat_429_ThanhRetry0() -> None:
    consumer, _out, cache, *_ = _chat(FakeAnthropic(exc=status_error(anthropic.RateLimitError, 429)))
    with pytest.raises(Retry) as ei:
        await consumer.handle(_raw())
    assert ei.value.delay_ms == 0
    assert cache.puts == []


@pytest.mark.parametrize(
    "sdk_kw",
    [
        {"exc": anthropic.APITimeoutError(request=http_request())},
        {"delay": 1.0},
    ],
    ids=["APITimeoutError", "wait_for"],
)
async def test_AC8_Chat_Timeout_TraLoiDuPhong_KhongCache(sdk_kw) -> None:
    consumer, out, cache, flight, _ = _chat(FakeAnthropic(**sdk_kw), timeout_seconds=0.05)
    await consumer.handle(_raw())
    assert out[-1]["source"] == "fallback"
    assert out[-1]["answer"] == "FALLBACK"
    assert cache.puts == []
    assert flight.published == []


@pytest.mark.parametrize(
    ("content", "stop"),
    [
        ([text_block("PHAN-TRA-LOI-DO-DANG")], "max_tokens"),
        ([text_block("PHAN-TRA-LOI-DO-DANG")], "refusal"),
        ([thinking_block()], "end_turn"),
    ],
    ids=["max_tokens", "refusal", "empty"],
)
async def test_AC10_Chat_KhongTronVen_DuPhong_KhongLoPhanDoDang(content, stop) -> None:
    consumer, out, cache, flight, _ = _chat(FakeAnthropic(content=content, stop_reason=stop))
    await consumer.handle(_raw())

    assert out[-1]["source"] == "fallback"
    assert "PHAN-TRA-LOI-DO-DANG" not in json.dumps(out, ensure_ascii=False)
    assert cache.puts == []
    assert flight.published == []


# ---------------------------------------------------------------- moderation


def _mod(sdk: FakeAnthropic):
    delivered: list[tuple] = []

    def cb(event_id, result):
        delivered.append((event_id, result))

    lim = FakeLimiter()
    consumer = ModerationConsumer(make_client(sdk), "http://event", callback_fn=cb, limiter=lim)
    return consumer, delivered, lim


def _delivery(attempt: int = 1) -> Delivery:
    return Delivery(body=mod_body(), headers={}, message_id=None, routing_key="moderation", attempt=attempt)


async def test_AC3_KiemDuyet_QuaLimiter_MaxTokens512CongHeadroom() -> None:
    sdk = FakeAnthropic(content=[text_block('{"score": 0.1, "axes": {}, "reasons": ["ok"]}')])
    consumer, delivered, lim = _mod(sdk)
    await make_handler(consumer)(_delivery())

    call = sdk.messages.calls[0]
    assert call["max_tokens"] == 512 + HEADROOM
    assert lim.acquired and lim.acquired[0] >= call["max_tokens"] + 256
    assert delivered[0][1].verdict is AiVerdict.AUTO_APPROVED


@pytest.mark.parametrize(
    ("content", "stop"),
    [
        ([text_block('{"score": 0.1}')], "refusal"),
        ([text_block('{"score": 0.1, "axes"')], "max_tokens"),
        ([], "end_turn"),
    ],
    ids=["refusal", "max_tokens", "empty"],
)
async def test_AC10_KiemDuyet_KhongTronVen_NeedsHumanMotLan_KhongRetry(content, stop) -> None:
    consumer, delivered, _ = _mod(FakeAnthropic(content=content, stop_reason=stop))

    await make_handler(consumer)(_delivery(attempt=1))  # khong nem -> khong retry

    assert len(delivered) == 1
    _, res = delivered[0]
    assert res.verdict is AiVerdict.NEEDS_HUMAN
    assert res.score == 0.5
    assert res.reasons == [INCOMPLETE_REASON]


async def test_AC7_KiemDuyet_429_LanDau_Retry() -> None:
    consumer, delivered, lim = _mod(FakeAnthropic(exc=status_error(anthropic.RateLimitError, 429)))
    with pytest.raises(RateLimited):
        await make_handler(consumer)(_delivery(attempt=1))
    assert delivered == []
    assert lim.refunds == [(lim.acquired[0], 0)]
