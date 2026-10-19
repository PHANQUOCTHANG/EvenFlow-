"""Unit tests for SingleFlight (EV-162)."""

from __future__ import annotations

import asyncio

import pytest

from app.pipeline.singleflight import LEADER_TTL_SECONDS, SingleFlight


class FakeRedis:
    def __init__(self) -> None:
        self.kv: dict[str, str | bytes] = {}
        self.ttls: dict[str, int] = {}

    async def set(
        self,
        key: str,
        value: str | bytes,
        *,
        nx: bool = False,
        ex: int | None = None,
    ) -> bool:
        if nx and key in self.kv:
            return False
        self.kv[key] = value
        if ex is not None:
            self.ttls[key] = ex
        return True

    async def get(self, key: str) -> str | bytes | None:
        return self.kv.get(key)

    async def setex(self, key: str, ttl: int, value: str | bytes) -> None:
        self.kv[key] = value
        self.ttls[key] = ttl

    async def delete(self, key: str) -> int:
        if key in self.kv:
            del self.kv[key]
            self.ttls.pop(key, None)
            return 1
        return 0


@pytest.mark.asyncio
async def test_singleflight_acquire():
    redis = FakeRedis()
    sf = SingleFlight(redis)
    key = "q-hash-123"

    # First caller acquires leadership
    assert await sf.acquire(key) is True
    assert redis.kv.get(f"{key}:flight") == "1"
    assert redis.ttls.get(f"{key}:flight") == LEADER_TTL_SECONDS

    # Second concurrent caller fails to acquire leadership
    assert await sf.acquire(key) is False


@pytest.mark.asyncio
async def test_singleflight_wait_success_concurrent():
    redis = FakeRedis()
    sf = SingleFlight(redis)
    key = "q-hash-456"

    async def delayed_leader():
        await asyncio.sleep(0.08)
        await sf.publish(key, "Ket qua cho 999 nguoi")

    # Start leader task concurrently
    leader_task = asyncio.create_task(delayed_leader())

    # Follower waits for result
    res = await sf.wait(key, timeout=1.0)
    assert res == "Ket qua cho 999 nguoi"

    await leader_task


@pytest.mark.asyncio
async def test_singleflight_wait_late_arrival():
    redis = FakeRedis()
    sf = SingleFlight(redis)
    key = "q-hash-789"

    # Result already published before wait() is called
    await sf.publish(key, "Ket qua da san sang")

    res = await sf.wait(key, timeout=0.5)
    assert res == "Ket qua da san sang"


@pytest.mark.asyncio
async def test_singleflight_wait_timeout():
    redis = FakeRedis()
    sf = SingleFlight(redis)
    key = "q-hash-timeout"

    # Nobody publishes result
    res = await sf.wait(key, timeout=0.1)
    assert res is None


@pytest.mark.asyncio
async def test_singleflight_release_allows_reacquire():
    redis = FakeRedis()
    sf = SingleFlight(redis)
    key = "q-hash-reacquire"

    assert await sf.acquire(key) is True
    assert await sf.acquire(key) is False

    await sf.release(key)
    assert f"{key}:flight" not in redis.kv

    # Now acquire succeeds again
    assert await sf.acquire(key) is True


@pytest.mark.asyncio
async def test_singleflight_bytes_result_decoding():
    redis = FakeRedis()
    sf = SingleFlight(redis)
    key = "q-hash-bytes"

    redis.kv[f"{key}:result"] = b"Ket qua dang bytes"
    res = await sf.wait(key, timeout=0.5)
    assert res == "Ket qua dang bytes"
