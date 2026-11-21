"""AC-14/AC-15: Settings cho Anthropic -- validate luc khoi dong, alias cu, khong lo bi mat."""

from __future__ import annotations

import pytest
from pydantic import SecretStr, ValidationError

from app.config import Settings

from .conftest import ANTHROPIC_KEY, GEMINI_KEY


def _s(**kw) -> Settings:
    return Settings(_env_file=None, **kw)


def test_AC14_MacDinh() -> None:
    s = _s()
    assert s.ai_mock is True
    assert s.anthropic_model_chat == "claude-haiku-5-5"
    assert s.anthropic_model_moderation == "claude-haiku-5-5"
    assert s.ai_timeout_seconds == 10
    assert s.ai_rpm == 55
    assert s.ai_tpm == 900_000
    assert s.ai_thinking_headroom_tokens == 1024
    assert s.gemini_embed_model == "text-embedding-004"


def test_AC14_ThatMaThieuKeyAnthropic_TuChoi_NeuTenBien(monkeypatch) -> None:
    monkeypatch.setenv("AI_MOCK", "0")
    with pytest.raises(ValidationError, match="ANTHROPIC_API_KEY"):
        _s()


def test_AC14_ThatCoKeyAnthropic_KhongKeyGemini_HopLe(monkeypatch) -> None:
    monkeypatch.setenv("AI_MOCK", "0")
    monkeypatch.setenv("ANTHROPIC_API_KEY", ANTHROPIC_KEY)
    s = _s()
    assert s.ai_mock is False
    assert s.anthropic_api_key.get_secret_value() == ANTHROPIC_KEY


@pytest.mark.parametrize("var", ["ANTHROPIC_MODEL_CHAT", "ANTHROPIC_MODEL_MODERATION"])
def test_AC14_TenModelRong_TuChoi(monkeypatch, var) -> None:
    monkeypatch.setenv(var, "")
    with pytest.raises(ValidationError):
        _s()


@pytest.mark.parametrize(
    ("var", "value"),
    [
        ("AI_TIMEOUT_SECONDS", "0"),
        ("AI_TIMEOUT_SECONDS", "-1"),
        ("AI_RPM", "0"),
        ("AI_TPM", "0"),
        ("AI_THINKING_HEADROOM_TOKENS", "-1"),
    ],
)
def test_AC14_GiaTriSo_KhongHopLe_TuChoi(monkeypatch, var, value) -> None:
    monkeypatch.setenv(var, value)
    with pytest.raises(ValidationError):
        _s()


def test_AC14_Headroom0_HopLe(monkeypatch) -> None:
    monkeypatch.setenv("AI_THINKING_HEADROOM_TOKENS", "0")
    assert _s().ai_thinking_headroom_tokens == 0


def test_AC14_EnvMoi_DocDung(monkeypatch) -> None:
    monkeypatch.setenv("AI_MOCK", "0")
    monkeypatch.setenv("ANTHROPIC_API_KEY", ANTHROPIC_KEY)
    monkeypatch.setenv("ANTHROPIC_MODEL_CHAT", "claude-a")
    monkeypatch.setenv("ANTHROPIC_MODEL_MODERATION", "claude-b")
    monkeypatch.setenv("AI_TIMEOUT_SECONDS", "7")
    monkeypatch.setenv("AI_RPM", "40")
    monkeypatch.setenv("AI_TPM", "400000")
    monkeypatch.setenv("AI_THINKING_HEADROOM_TOKENS", "256")
    s = _s()
    assert (s.anthropic_model_chat, s.anthropic_model_moderation) == ("claude-a", "claude-b")
    assert (s.ai_timeout_seconds, s.ai_rpm, s.ai_tpm, s.ai_thinking_headroom_tokens) == (7, 40, 400_000, 256)


# ---------------------------------------------------------------- alias (C1/C2)


def test_AC14_AliasGEMINI_MOCK0_ThieuKey_TuChoi(monkeypatch) -> None:
    monkeypatch.setenv("GEMINI_MOCK", "0")
    with pytest.raises(ValidationError, match="ANTHROPIC_API_KEY"):
        _s()


def test_AC14_AliasGEMINI_MOCK0_CoKey_TatMock(monkeypatch) -> None:
    monkeypatch.setenv("GEMINI_MOCK", "0")
    monkeypatch.setenv("ANTHROPIC_API_KEY", ANTHROPIC_KEY)
    assert _s().ai_mock is False


def test_AC14_AI_MOCKThangAlias(monkeypatch) -> None:
    monkeypatch.setenv("AI_MOCK", "1")
    monkeypatch.setenv("GEMINI_MOCK", "0")
    assert _s().ai_mock is True


@pytest.mark.parametrize(
    ("alias", "new", "field", "value"),
    [
        ("GEMINI_TIMEOUT_SECONDS", "AI_TIMEOUT_SECONDS", "ai_timeout_seconds", 5),
        ("GEMINI_RPM", "AI_RPM", "ai_rpm", 30),
        ("GEMINI_TPM", "AI_TPM", "ai_tpm", 300_000),
    ],
)
def test_AC14_AliasSo_DuocDoc_VaTenMoiThang(monkeypatch, alias, new, field, value) -> None:
    monkeypatch.setenv(alias, str(value))
    assert getattr(_s(), field) == value

    monkeypatch.setenv(new, str(value * 2))
    assert getattr(_s(), field) == value * 2


def test_AC14_BienCuDaBo_BiBoQua(monkeypatch) -> None:
    monkeypatch.setenv("GEMINI_MODEL_FAST", "gemini-2.0-flash")
    monkeypatch.setenv("GEMINI_MODEL_DEEP", "gemini-2.5-pro")
    _s()


# ---------------------------------------------------------------- AC-15 secrets


def test_AC15_KeyLaSecretStr_ReprKhongLo(monkeypatch) -> None:
    monkeypatch.setenv("AI_MOCK", "0")
    monkeypatch.setenv("ANTHROPIC_API_KEY", ANTHROPIC_KEY)
    monkeypatch.setenv("GEMINI_API_KEY", GEMINI_KEY)
    s = _s()
    assert isinstance(s.anthropic_api_key, SecretStr)
    assert isinstance(s.gemini_api_key, SecretStr)
    assert s.gemini_api_key.get_secret_value() == GEMINI_KEY
    for text in (repr(s), str(s), str(s.model_dump())):
        assert ANTHROPIC_KEY not in text
        assert GEMINI_KEY not in text


def test_AC15_LoiValidate_KhongLoKey(monkeypatch) -> None:
    monkeypatch.setenv("AI_MOCK", "0")
    monkeypatch.setenv("ANTHROPIC_API_KEY", ANTHROPIC_KEY)
    monkeypatch.setenv("AI_RPM", "0")
    with pytest.raises(ValidationError) as ei:
        _s()
    assert ANTHROPIC_KEY not in str(ei.value)
