"""ops-warroom fix round 1 (X-9): /v1/ops/ai/mode checks identity like Go authx.

The role is compared without trimming and the identity id must be UUID-shaped.
"""

from __future__ import annotations

from fastapi.testclient import TestClient

from app.main import app

client = TestClient(app)
PATH = "/v1/ops/ai/mode"
ID = "22222222-2222-4222-8222-222222222222"


def test_role_with_whitespace_is_not_ops() -> None:
    for role in (" ops ", "ops ", " ops", "ops\t"):
        r = client.get(PATH, headers={"X-Identity-Id": ID, "X-Identity-Role": role})
        assert r.status_code == 403, repr(role)
        assert r.json()["type"] == "/problems/forbidden"


def test_non_uuid_identity_is_401() -> None:
    for ident in ("anything-not-uuid", " " + ID, ID + " ", ID[:-1], "x"):
        r = client.get(PATH, headers={"X-Identity-Id": ident, "X-Identity-Role": "ops"})
        assert r.status_code == 401, repr(ident)
        assert r.json()["status"] == 401
        assert r.headers["cache-control"] == "no-store"


def test_uuid_identity_any_case_is_accepted() -> None:
    for ident in (ID, "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"):
        r = client.get(PATH, headers={"X-Identity-Id": ident, "X-Identity-Role": "ops"})
        assert r.status_code == 200, ident
