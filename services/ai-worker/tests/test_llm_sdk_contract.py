"""AC-2: moi kwarg LlmClient gui cho messages.create phai co trong chu ky that cua SDK (khong mang)."""

from __future__ import annotations

import inspect

import anthropic.resources.messages as anthropic_messages

from tests.llm.conftest import FakeAnthropic, FakeTools, chat_job, make_client, mod_job, text_block


def _accepted() -> set[str]:
    params = inspect.signature(anthropic_messages.AsyncMessages.create).parameters
    assert not any(p.kind is inspect.Parameter.VAR_KEYWORD for p in params.values())
    return {n for n in params if n != "self"}


async def test_AC2_KwargsChat_CoTrongChuKySdk() -> None:
    sdk = FakeAnthropic()
    c = make_client(sdk)
    prompt = await c.build_prompt(chat_job(), FakeTools())
    await c.generate(prompt, max_output_tokens=512)

    sent = set(sdk.messages.calls[0])
    assert sent, "khong bat duoc kwarg nao"
    assert sent <= _accepted(), f"kwarg SDK khong nhan: {sorted(sent - _accepted())}"


async def test_AC2_KwargsKiemDuyet_CoTrongChuKySdk() -> None:
    sdk = FakeAnthropic(content=[text_block('{"score": 0.1}')])
    await make_client(sdk).scan_moderation(mod_job())

    sent = set(sdk.messages.calls[0])
    assert sent <= _accepted(), f"kwarg SDK khong nhan: {sorted(sent - _accepted())}"


def test_AC2_ConstructorAsyncAnthropic_NhanTimeoutVaMaxRetries() -> None:
    import anthropic

    params = inspect.signature(anthropic.AsyncAnthropic.__init__).parameters
    for name in ("api_key", "timeout", "max_retries"):
        assert name in params
