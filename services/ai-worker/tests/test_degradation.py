"""Unit tests for degradation decisions, SignalReader, and SignalCache (EV-164)."""

from __future__ import annotations

import asyncio

import pytest

from app.pipeline.degradation import (
    KEY_BUDGET_RATIO,
    KEY_CIRCUIT_OPEN,
    KEY_MANUAL_OVERRIDE,
    KEY_QUEUE_LAG,
    Mode,
    SignalCache,
    SignalReader,
    Signals,
    _parse_bool,
    _parse_float,
    _parse_mode,
    decide,
    policy_for,
)


class FakeRedis:
    def __init__(self, data: dict[str, str | bytes] | None = None, fail: bool = False) -> None:
        self.data = data or {}
        self.fail = fail

    async def get(self, key: str) -> str | bytes | None:
        if self.fail:
            raise ConnectionError("Redis connection down")
        return self.data.get(key)


def test_decide_thresholds():
    # 1. Normal
    s_normal = Signals(chat_queue_lag_seconds=0.0, budget_used_ratio=0.0, gemini_circuit_open=False)
    assert decide(s_normal) == Mode.NORMAL

    # 2. Saving by lag
    s_lag_saving = Signals(chat_queue_lag_seconds=35.0, budget_used_ratio=0.1, gemini_circuit_open=False)
    assert decide(s_lag_saving) == Mode.SAVING

    # 3. Saving by budget
    s_budget_saving = Signals(chat_queue_lag_seconds=5.0, budget_used_ratio=0.85, gemini_circuit_open=False)
    assert decide(s_budget_saving) == Mode.SAVING

    # 4. FAQ_ONLY by lag
    s_lag_faq = Signals(chat_queue_lag_seconds=125.0, budget_used_ratio=0.1, gemini_circuit_open=False)
    assert decide(s_lag_faq) == Mode.FAQ_ONLY

    # 5. FAQ_ONLY by budget
    s_budget_faq = Signals(chat_queue_lag_seconds=5.0, budget_used_ratio=1.0, gemini_circuit_open=False)
    assert decide(s_budget_faq) == Mode.FAQ_ONLY

    # 6. Circuit breaker open -> OFF
    s_circuit = Signals(chat_queue_lag_seconds=0.0, budget_used_ratio=0.0, gemini_circuit_open=True)
    assert decide(s_circuit) == Mode.OFF

    # 7. Manual override beats everything
    s_override = Signals(
        chat_queue_lag_seconds=200.0,
        budget_used_ratio=1.5,
        gemini_circuit_open=True,
        manual_override=Mode.NORMAL,
    )
    assert decide(s_override) == Mode.NORMAL


def test_policies():
    for mode in Mode:
        pol = policy_for(mode)
        assert pol is not None
        if mode == Mode.NORMAL:
            assert pol.call_llm is True
            assert pol.allow_open_questions is True
        elif mode in (Mode.FAQ_ONLY, Mode.OFF):
            assert pol.call_llm is False
            assert pol.allow_open_questions is False


def test_parsers():
    assert _parse_float("12.5") == 12.5
    assert _parse_float(b"42.0") == 42.0
    assert _parse_float("not-a-number", default=5.0) == 5.0
    assert _parse_float(None, default=1.0) == 1.0

    assert _parse_bool("1") is True
    assert _parse_bool(b"true") is True
    assert _parse_bool("open") is True
    assert _parse_bool("0") is False
    assert _parse_bool(None) is False

    assert _parse_mode("SAVING") == Mode.SAVING
    assert _parse_mode(b"FAQ_ONLY") == Mode.FAQ_ONLY
    assert _parse_mode("AUTO") is None
    assert _parse_mode("UNKNOWN") is None
    assert _parse_mode(None) is None


@pytest.mark.asyncio
async def test_signal_reader_fetch_success():
    redis = FakeRedis({
        KEY_QUEUE_LAG: "45.0",
        KEY_BUDGET_RATIO: "0.82",
        KEY_CIRCUIT_OPEN: "0",
        KEY_MANUAL_OVERRIDE: "SAVING",
    })
    reader = SignalReader(redis)
    signals = await reader.fetch()

    assert signals.chat_queue_lag_seconds == 45.0
    assert signals.budget_used_ratio == 0.82
    assert signals.gemini_circuit_open is False
    assert signals.manual_override == Mode.SAVING


@pytest.mark.asyncio
async def test_signal_reader_fetch_defaults_on_missing_or_error():
    # Empty redis
    redis_empty = FakeRedis({})
    reader_empty = SignalReader(redis_empty)
    signals_empty = await reader_empty.fetch(default_override=Mode.NORMAL)
    assert signals_empty.chat_queue_lag_seconds == 0.0
    assert signals_empty.budget_used_ratio == 0.0
    assert signals_empty.gemini_circuit_open is False
    assert signals_empty.manual_override == Mode.NORMAL

    # Redis error
    redis_fail = FakeRedis(fail=True)
    reader_fail = SignalReader(redis_fail)
    signals_fail = await reader_fail.fetch(default_override=Mode.SAVING)
    assert signals_fail.chat_queue_lag_seconds == 0.0
    assert signals_fail.budget_used_ratio == 0.0
    assert signals_fail.manual_override == Mode.SAVING


@pytest.mark.asyncio
async def test_signal_cache_refresh_and_background_loop():
    redis = FakeRedis({
        KEY_QUEUE_LAG: "10.0",
    })
    reader = SignalReader(redis)
    cache = SignalCache(reader)

    assert cache.get_signals().chat_queue_lag_seconds == 0.0

    # Refresh
    await cache.refresh()
    assert cache.get_signals().chat_queue_lag_seconds == 10.0

    # Start background refresh
    cache.start_background_refresh(interval_seconds=0.05)

    # Change redis data
    redis.data[KEY_QUEUE_LAG] = "50.0"
    await asyncio.sleep(0.12)
    assert cache.get_signals().chat_queue_lag_seconds == 50.0

    # Stop background refresh
    await cache.stop_background_refresh()
    assert cache._task is None
