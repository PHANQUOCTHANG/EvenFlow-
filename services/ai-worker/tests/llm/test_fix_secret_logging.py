"""Fix X-1: key khong bao gio vao log -- ke ca key co \\n cuoi va chuoi traceback cua SDK.

Case chinh lay tu repro cua challenger (test_key_newline_leak): SDK Anthropic that, may chu
cuc bo dong ket noi, key co \\n cuoi.
"""

from __future__ import annotations

import asyncio
import logging
import time
import traceback

import anthropic
import httpx2
import pytest
from pydantic import ValidationError

from app.config import Settings, clean_api_key
from app.consumers.chat import ChatConsumer
from app.llm.client import LlmClient, Prompt
from app.pipeline.semantic_cache import SemanticCache

from .conftest import (
    ANTHROPIC_KEY,
    GEMINI_KEY,
    FakeAnthropic,
    FakeLimiter,
    FakeTools,
    http_request,
    make_client,
)

KEY = "sk-ant-CHALLENGE-SECRET-777"


def _full_log(records: list[logging.LogRecord]) -> str:
    """Moi thu mot formatter co the in: message, extra va ca traceback."""
    fmt = logging.Formatter("%(message)s")
    out = []
    for r in records:
        out.append(fmt.format(r))
        out.append(str(r.__dict__))
        if r.exc_info:
            out.append("".join(traceback.format_exception(*r.exc_info)))
    return "\n".join(out)


# ---------------------------------------------------------------- startup validation


@pytest.mark.parametrize("field", ["anthropic_api_key", "gemini_api_key"])
@pytest.mark.parametrize("raw", [KEY + "\n", " " + KEY + " ", "\t" + KEY + "\r\n"])
def test_X1_KeyKhoangTrangBaoNgoai_DuocCatMotLan(field, raw) -> None:
    kw = {"anthropic_api_key": ANTHROPIC_KEY, field: raw}
    s = Settings(_env_file=None, ai_mock=False, **kw)
    assert getattr(s, field).get_secret_value() == KEY


@pytest.mark.parametrize("field", ["anthropic_api_key", "gemini_api_key"])
@pytest.mark.parametrize("bad", ["sk-ant CHALLENGE", "sk-ant\nCHALLENGE", "sk-ant\x00CHALLENGE", "sk-ant\x7fX",
                                 "sk-ant-éX", "sk-ant\u200bX"])
def test_X1_KeyCoKhoangTrangHoacKyTuDieuKhienBenTrong_TuChoi_KhongInKey(field, bad) -> None:
    kw = {"anthropic_api_key": ANTHROPIC_KEY, field: bad}
    with pytest.raises(ValidationError) as ei:
        Settings(_env_file=None, ai_mock=False, **kw)
    text = str(ei.value) + repr(ei.value)
    assert field.upper() in text
    assert bad not in text and "CHALLENGE" not in text


def test_X1_KeyChiCoKhoangTrang_LaThieuKey(monkeypatch) -> None:
    with pytest.raises(ValidationError, match="ANTHROPIC_API_KEY"):
        Settings(_env_file=None, ai_mock=False, anthropic_api_key=" \n")


def test_X1_KeyTuEnv_CoNewlineCuoi_DuocCat(monkeypatch) -> None:
    monkeypatch.setenv("AI_MOCK", "0")
    monkeypatch.setenv("ANTHROPIC_API_KEY", KEY + "\n")
    monkeypatch.setenv("GEMINI_API_KEY", GEMINI_KEY + "\n")
    s = Settings(_env_file=None)
    assert s.anthropic_api_key.get_secret_value() == KEY
    assert s.gemini_api_key.get_secret_value() == GEMINI_KEY


def test_X1_CleanApiKey_LoiKhongChuaGiaTri() -> None:
    with pytest.raises(ValueError) as ei:
        clean_api_key("ANTHROPIC_API_KEY", "abc def-SECRET")
    assert "SECRET" not in str(ei.value)


def test_X1_LlmClient_CatKeyVaTuChoiKeyBan() -> None:
    sdk = FakeAnthropic()
    assert LlmClient(KEY + "\n", mock=False, sdk=sdk, gemini_api_key="")._api_key == KEY
    with pytest.raises(RuntimeError) as ei:
        LlmClient("sk ant SECRET", mock=False, sdk=sdk, gemini_api_key="")
    assert "SECRET" not in str(ei.value) and ei.value.__cause__ is None
    with pytest.raises(RuntimeError) as ei:
        LlmClient(KEY, mock=False, sdk=sdk, gemini_api_key="g k SECRET")
    assert "SECRET" not in str(ei.value)


# ---------------------------------------------------------------- real SDK: header carries the stripped key


async def test_X1_SdkThat_KeyCoNewline_GuiHeaderDaCat() -> None:
    seen: dict = {}

    def handler(req):
        seen.update(dict(req.headers))
        return httpx2.Response(200, json={
            "id": "m", "type": "message", "role": "assistant", "model": "claude-haiku-5-5",
            "content": [{"type": "text", "text": "ok"}], "stop_reason": "end_turn", "stop_sequence": None,
            "usage": {"input_tokens": 1, "output_tokens": 1}})

    c = LlmClient(KEY + "\n", mock=False, gemini_api_key="")
    c._sdk = anthropic.AsyncAnthropic(api_key=c._api_key, max_retries=0, timeout=5,
                                      http_client=httpx2.AsyncClient(transport=httpx2.MockTransport(handler)))
    await c.generate(Prompt("s", "u"), max_output_tokens=16)
    # SDK 2.x gui key qua Authorization: Bearer (ban cu: x-api-key) -- dung bang key da cat.
    sent = seen.get("x-api-key") or seen["authorization"].removeprefix("Bearer ")
    assert sent == KEY


async def test_X1_ReproChallenger_KeyNewline_KhongVaoLog(monkeypatch, caplog) -> None:
    """Repro challenger: may chu dong ket noi, chat consumer log loi -- key khong duoc lo."""

    async def serve(r, w):
        await r.read(100)
        w.close()

    srv = await asyncio.start_server(serve, "127.0.0.1", 0)
    port = srv.sockets[0].getsockname()[1]
    monkeypatch.setenv("ANTHROPIC_BASE_URL", f"http://127.0.0.1:{port}")
    try:
        llm = LlmClient(KEY + "\n", mock=False, timeout_seconds=3, gemini_api_key="")
        consumer, out = _chat_consumer(llm)
        caplog.set_level(logging.DEBUG)
        await consumer.handle(_raw())
    finally:
        srv.close()
    assert out and out[0]["source"] == "fallback"
    assert KEY not in _full_log(caplog.records)
    assert KEY not in caplog.text


# ---------------------------------------------------------------- chat / cache never log a traceback chain


class _Cache:
    async def get(self, event_id, question):
        return None

    async def put(self, *a):
        pass

    def key(self, event_id, question):
        return f"k:{event_id}"


class _Flight:
    async def acquire(self, key):
        return True

    async def wait(self, key, timeout):
        return None

    async def publish(self, key, answer):
        pass

    async def release(self, key):
        pass


class _Faq:
    async def match(self, *a):
        return None

    def fallback(self, notice):
        return "FALLBACK"


def _raw() -> bytes:
    import json

    return json.dumps({"job_id": "j", "event_id": "ev", "identity_id": "u", "question": "Con ve?",
                       "queue_token": "qt", "enqueued_at": time.time()}).encode()


def _chat_consumer(llm):
    from app.pipeline import degradation

    out: list[dict] = []

    async def publish(job_id, payload):
        out.append(payload)

    def signals():
        return degradation.Signals(chat_queue_lag_seconds=0.0, budget_used_ratio=0.0, gemini_circuit_open=False)

    return ChatConsumer(FakeLimiter(), _Cache(), _Faq(), _Flight(), llm, FakeTools(), signals, publish), out


def _leaky_connection_error() -> anthropic.APIConnectionError:
    exc = anthropic.APIConnectionError(request=http_request())
    exc.__cause__ = ValueError(f"Illegal header value b'Bearer {KEY}\\n'")
    return exc


async def test_X1_Chat_LoiCoChuoiChuaKey_LogChiTenLoi(caplog) -> None:
    consumer, out = _chat_consumer(make_client(FakeAnthropic(exc=_leaky_connection_error())))
    caplog.set_level(logging.DEBUG)
    await consumer.handle(_raw())
    assert out[0]["source"] == "fallback"
    rec = next(r for r in caplog.records if r.getMessage() == "goi llm that bai")
    assert rec.exc_info is None
    assert rec.error_type == "APIConnectionError"
    assert KEY not in _full_log(caplog.records)


async def test_X1_Chat_LoiHttp_LogMaTrangThai(caplog) -> None:
    resp = __import__("httpx").Response(500, request=http_request())
    exc = anthropic.InternalServerError("loi " + KEY, response=resp, body=None)
    consumer, _ = _chat_consumer(make_client(FakeAnthropic(exc=exc)))
    caplog.set_level(logging.DEBUG)
    await consumer.handle(_raw())
    rec = next(r for r in caplog.records if r.getMessage() == "goi llm that bai")
    assert rec.status_code == 500
    assert KEY not in _full_log(caplog.records)


class _Redis:
    async def get(self, key):
        return None

    async def setex(self, *a):
        pass


class _LeakyEmbedder:
    async def embed(self, text):
        exc = RuntimeError("embed loi")
        exc.__cause__ = ValueError(f"x-goog-api-key: {GEMINI_KEY}")
        raise RuntimeError(f"wrapped {GEMINI_KEY}") from exc


async def test_X1_CacheNguNghia_EmbedLoi_KhongLogKey(caplog) -> None:
    cache = SemanticCache(_Redis(), _LeakyEmbedder())
    caplog.set_level(logging.DEBUG)
    assert await cache.get("ev", "Con ve?") is None
    await cache.put("ev", "Con ve?", "tra loi")
    msgs = [r for r in caplog.records if "embed that bai" in r.getMessage()]
    assert len(msgs) == 2
    assert all(r.exc_info is None and r.error_type == "RuntimeError" for r in msgs)
    assert GEMINI_KEY not in _full_log(caplog.records)
