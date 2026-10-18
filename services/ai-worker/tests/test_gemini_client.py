"""Gemini that qua SDK duoc tiem vao -- khong goi mang (AC-7)."""

from __future__ import annotations

import asyncio
from types import SimpleNamespace

import pytest
from google.genai import errors
from prometheus_client import REGISTRY

from app.gemini.client import GeminiClient, RateLimited, mock_embedding


class FakeModels:
    def __init__(self, *, text="xin chao", total=123, delay=0.0, raise_exc=None) -> None:
        self.calls: list[dict] = []
        self._text, self._total, self._delay, self._exc = text, total, delay, raise_exc

    async def generate_content(self, *, model, contents, config):
        self.calls.append({"model": model, "contents": contents, "config": config})
        if self._delay:
            await asyncio.sleep(self._delay)
        if self._exc is not None:
            raise self._exc
        usage = None if self._total is None else SimpleNamespace(total_token_count=self._total)
        return SimpleNamespace(text=self._text, usage_metadata=usage)

    async def embed_content(self, *, model, contents):
        self.calls.append({"embed_model": model, "contents": contents})
        return SimpleNamespace(embeddings=[SimpleNamespace(values=[0.1, 0.2, 0.3])])


def _client(models: FakeModels, **kw) -> GeminiClient:
    sdk = SimpleNamespace(aio=SimpleNamespace(models=models))
    return GeminiClient(api_key="k", model="gemini-test", mock=False, sdk=sdk, **kw)


async def test_Generate_TraTokenThatVaGioiHanDauRa() -> None:
    models = FakeModels(text="tra loi", total=321)
    gen = await _client(models).generate("prompt", max_output_tokens=192)

    assert gen.text == "tra loi"
    assert gen.total_tokens == 321
    assert gen.model == "gemini-test"
    call = models.calls[0]
    assert call["model"] == "gemini-test"
    assert call["contents"] == "prompt"
    assert call["config"].max_output_tokens == 192


async def test_Generate_ThieuUsageThiTokenLaNone() -> None:
    # None = khong biet -> nguoi goi giu nguyen so da dat cho, khong hoan nham.
    gen = await _client(FakeModels(total=None)).generate("p", max_output_tokens=10)
    assert gen.total_tokens is None


async def test_Generate_QuaTimeoutThiLoi() -> None:
    with pytest.raises(TimeoutError):
        await _client(FakeModels(delay=1.0), timeout_seconds=0.05).generate("p", max_output_tokens=10)


async def test_Generate_429ThanhRateLimitedVaTangCounter() -> None:
    before = REGISTRY.get_sample_value("ai_gemini_429_total") or 0.0
    exc = errors.ClientError(429, {"error": {"code": 429, "message": "quota", "status": "RESOURCE_EXHAUSTED"}})

    with pytest.raises(RateLimited):
        await _client(FakeModels(raise_exc=exc)).generate("p", max_output_tokens=10)

    assert REGISTRY.get_sample_value("ai_gemini_429_total") == before + 1


async def test_Generate_Loi4xxKhacKhongPhaiRateLimited() -> None:
    exc = errors.ClientError(400, {"error": {"code": 400, "message": "bad", "status": "INVALID_ARGUMENT"}})
    with pytest.raises(errors.ClientError):
        await _client(FakeModels(raise_exc=exc)).generate("p", max_output_tokens=10)


async def test_Embed_That() -> None:
    models = FakeModels()
    vec = await _client(models, embed_model="text-embedding-test").embed("con ve khong")
    assert vec == [0.1, 0.2, 0.3]
    assert models.calls[0] == {"embed_model": "text-embedding-test", "contents": "con ve khong"}


async def test_Embed_MockTatDinhVaBoDau() -> None:
    c = GeminiClient(mock=True)
    a = await c.embed("Còn vé không?")
    b = await c.embed("con ve khong")
    assert a == b
    assert len(a) == 64
    assert abs(sum(x * x for x in a) - 1.0) < 1e-9


def test_mock_embedding_CauKhacNhauThiVectorKhac() -> None:
    assert mock_embedding("con ve khong") != mock_embedding("bao gio toi luot")


def test_mock_embedding_CauRong() -> None:
    assert mock_embedding("") == [0.0] * 64


def test_ThatMaThieuKey_TuChoi() -> None:
    with pytest.raises(RuntimeError, match="GEMINI_API_KEY"):
        GeminiClient(api_key="", mock=False)


def test_KhongThamSo_VanDungDuocOCheDoMock(monkeypatch) -> None:
    # Hop dong cu cua test kiem duyet: GeminiClient() doc GEMINI_MOCK tu env.
    monkeypatch.setenv("GEMINI_MOCK", "1")
    GeminiClient()
