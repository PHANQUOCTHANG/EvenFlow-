"""Fakes chung cho test ai-anthropic: SDK Anthropic/Gemini gia, khong goi mang, khong can key.

Hop dong giao dien ma cac test nay ap dat (spec ai-anthropic, C3/AC-2/AC-12):
  LlmClient(api_key=..., *, mock=..., sdk=<AsyncAnthropic-like>, chat_model=..., moderation_model=...,
            timeout_seconds=..., thinking_headroom_tokens=..., gemini_api_key=..., embed_model=...,
            embed_sdk=<genai.Client-like>)
  sdk.messages.create(**kwargs) -> resp(content=[blocks], usage, stop_reason)
  embed_sdk.aio.models.embed_content(model=..., contents=...) -> resp.embeddings[0].values
"""

from __future__ import annotations

import asyncio
import json
import logging
from types import SimpleNamespace
from typing import Any

import httpx
import pytest

ANTHROPIC_KEY = "sk-ant-TEST-SECRET-0001"
GEMINI_KEY = "gemini-TEST-SECRET-0002"
CHAT_MODEL = "claude-chat-test"
MOD_MODEL = "claude-mod-test"
HEADROOM = 1024

# Dat cac bien nay ve trang thai sach de test khong phu thuoc may dev / CI.
_ENV_VARS = (
    "AI_MOCK", "GEMINI_MOCK", "ANTHROPIC_API_KEY", "ANTHROPIC_MODEL_CHAT", "ANTHROPIC_MODEL_MODERATION",
    "AI_TIMEOUT_SECONDS", "GEMINI_TIMEOUT_SECONDS", "AI_RPM", "AI_TPM", "GEMINI_RPM", "GEMINI_TPM",
    "AI_THINKING_HEADROOM_TOKENS", "GEMINI_API_KEY", "GEMINI_EMBED_MODEL", "GEMINI_MODEL_FAST",
    "GEMINI_MODEL_DEEP",
)


@pytest.fixture(autouse=True)
def _clean_env(monkeypatch):
    for name in _ENV_VARS:
        monkeypatch.delenv(name, raising=False)


@pytest.fixture
def fast_sleep(monkeypatch):
    """Mock mode ngu 0.3-1.2s de gia lap do tre; test khong can doi."""
    orig = asyncio.sleep

    async def _fast(_delay=0, *a, **k):
        await orig(0)

    monkeypatch.setattr(asyncio, "sleep", _fast)


def text_block(text: str) -> SimpleNamespace:
    return SimpleNamespace(type="text", text=text)


def thinking_block(text: str = "suy nghi noi bo") -> SimpleNamespace:
    return SimpleNamespace(type="thinking", thinking=text, signature="sig")


_DEFAULT = object()


class FakeMessages:
    def __init__(self, *, content: Any = _DEFAULT, usage: Any = _DEFAULT, stop_reason: str = "end_turn",
                 delay: float = 0.0, exc: BaseException | None = None) -> None:
        self.calls: list[dict] = []
        self._content = [text_block("tra loi")] if content is _DEFAULT else content
        self._usage = SimpleNamespace(input_tokens=100, output_tokens=23) if usage is _DEFAULT else usage
        self._stop = stop_reason
        self._delay = delay
        self._exc = exc

    async def create(self, **kwargs: Any) -> SimpleNamespace:
        self.calls.append(kwargs)
        if self._delay:
            await asyncio.sleep(self._delay)
        if self._exc is not None:
            raise self._exc
        return SimpleNamespace(content=self._content, usage=self._usage, stop_reason=self._stop)


class FakeAnthropic:
    def __init__(self, **kw: Any) -> None:
        self.messages = FakeMessages(**kw)


class FakeGenaiModels:
    def __init__(self, *, delay: float = 0.0) -> None:
        self.calls: list[dict] = []
        self._delay = delay

    async def embed_content(self, *, model, contents):
        self.calls.append({"model": model, "contents": contents})
        if self._delay:
            await asyncio.sleep(self._delay)
        return SimpleNamespace(embeddings=[SimpleNamespace(values=[0.1, 0.2, 0.3])])

    async def generate_content(self, **kw):  # duong sinh van ban KHONG duoc dung Gemini nua (D1)
        raise AssertionError("generate_content cua Gemini khong duoc goi sau khi chuyen sang Anthropic")


def fake_genai(**kw: Any) -> SimpleNamespace:
    return SimpleNamespace(aio=SimpleNamespace(models=FakeGenaiModels(**kw)))


def make_client(sdk: Any, **kw: Any):
    from app.llm.client import LlmClient

    args: dict[str, Any] = {
        "mock": False,
        "sdk": sdk,
        "chat_model": CHAT_MODEL,
        "moderation_model": MOD_MODEL,
        "timeout_seconds": 5.0,
        "thinking_headroom_tokens": HEADROOM,
        "gemini_api_key": "",
    }
    args.update(kw)
    return LlmClient(ANTHROPIC_KEY, **args)


class FakeTools:
    def __init__(self, ctx: str = "Su kien: Concert A, 20:00, con 120 ve.") -> None:
        self.ctx = ctx

    async def event_context(self, event_id: str) -> str:
        return self.ctx


def chat_job(question: str = "Con ve khong?", event_id: str = "ev-1") -> SimpleNamespace:
    return SimpleNamespace(job_id="j-1", event_id=event_id, identity_id="u-1", question=question,
                           queue_token="qt", enqueued_at=0.0)


def mod_job(title: str = "Concert A", desc: str = "Dem nhac ngoai troi") -> Any:
    from app.domain.moderation import ModerationJob

    return ModerationJob(job_id="m-1", event_id="ev-9", event_title=title, event_description=desc,
                         enqueued_at=0.0)


def mod_body(title: str = "Concert A", desc: str = "Dem nhac ngoai troi") -> bytes:
    return json.dumps({"job_id": "m-1", "event_id": "ev-9", "event_title": title,
                       "event_description": desc, "enqueued_at": 0.0}).encode()


def user_text(call: dict) -> str:
    """Gom toan bo text cua messages (content la str hoac list block)."""
    parts: list[str] = []
    for m in call["messages"]:
        c = m["content"]
        if isinstance(c, str):
            parts.append(c)
        else:
            for b in c:
                parts.append(b["text"] if isinstance(b, dict) else b.text)
    return "\n".join(parts)


def system_text(call: dict) -> str:
    s = call["system"]
    if isinstance(s, str):
        return s
    return "".join(b["text"] if isinstance(b, dict) else b.text for b in s)


class FakeLimiter:
    def __init__(self) -> None:
        self.acquired: list[int] = []
        self.refunds: list[tuple[int, int]] = []

    async def acquire(self, est: int):
        from app.llm.limiter import Permit

        self.acquired.append(est)
        return Permit(True, 0, est)

    async def refund(self, reserved: int, actual: int) -> None:
        self.refunds.append((reserved, actual))


def http_request() -> httpx.Request:
    return httpx.Request("POST", "https://api.anthropic.com/v1/messages")


def status_error(cls: type, status: int, message: str = "loi"):
    resp = httpx.Response(status, request=http_request())
    return cls(message, response=resp, body=None)


def all_log_text(records: list[logging.LogRecord]) -> str:
    fmt = logging.Formatter("%(message)s")
    out = []
    for r in records:
        out.append(fmt.format(r))
        out.append(str({k: v for k, v in r.__dict__.items() if k not in ("exc_info", "exc_text")}))
    return "\n".join(out)
