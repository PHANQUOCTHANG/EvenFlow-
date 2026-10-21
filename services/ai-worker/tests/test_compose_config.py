"""Cau hinh compose cua ai-worker khop voi service no goi (EV-120, AC-4)."""

from __future__ import annotations

from pathlib import Path
from urllib.parse import urlsplit

import pytest
import yaml

COMPOSE = Path(__file__).resolve().parents[3] / "deploy" / "compose" / "docker-compose.yml"

pytestmark = pytest.mark.skipif(not COMPOSE.is_file(), reason="chi chay trong repo")


def _services() -> dict:
    return yaml.safe_load(COMPOSE.read_text(encoding="utf-8"))["services"]


def test_CallbackKiemDuyetTroThangEventService_DungCong() -> None:
    svc = _services()
    target = urlsplit(svc["ai-worker"]["environment"]["EVENT_API_BASE"])
    event_port = svc["event"]["environment"]["HTTP_ADDR"].rsplit(":", 1)[1]

    assert target.hostname == "event"
    assert str(target.port) == event_port


def test_SystemToolsVanDiQuaGateway() -> None:
    svc = _services()
    assert urlsplit(svc["ai-worker"]["environment"]["INTERNAL_API_BASE"]).hostname == "gateway"
