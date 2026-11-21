"""LlmClient that qua SDK Anthropic duoc tiem vao -- khong goi mang (AC-7)."""

from __future__ import annotations

import asyncio
from types import SimpleNamespace

import anthropic
import httpx
import pytest
from prometheus_client import REGISTRY

from app.llm.client import LlmClient, Prompt, RateLimited, mock_embedding


class FakeMessages:
    def __init__(self, *, text="xin chao", usage=(100, 23), delay=0.0, raise_exc=None) -> None:
        self.calls: list[dict] = []
        self._text, self._usage, self._delay, self._exc = text, usage, delay, raise_exc

    async def create(self, **kw):
        self.calls.append(kw)
        if self._delay:
            await asyncio.sleep(self._delay)
        if self._exc is not None:
            raise self._exc
        usage = (
            None
            if self._usage is None
            else SimpleNamespace(input_tokens=self._usage[0], output_tokens=self._usage[1])
        )
        return SimpleNamespace(
            content=[SimpleNamespace(type="text", text=self._text)],
            usage=usage,
            stop_reason="end_turn",
        )


class FakeModels:
    """Chi con embed_content: embedding van di Gemini."""

    def __init__(self) -> None:
        self.calls: list[dict] = []

    async def embed_content(self, *, model, contents):
        self.calls.append({"embed_model": model, "contents": contents})
        return SimpleNamespace(embeddings=[SimpleNamespace(values=[0.1, 0.2, 0.3])])


_PROMPT = Prompt(system="s", user="prompt")


def _client(messages: FakeMessages, **kw) -> LlmClient:
    return LlmClient(
        "k",
        mock=False,
        sdk=SimpleNamespace(messages=messages),
        chat_model="claude-test",
        thinking_headroom_tokens=0,
        gemini_api_key="",
        **kw,
    )


def _status_error(cls, status: int, message: str):
    req = httpx.Request("POST", "https://api.anthropic.com/v1/messages")
    return cls(message, response=httpx.Response(status, request=req), body=None)


async def test_Generate_TraTokenThatVaGioiHanDauRa() -> None:
    messages = FakeMessages(text="tra loi", usage=(300, 21))
    gen = await _client(messages).generate(_PROMPT, max_output_tokens=192)

    assert gen.text == "tra loi"
    assert gen.total_tokens == 321
    assert gen.model == "claude-test"
    call = messages.calls[0]
    assert call["model"] == "claude-test"
    assert call["messages"] == [{"role": "user", "content": "prompt"}]
    assert call["max_tokens"] == 192


async def test_Generate_ThieuUsageThiTokenLaNone() -> None:
    # None = khong biet -> nguoi goi giu nguyen so da dat cho, khong hoan nham.
    gen = await _client(FakeMessages(usage=None)).generate(_PROMPT, max_output_tokens=10)
    assert gen.total_tokens is None


async def test_Generate_QuaTimeoutThiLoi() -> None:
    with pytest.raises(TimeoutError):
        await _client(FakeMessages(delay=1.0), timeout_seconds=0.05).generate(
            _PROMPT, max_output_tokens=10
        )


async def test_Generate_429ThanhRateLimitedVaTangCounter() -> None:
    before = REGISTRY.get_sample_value("ai_llm_429_total") or 0.0
    exc = _status_error(anthropic.RateLimitError, 429, "quota")

    with pytest.raises(RateLimited):
        await _client(FakeMessages(raise_exc=exc)).generate(_PROMPT, max_output_tokens=10)

    assert REGISTRY.get_sample_value("ai_llm_429_total") == before + 1


async def test_Generate_Loi4xxKhacKhongPhaiRateLimited() -> None:
    exc = _status_error(anthropic.BadRequestError, 400, "bad")
    with pytest.raises(anthropic.BadRequestError):
        await _client(FakeMessages(raise_exc=exc)).generate(_PROMPT, max_output_tokens=10)


async def test_Embed_That() -> None:
    models = FakeModels()
    c = LlmClient(
        "k",
        mock=False,
        sdk=SimpleNamespace(messages=FakeMessages()),
        gemini_api_key="g",
        embed_model="text-embedding-test",
        embed_sdk=SimpleNamespace(aio=SimpleNamespace(models=models)),
    )
    vec = await c.embed("con ve khong")
    assert vec == [0.1, 0.2, 0.3]
    assert models.calls[0] == {"embed_model": "text-embedding-test", "contents": "con ve khong"}


async def test_Embed_MockTatDinhVaBoDau() -> None:
    c = LlmClient(mock=True)
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
    with pytest.raises(RuntimeError, match="ANTHROPIC_API_KEY"):
        LlmClient(api_key="", mock=False)


def test_KhongThamSo_VanDungDuocOCheDoMock(monkeypatch) -> None:
    # Hop dong cu cua test kiem duyet: LlmClient() doc GEMINI_MOCK tu env.
    monkeypatch.setenv("GEMINI_MOCK", "1")
    LlmClient()
