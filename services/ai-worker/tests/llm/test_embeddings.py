"""AC-12/AC-13: embedding cache ngu nghia (L3) van o Gemini khi co key; khong key thi tat L3 gon."""

from __future__ import annotations

import importlib
import json
import logging

import pytest

from app.pipeline.semantic_cache import SemanticCache

from .conftest import GEMINI_KEY, FakeAnthropic, FakeTools, chat_job, fake_genai, make_client


def test_AC12_ModuleEmbedderTonTai() -> None:
    importlib.import_module("app.llm.embedder")


async def test_AC12_CoKeyGemini_EmbedQuaGeminiDungModel() -> None:
    genai_sdk = fake_genai()
    c = make_client(FakeAnthropic(), gemini_api_key=GEMINI_KEY, embed_model="text-embedding-test",
                    embed_sdk=genai_sdk)

    vec = await c.embed("con ve khong")

    assert vec == [0.1, 0.2, 0.3]
    assert genai_sdk.aio.models.calls == [{"model": "text-embedding-test", "contents": "con ve khong"}]


async def test_AC12_EmbedCungTimeout() -> None:
    c = make_client(FakeAnthropic(), gemini_api_key=GEMINI_KEY, embed_model="text-embedding-test",
                    embed_sdk=fake_genai(delay=1.0), timeout_seconds=0.05)
    with pytest.raises(TimeoutError):
        await c.embed("con ve khong")


async def test_AC12_DuongSinhVanBan_KhongChamGemini() -> None:
    # fake_genai.generate_content nem AssertionError neu bi goi (D1).
    sdk = FakeAnthropic()
    c = make_client(sdk, gemini_api_key=GEMINI_KEY, embed_sdk=fake_genai())
    prompt = await c.build_prompt(chat_job(), FakeTools())
    await c.generate(prompt, max_output_tokens=512)
    assert len(sdk.messages.calls) == 1


async def test_AC13_KhongKeyGemini_KhoiDongDuoc_KhongTaoClientGemini(monkeypatch) -> None:
    from google import genai

    def _boom(*a, **k):
        raise AssertionError("khong duoc tao genai.Client khi GEMINI_API_KEY rong")

    monkeypatch.setattr(genai, "Client", _boom)
    sdk = FakeAnthropic()
    c = make_client(sdk, gemini_api_key="")
    prompt = await c.build_prompt(chat_job(), FakeTools())
    await c.generate(prompt, max_output_tokens=512)
    assert len(sdk.messages.calls) == 1


# ---------------------------------------------------------------- SemanticCache khi L3 tat


class FakePipe:
    def __init__(self, redis: FakeRedis) -> None:
        self._r = redis

    async def __aenter__(self):
        return self

    async def __aexit__(self, *a):
        return False

    def hset(self, name, key, value):
        self._r.hsets.append((name, key, value))

    def expire(self, *a):
        pass

    async def execute(self):
        return []


class FakeRedis:
    def __init__(self) -> None:
        self.kv: dict[str, str] = {}
        self.hsets: list[tuple] = []

    async def get(self, key):
        return self.kv.get(key)

    async def setex(self, key, ttl, value):
        self.kv[key] = value

    async def hgetall(self, name):
        return {}

    def pipeline(self):
        return FakePipe(self)


async def test_AC13_CacheKhongEmbedder_L1VanChay_KhongLog(caplog) -> None:
    redis = FakeRedis()
    cache = SemanticCache(redis, None)
    caplog.set_level(logging.DEBUG)

    await cache.put("ev-1", "Con ve khong?", "Con 120 ve.")
    assert redis.kv[cache.key("ev-1", "Con ve khong?")] == "Con 120 ve."
    assert redis.hsets == []  # khong ghi vector

    assert await cache.get("ev-1", "còn vé không") == "Con 120 ve."  # L1 hit (chuan hoa)

    for i in range(100):
        assert await cache.get("ev-1", f"cau hoi moi so {i}") is None
        await cache.put("ev-1", f"cau hoi moi so {i}", "x")

    embed_logs = [r for r in caplog.records if "embed" in r.getMessage().lower()]
    assert embed_logs == []
    assert all(r.levelno < logging.WARNING for r in caplog.records)
    assert len(redis.hsets) == 0
    json.dumps(redis.kv)  # chi chuoi
