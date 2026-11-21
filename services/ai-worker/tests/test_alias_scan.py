"""AC-14 (plan-review R-1): phat hien bien moi truong cu (alias GEMINI_*) va co mock dung chung."""

from __future__ import annotations

import pytest

from app.config import deprecated_aliases_used, resolve_mock_flag


def test_KhongBienNao_KhongAlias() -> None:
    assert deprecated_aliases_used(env_file=None) == []


def test_AliasTrongOsEnviron(monkeypatch) -> None:
    monkeypatch.setenv("GEMINI_MOCK", "1")
    monkeypatch.setenv("GEMINI_RPM", "30")
    assert sorted(deprecated_aliases_used(env_file=None)) == ["GEMINI_MOCK", "GEMINI_RPM"]


def test_AliasKhongPhanBietHoaThuong(monkeypatch) -> None:
    monkeypatch.setenv("gemini_tpm", "300000")
    assert deprecated_aliases_used(env_file=None) == ["GEMINI_TPM"]


def test_TenMoiDaDat_KhongTinhAlias(monkeypatch) -> None:
    monkeypatch.setenv("GEMINI_MOCK", "0")
    monkeypatch.setenv("AI_MOCK", "1")
    assert deprecated_aliases_used(env_file=None) == []


def test_AliasTrongFileEnv(tmp_path) -> None:
    env = tmp_path / ".env"
    env.write_text("GEMINI_MOCK=0\ngemini_timeout_seconds=5\nAI_RPM=40\n", "utf-8")
    assert sorted(deprecated_aliases_used(env_file=env)) == ["GEMINI_MOCK", "GEMINI_TIMEOUT_SECONDS"]


def test_TenMoiTrongEnv_AliasTrongFile_KhongTinh(tmp_path, monkeypatch) -> None:
    env = tmp_path / ".env"
    env.write_text("GEMINI_MOCK=0\n", "utf-8")
    monkeypatch.setenv("AI_MOCK", "1")
    assert deprecated_aliases_used(env_file=env) == []


def test_EnvFileNone_BoQuaFile(tmp_path, monkeypatch) -> None:
    (tmp_path / ".env").write_text("GEMINI_MOCK=0\n", "utf-8")
    monkeypatch.chdir(tmp_path)
    assert deprecated_aliases_used(env_file=None) == []
    assert deprecated_aliases_used() == ["GEMINI_MOCK"]  # mac dinh doc .env trong thu muc hien tai


def test_FileKhongTonTai_KhongLoi(tmp_path) -> None:
    assert deprecated_aliases_used(env_file=tmp_path / "khong-co.env") == []


@pytest.mark.parametrize(
    ("env", "expected"),
    [
        ({}, True),
        ({"AI_MOCK": "0"}, False),
        ({"AI_MOCK": "1"}, True),
        ({"GEMINI_MOCK": "0"}, False),
        ({"GEMINI_MOCK": "false"}, False),
        ({"AI_MOCK": "1", "GEMINI_MOCK": "0"}, True),
        ({"AI_MOCK": "0", "GEMINI_MOCK": "1"}, False),
        ({"ai_mock": "0"}, False),
    ],
)
def test_ResolveMockFlag(env, expected) -> None:
    assert resolve_mock_flag(env) is expected
