"""AC-13/AC-15: lap rap LLM + log khoi dong la ham thuan, test khong can Redis/RabbitMQ.

Hop dong: app.llm.wiring.build_llm(settings) -> (llm, embedder) voi embedder=None khi L3 tat;
app.llm.wiring.log_startup(settings, embedder, deprecated=[ten bien cu da dung]).
"""

from __future__ import annotations

import logging

import pytest

from app.config import Settings
from app.llm import wiring

ANTHROPIC_KEY = "sk-ant-WIRING-SECRET-1111"
GEMINI_KEY = "gemini-WIRING-SECRET-2222"
L3_OFF = "semantic cache L3 disabled (GEMINI_API_KEY empty)"


_MP = pytest.MonkeyPatch()


@pytest.fixture(autouse=True)
def _undo_env():
    yield
    _MP.undo()


def _settings(**kw) -> Settings:
    # Qua bien moi truong (ten trong spec), khong qua kwarg: alias (C1/C2) khong phu thuoc populate_by_name.
    for k, v in kw.items():
        _MP.setenv(k.upper(), "1" if v is True else "0" if v is False else str(v))
    return Settings(_env_file=None)


def _real(**kw) -> Settings:
    return _settings(ai_mock=False, anthropic_api_key=ANTHROPIC_KEY, **kw)


@pytest.fixture
def no_sdk(monkeypatch):
    import anthropic
    from google import genai

    def _boom(*a, **k):
        raise AssertionError("khong duoc tao client SDK khi lap rap")

    monkeypatch.setattr(anthropic, "AsyncAnthropic", _boom)
    monkeypatch.setattr(genai, "Client", _boom)


def _unpack(built):
    llm, embedder = built
    return llm, embedder


def _text(records) -> str:
    fmt = logging.Formatter("%(message)s")
    return "\n".join(fmt.format(r) + " " + str(r.__dict__) for r in records)


def test_AC13_ThatKhongKeyGemini_EmbedderNone_KhongTaoSdk(no_sdk) -> None:
    llm, embedder = _unpack(wiring.build_llm(_real()))
    assert llm is not None
    assert embedder is None


def test_AC12_ThatCoKeyGemini_CoEmbedder() -> None:
    _, embedder = _unpack(wiring.build_llm(_real(gemini_api_key=GEMINI_KEY)))
    assert embedder is not None
    assert hasattr(embedder, "embed")


def test_AC1_Mock_CoEmbedderMock_KhongTaoSdk(no_sdk) -> None:
    llm, embedder = _unpack(wiring.build_llm(_settings(ai_mock=True)))
    assert llm is not None
    assert embedder is not None  # mock_embedding


def test_AC13_LogL3TatDungMotLan_KhongLoKey(caplog) -> None:
    s = _real()
    _, embedder = _unpack(wiring.build_llm(s))
    caplog.set_level(logging.DEBUG)

    wiring.log_startup(s, embedder, deprecated=[])

    l3 = [r for r in caplog.records if L3_OFF in r.getMessage()]
    assert len(l3) == 1
    assert l3[0].levelno in (logging.INFO, logging.WARNING)
    text = _text(caplog.records)
    assert ANTHROPIC_KEY not in text


def test_AC13_CoKeyGemini_KhongLogL3Tat(caplog) -> None:
    s = _real(gemini_api_key=GEMINI_KEY)
    _, embedder = _unpack(wiring.build_llm(s))
    caplog.set_level(logging.DEBUG)
    wiring.log_startup(s, embedder, deprecated=[])
    assert not [r for r in caplog.records if L3_OFF in r.getMessage()]
    text = _text(caplog.records)
    assert ANTHROPIC_KEY not in text
    assert GEMINI_KEY not in text


def test_AC15_LogSanSang_CoProviderModelVaL3_KhongKey(caplog) -> None:
    s = _real(gemini_api_key=GEMINI_KEY, anthropic_model_chat="claude-chat-x",
              anthropic_model_moderation="claude-mod-y")
    _, embedder = _unpack(wiring.build_llm(s))
    caplog.set_level(logging.DEBUG)
    wiring.log_startup(s, embedder, deprecated=[])

    text = _text(caplog.records)
    assert "anthropic" in text
    assert "claude-chat-x" in text
    assert "claude-mod-y" in text
    assert "semantic_l3" in text
    assert "ai_mock" in text
    assert ANTHROPIC_KEY not in text
    assert GEMINI_KEY not in text


def test_AC14_AliasCu_CanhBaoMotLanNeuTenMoi(caplog) -> None:
    s = _settings(ai_mock=True)
    _, embedder = _unpack(wiring.build_llm(s))
    caplog.set_level(logging.DEBUG)
    wiring.log_startup(s, embedder, deprecated=["GEMINI_MOCK"])

    warns = [r for r in caplog.records if r.levelno == logging.WARNING and "GEMINI_MOCK" in r.getMessage()]
    assert len(warns) == 1
    assert "AI_MOCK" in warns[0].getMessage()


def test_AC14_KhongAlias_KhongCanhBaoDeprecated(caplog) -> None:
    s = _settings(ai_mock=True)
    _, embedder = _unpack(wiring.build_llm(s))
    caplog.set_level(logging.DEBUG)
    wiring.log_startup(s, embedder, deprecated=[])
    assert not [r for r in caplog.records if "GEMINI_MOCK" in r.getMessage()]
