"""Chung cho moi test ai-worker (AC-20): khong test nao doc key/cau hinh AI that tu moi truong."""

from __future__ import annotations

import os

import pytest

_PREFIXES = ("AI_", "GEMINI_", "ANTHROPIC_")


@pytest.fixture(autouse=True)
def _clear_ai_env(monkeypatch):
    # Quet theo tien to (khong danh sach co dinh): bien moi them sau nay cung bi xoa.
    for name in list(os.environ):
        if name.upper().startswith(_PREFIXES):
            monkeypatch.delenv(name, raising=False)
