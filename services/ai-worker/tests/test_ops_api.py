"""ops-warroom AC-18: GET /v1/ops/ai/mode (black box through the ASGI app).

Assumptions: endpoint lives on `app.main.app`; reads X-Identity-Id / X-Identity-Role; response per contracts §5.1
(mode, manual_override, signals{3}, generated_at) without `policy`; errors are problem+JSON; /admin/* unchanged.
Uses fastapi TestClient (no lifespan) so no Redis/RabbitMQ is needed.
"""

from __future__ import annotations

from datetime import datetime

from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)
OPS = {"X-Identity-Id": "22222222-2222-4222-8222-222222222222", "X-Identity-Role": "ops"}
PATH = "/v1/ops/ai/mode"


def test_ops_gets_mode_with_contract_shape() -> None:
    r = client.get(PATH, headers=OPS)
    assert r.status_code == 200
    body = r.json()
    assert set(body) == {"mode", "manual_override", "signals", "generated_at"}
    assert set(body["signals"]) == {"chat_queue_lag_seconds", "budget_used_ratio", "gemini_circuit_open"}
    assert "policy" not in body
    datetime.fromisoformat(body["generated_at"])


def test_same_values_as_admin_mode() -> None:
    admin = client.get("/admin/mode").json()
    ops = client.get(PATH, headers=OPS).json()
    assert ops["mode"] == admin["mode"]
    assert ops["manual_override"] == admin["manual_override"]
    assert ops["signals"] == admin["signals"]


def test_non_ops_roles_forbidden_with_problem_json() -> None:
    for role in ("buyer", "organizer", "moderator", ""):
        r = client.get(PATH, headers={"X-Identity-Id": OPS["X-Identity-Id"], "X-Identity-Role": role})
        assert r.status_code == 403, role
        body = r.json()
        assert body["type"] == "/problems/forbidden"
        assert body["status"] == 403
        assert "title" in body and "detail" in body


def test_missing_identity_is_401_problem_json() -> None:
    for headers in ({}, {"X-Identity-Role": "ops"}):
        r = client.get(PATH, headers=headers)
        assert r.status_code == 401
        body = r.json()
        assert body["status"] == 401
        assert body["type"].startswith("/problems/")


def test_admin_mode_unchanged() -> None:
    r = client.get("/admin/mode")
    assert r.status_code == 200
    assert "policy" in r.json()
