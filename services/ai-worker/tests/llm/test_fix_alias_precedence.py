"""Fix X-5/R-3: ten moi thang alias cu bat ke nguon (process env hay .env); LlmClient() khop Settings.

Case A/B/C lay tu repro cua challenger (test_alias_precedence).
"""

from __future__ import annotations

import pytest

from app.config import Settings, deprecated_aliases_used
from app.llm.client import LlmClient

from .conftest import ANTHROPIC_KEY


def test_X5_A_AI_MOCKTrongEnvFile_ThangGEMINI_MOCKTrongShell(tmp_path, monkeypatch) -> None:
    env = tmp_path / ".env"
    env.write_text("AI_MOCK=1\nAI_RPM=10\n", "utf-8")
    monkeypatch.setenv("GEMINI_MOCK", "0")
    monkeypatch.setenv("GEMINI_RPM", "999")
    monkeypatch.setenv("ANTHROPIC_API_KEY", ANTHROPIC_KEY)
    s = Settings(_env_file=str(env))
    assert s.ai_mock is True
    assert s.ai_rpm == 10
    # Alias khong co hieu luc -> khong canh bao, va dung la gia tri ten moi dang chay.
    assert deprecated_aliases_used(env) == []


def test_X5_B_AI_MOCK0TrongEnvFile_ThangGEMINI_MOCK1TrongShell(tmp_path, monkeypatch) -> None:
    env = tmp_path / ".env"
    env.write_text("AI_MOCK=0\nANTHROPIC_API_KEY=k\n", "utf-8")
    monkeypatch.setenv("GEMINI_MOCK", "1")
    s = Settings(_env_file=str(env))
    assert s.ai_mock is False


@pytest.mark.parametrize(
    ("alias", "new", "field", "file_val", "env_val"),
    [
        ("GEMINI_TIMEOUT_SECONDS", "AI_TIMEOUT_SECONDS", "ai_timeout_seconds", 4, 9),
        ("GEMINI_RPM", "AI_RPM", "ai_rpm", 11, 99),
        ("GEMINI_TPM", "AI_TPM", "ai_tpm", 1111, 9999),
    ],
)
def test_X5_MoiAlias_TenMoiTrongFileThangAliasTrongShell(tmp_path, monkeypatch, alias, new, field,
                                                         file_val, env_val) -> None:
    env = tmp_path / ".env"
    env.write_text(f"{new}={file_val}\n", "utf-8")
    monkeypatch.setenv(alias, str(env_val))
    assert getattr(Settings(_env_file=str(env)), field) == file_val


def test_X5_AliasChiTrongShell_VanDocVaCanhBao(tmp_path, monkeypatch) -> None:
    env = tmp_path / ".env"
    env.write_text("AI_TPM=1234\n", "utf-8")
    monkeypatch.setenv("GEMINI_RPM", "33")
    s = Settings(_env_file=str(env))
    assert (s.ai_rpm, s.ai_tpm) == (33, 1234)
    assert deprecated_aliases_used(env) == ["GEMINI_RPM"]


def test_X5_TenMoiTrongShell_ThangTenMoiTrongFile(tmp_path, monkeypatch) -> None:
    env = tmp_path / ".env"
    env.write_text("AI_RPM=10\n", "utf-8")
    monkeypatch.setenv("AI_RPM", "20")
    assert Settings(_env_file=str(env)).ai_rpm == 20


def test_X5_ThamSoKhoiTao_VanThangMoiNguon(tmp_path, monkeypatch) -> None:
    env = tmp_path / ".env"
    env.write_text("AI_MOCK=1\n", "utf-8")
    monkeypatch.setenv("GEMINI_MOCK", "1")
    s = Settings(_env_file=str(env), ai_mock=False, anthropic_api_key=ANTHROPIC_KEY)
    assert s.ai_mock is False


def test_X5_C_LlmClientKhongThamSo_DocEnvFileGiongSettings(tmp_path, monkeypatch) -> None:
    (tmp_path / ".env").write_text(f"AI_MOCK=0\nANTHROPIC_API_KEY={ANTHROPIC_KEY}\n", "utf-8")
    monkeypatch.chdir(tmp_path)
    assert Settings().ai_mock is False
    c = LlmClient()
    assert c.mock is False
    assert c._api_key == ANTHROPIC_KEY


def test_X5_LlmClientKhongThamSo_AI_MOCKTrongFileThangGEMINI_MOCKTrongShell(tmp_path, monkeypatch) -> None:
    (tmp_path / ".env").write_text("AI_MOCK=1\n", "utf-8")
    monkeypatch.chdir(tmp_path)
    monkeypatch.setenv("GEMINI_MOCK", "0")
    monkeypatch.setenv("ANTHROPIC_API_KEY", ANTHROPIC_KEY)
    assert Settings().ai_mock is True
    assert LlmClient().mock is True
