"""AC-1: che do mock khong goi mang, khong dung SDK nao, giu nguyen hinh dang ket qua."""

from __future__ import annotations

from pathlib import Path

import pytest

from app.domain.moderation import AiVerdict
from app.llm.client import Generation, LlmClient, mock_embedding, wrap_untrusted

from .conftest import FakeTools, chat_job, mod_job

_PROMPTS = Path(__file__).resolve().parents[2] / "app" / "prompts"


@pytest.fixture
def no_sdk(monkeypatch):
    """Tao bat ky client Anthropic/Gemini nao trong che do mock la loi."""
    import anthropic
    from google import genai

    def _boom(*a, **k):
        raise AssertionError("mock mode khong duoc tao client SDK")

    monkeypatch.setattr(anthropic, "AsyncAnthropic", _boom)
    monkeypatch.setattr(genai, "Client", _boom)
    import app.llm.client as mod

    if hasattr(mod, "AsyncAnthropic"):
        monkeypatch.setattr(mod, "AsyncAnthropic", _boom)


async def test_AC1_MockGenerate_HinhDangCu_KhongTaoSdk(no_sdk, fast_sleep) -> None:
    c = LlmClient(mock=True)
    prompt = await c.build_prompt(chat_job("Con ve khong?"), FakeTools())
    gen = await c.generate(prompt, max_output_tokens=512)

    assert isinstance(gen, Generation)
    assert gen.model == "mock"
    assert gen.text
    system = (_PROMPTS / "assistant.v3.md").read_text("utf-8")
    combined_min = len(system) + len(wrap_untrusted("cau_hoi_khach", "Con ve khong?"))
    # total = len(system+user)//4 + 64 tren van ban ghep: it nhat bang phan system + cau hoi.
    assert gen.total_tokens >= combined_min // 4 + 64
    assert gen.total_tokens <= (combined_min + 2000) // 4 + 64


@pytest.mark.parametrize(
    ("title", "verdict", "score"),
    [
        ("Ban vũ khí giá rẻ", AiVerdict.AUTO_REJECTED, 0.9),
        ("Sự kiện nghi vấn bản quyền", AiVerdict.NEEDS_HUMAN, 0.55),
        ("Dem nhac acoustic", AiVerdict.AUTO_APPROVED, 0.1),
    ],
)
async def test_AC1_MockKiemDuyet_TuKhoaGiuNguyen(no_sdk, fast_sleep, title, verdict, score) -> None:
    res = await LlmClient(mock=True).scan_moderation(mod_job(title=title, desc="mo ta"))
    assert res.verdict is verdict
    assert res.score == score


async def test_AC1_MockEmbed_LaMockEmbedding(no_sdk) -> None:
    c = LlmClient(mock=True)
    assert await c.embed("Còn vé không?") == mock_embedding("con ve khong")


def test_AC1_KhongThamSo_DocAI_MOCK(monkeypatch, no_sdk) -> None:
    monkeypatch.setenv("AI_MOCK", "1")
    LlmClient()


def test_AC1_KhongThamSo_KhongBienNao_MacDinhMock(no_sdk) -> None:
    LlmClient()


def test_AC1_KhongThamSo_AliasGEMINI_MOCK(monkeypatch, no_sdk) -> None:
    monkeypatch.setenv("GEMINI_MOCK", "1")
    LlmClient()


def test_AC1_AI_MOCK0_ThieuKey_TuChoi(monkeypatch) -> None:
    monkeypatch.setenv("AI_MOCK", "0")
    with pytest.raises((RuntimeError, ValueError), match="ANTHROPIC_API_KEY"):
        LlmClient()


def test_AC1_AI_MOCKThangAlias(monkeypatch, no_sdk) -> None:
    # AI_MOCK=1 thang GEMINI_MOCK=0: van mock, khong doi key.
    monkeypatch.setenv("AI_MOCK", "1")
    monkeypatch.setenv("GEMINI_MOCK", "0")
    LlmClient()
