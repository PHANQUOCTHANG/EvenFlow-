from __future__ import annotations

import json
from typing import Self

import pytest

from app.pipeline.semantic_cache import (
    TTL_SECONDS,
    SemanticCache,
    _cosine,
    normalize,
)


class FakePipeline:
    def __init__(self, redis: FakeRedis) -> None:
        self.redis = redis
        self.ops: list[tuple] = []

    def hset(self, name: str, key: str, value: str) -> FakePipeline:
        self.ops.append(("hset", name, key, value))
        return self

    def expire(self, name: str, ttl: int) -> FakePipeline:
        self.ops.append(("expire", name, ttl))
        return self

    async def execute(self) -> list:
        for op in self.ops:
            if op[0] == "hset":
                _, name, k, v = op
                if name not in self.redis.hashes:
                    self.redis.hashes[name] = {}
                self.redis.hashes[name][k] = v
            elif op[0] == "expire":
                _, name, ttl = op
                self.redis.ttls[name] = ttl
        return [True] * len(self.ops)

    async def __aenter__(self) -> Self:
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb) -> None:
        pass


class FakeRedis:
    def __init__(self) -> None:
        self.kv: dict[str, str | bytes] = {}
        self.ttls: dict[str, int] = {}
        self.hashes: dict[str, dict[str, str]] = {}

    async def get(self, key: str):
        return self.kv.get(key)

    async def setex(self, key: str, ttl: int, value: str) -> None:
        self.kv[key] = value
        self.ttls[key] = ttl

    async def hgetall(self, name: str) -> dict[str, str]:
        return self.hashes.get(name, {})

    def pipeline(self) -> FakePipeline:
        return FakePipeline(self)


class FakeEmbedder:
    def __init__(self, vectors: dict[str, list[float]] | None = None, fail: bool = False) -> None:
        self.vectors = vectors or {}
        self.fail = fail
        self.call_count = 0

    async def embed(self, text: str) -> list[float]:
        self.call_count += 1
        if self.fail:
            raise RuntimeError("Embedder service unavailable")
        return self.vectors.get(text, [1.0, 0.0, 0.0])


def test_normalize():
    assert normalize("Còn vé không?") == "con ve khong"
    assert normalize("ĐẾN BAO GIỜ THÌ MỞ?") == "den bao gio thi mo"
    assert normalize("   gia   ve   la   bao   nhieu???  ") == "gia ve la bao nhieu"
    assert normalize("Vé VIP... còn không!") == "ve vip con khong"


def test_cosine_similarity():
    # Identical
    assert pytest.approx(_cosine([1.0, 0.0], [1.0, 0.0])) == 1.0
    # Orthogonal
    assert pytest.approx(_cosine([1.0, 0.0], [0.0, 1.0])) == 0.0
    # Opposite
    assert pytest.approx(_cosine([1.0, 0.0], [-1.0, 0.0])) == -1.0
    # Different lengths
    assert _cosine([1.0, 0.0], [1.0, 0.0, 0.5]) == 0.0
    # Zero vector
    assert _cosine([0.0, 0.0], [1.0, 1.0]) == 0.0


@pytest.mark.asyncio
async def test_semantic_cache_exact_hit():
    redis = FakeRedis()
    embedder = FakeEmbedder()
    cache = SemanticCache(redis, embedder)

    event_id = "evt-123"
    question = "Còn vé không?"
    key = cache.key(event_id, question)
    redis.kv[key] = "Con ve VIP va GA"

    # Exact hit should return immediately without calling embedder
    result = await cache.get(event_id, question)
    assert result == "Con ve VIP va GA"
    assert embedder.call_count == 0

    # Also works when redis returns bytes
    redis.kv[key] = b"Con ve VIP va GA bytes"
    result_bytes = await cache.get(event_id, question)
    assert result_bytes == "Con ve VIP va GA bytes"
    assert embedder.call_count == 0


@pytest.mark.asyncio
async def test_semantic_cache_semantic_hit():
    redis = FakeRedis()
    # Embedder returns [1.0, 0.0] for original question, and [0.99, 0.05] for rephrased question
    embedder = FakeEmbedder(vectors={
        "con ve khong": [1.0, 0.0],
        "het ve chua ban": [0.99, 0.05],
    })
    cache = SemanticCache(redis, embedder)

    event_id = "evt-100"
    # Put original into cache
    await cache.put(event_id, "con ve khong", "Ve van con nhieu")
    assert embedder.call_count == 1

    # Now ask rephrased question (exact cache will miss)
    result = await cache.get(event_id, "het ve chua ban")
    assert result == "Ve van con nhieu"
    assert embedder.call_count == 2


@pytest.mark.asyncio
async def test_semantic_cache_threshold_miss():
    redis = FakeRedis()
    # Vector is far (< 0.93 similarity)
    embedder = FakeEmbedder(vectors={
        "hoi cau khac": [0.5, 0.5],
    })
    cache = SemanticCache(redis, embedder)

    event_id = "evt-100"
    vec_key = f"ai:vec:{{{event_id}}}"
    redis.hashes[vec_key] = {
        "k1": json.dumps({"v": [1.0, 0.0], "a": "Tra loi cu"}),
    }

    result = await cache.get(event_id, "hoi cau khac")
    assert result is None


@pytest.mark.asyncio
async def test_semantic_cache_custom_threshold():
    redis = FakeRedis()
    # Cosine of [1.0, 0.0] and [0.8, 0.6] is 0.8
    embedder = FakeEmbedder(vectors={
        "hoi tuong doi": [0.8, 0.6],
    })
    # Default threshold 0.93 would reject 0.8
    cache_default = SemanticCache(redis, embedder)
    event_id = "evt-100"
    vec_key = f"ai:vec:{{{event_id}}}"
    redis.hashes[vec_key] = {
        "k1": json.dumps({"v": [1.0, 0.0], "a": "Tra loi tuong doi"}),
    }
    assert await cache_default.get(event_id, "hoi tuong doi") is None

    # Lower threshold 0.75 accepts 0.8
    cache_low = SemanticCache(redis, embedder, similarity_threshold=0.75)
    assert await cache_low.get(event_id, "hoi tuong doi") == "Tra loi tuong doi"


@pytest.mark.asyncio
async def test_semantic_cache_embedder_failure():
    redis = FakeRedis()
    embedder = FakeEmbedder(fail=True)
    cache = SemanticCache(redis, embedder)

    event_id = "evt-100"
    # In get(): embedder failure should return None gracefully without raising
    result = await cache.get(event_id, "cau hoi bat ky")
    assert result is None

    # In put(): exact key is set, vector put skipped gracefully without raising
    await cache.put(event_id, "cau hoi", "cau tra loi")
    assert redis.kv[cache.key(event_id, "cau hoi")] == "cau tra loi"
    assert redis.ttls[cache.key(event_id, "cau hoi")] == TTL_SECONDS
    assert f"ai:vec:{{{event_id}}}" not in redis.hashes


@pytest.mark.asyncio
async def test_semantic_cache_nearest_empty_or_corrupted():
    redis = FakeRedis()
    embedder = FakeEmbedder()
    cache = SemanticCache(redis, embedder)

    event_id = "evt-100"
    # No hash in redis
    assert await cache.get(event_id, "cau hoi") is None

    # Corrupted json in hash
    redis.hashes[f"ai:vec:{{{event_id}}}"] = {
        "bad1": "not-a-json",
        "bad2": json.dumps({"corrupted": True}),
    }
    assert await cache.get(event_id, "cau hoi") is None


@pytest.mark.asyncio
async def test_semantic_cache_put_pipeline():
    redis = FakeRedis()
    embedder = FakeEmbedder()
    cache = SemanticCache(redis, embedder)

    event_id = "evt-200"
    await cache.put(event_id, "Gia ve the nao?", "Gia ve tu 500k")

    # Tier 1 exact cache setex
    exact_key = cache.key(event_id, "Gia ve the nao?")
    assert redis.kv[exact_key] == "Gia ve tu 500k"
    assert redis.ttls[exact_key] == TTL_SECONDS

    # Tier 2 hash set & expire via pipeline
    vec_hash = f"ai:vec:{{{event_id}}}"
    assert vec_hash in redis.hashes
    assert exact_key in redis.hashes[vec_hash]
    stored_payload = json.loads(redis.hashes[vec_hash][exact_key])
    assert stored_payload["a"] == "Gia ve tu 500k"
    assert stored_payload["v"] == [1.0, 0.0, 0.0]
    assert redis.ttls[vec_hash] == TTL_SECONDS
