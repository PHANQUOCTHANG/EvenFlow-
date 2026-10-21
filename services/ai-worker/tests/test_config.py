"""Cau hinh sai thi khong khoi dong (AC-10)."""

from __future__ import annotations

from pathlib import Path

import pytest
from pydantic import ValidationError

from app.config import Settings


def test_MacDinhHopLe() -> None:
    s = Settings(_env_file=None)
    assert s.chat_consumers == 20
    assert s.report_consumers == 1
    assert s.gemini_timeout_seconds > 0
    assert s.shutdown_grace_seconds > 0


@pytest.mark.parametrize(
    "override",
    [
        {"chat_consumers": 0},
        {"moderation_consumers": 0},
        {"anomaly_consumers": -1},
        {"report_consumers": -1},
        {"gemini_timeout_seconds": 0},
        {"shutdown_grace_seconds": -1},
        {"gemini_rpm": 0},
        {"gemini_tpm": 0},
    ],
)
def test_GiaTriSaiBiTuChoi(override) -> None:
    with pytest.raises(ValidationError):
        Settings(_env_file=None, **override)


def test_AnomalyVaReportDuocTat() -> None:
    # Chua co handler (EV-167, EV-168): dat 0 de khong mo pool.
    s = Settings(_env_file=None, anomaly_consumers=0, report_consumers=0)
    assert s.anomaly_consumers == 0
    assert s.report_consumers == 0


def test_TopologyMacDinh_TrongRepoTroToiDeploy() -> None:
    assert Settings(_env_file=None).ai_topology_file.is_file()


def test_TopologyMacDinh_TrongImageKhongVoLucImport() -> None:
    # /app/app/config.py chi co 3 cap cha: truoc day parents[3] nem IndexError luc import
    # va worker chet ngay khi khoi dong trong container.
    from pathlib import PurePosixPath

    from app.config import default_topology_file

    got = default_topology_file(Path(PurePosixPath("/app/app/config.py")))
    assert got == Path("topology") / "definitions.json"


def test_GeminiThatMaThieuKeyThiTuChoi() -> None:
    with pytest.raises(ValidationError, match="GEMINI_API_KEY"):
        Settings(_env_file=None, gemini_mock=False, gemini_api_key="")


def test_CallbackKiemDuyet_MacDinhDiThangEventService() -> None:
    # Gateway chan /internal/ (404) -> callback qua gateway lam su kien ket PENDING_REVIEW.
    s = Settings(_env_file=None)
    assert s.event_api_base == "http://event:8084"
    assert s.internal_api_base == "http://gateway:8080"  # SystemTools van qua gateway


@pytest.mark.parametrize("bad", ["", "event:8084", "ftp://event:8084", "http://"])
def test_EventApiBaseSaiBiTuChoi(bad) -> None:
    with pytest.raises(ValidationError, match="EVENT_API_BASE"):
        Settings(_env_file=None, event_api_base=bad)
