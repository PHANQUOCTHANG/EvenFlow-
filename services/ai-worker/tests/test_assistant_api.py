"""API tro ly phia khach (EV-171): nhan va xep hang, khong bao gio goi Gemini dong bo."""

from __future__ import annotations

import json

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from app.api.assistant import AssistantDeps, router
from app.api.quota import QuotaDecision
from app.pipeline import degradation
from app.pipeline.results import result_key

EVENT = "evt-1"
ME = "id-me"


class FakeQuota:
    def __init__(self, allowed: bool = True, used: int = 1) -> None:
        self.allowed, self.used = allowed, used
        self.calls: list[tuple] = []

    async def take(self, event_id, identity, member, now_ms):
        self.calls.append((event_id, identity, member))
        return QuotaDecision(allowed=self.allowed, used=self.used, limit=10, reset_at_ms=now_ms + 900_000)


class FakeRedis:
    def __init__(self) -> None:
        self.kv: dict[str, str] = {}
        self.ttl: dict[str, int] = {}

    async def set(self, key, value, ex=None):
        self.kv[key] = value
        self.ttl[key] = ex

    async def get(self, key):
        v = self.kv.get(key)
        return v.encode() if isinstance(v, str) else v


class FakePublisher:
    def __init__(self, fail: bool = False) -> None:
        self.jobs: list[tuple[str, dict, str]] = []
        self.fail = fail

    async def publish(self, key, body, *, message_id):
        if self.fail:
            raise ConnectionError("broker sap")
        self.jobs.append((key, body, message_id))


def _client(quota=None, pub=None, redis=None, mode=degradation.Mode.NORMAL):
    app = FastAPI()
    app.include_router(router)
    app.state.assistant = AssistantDeps(
        quota=quota or FakeQuota(), publish=(pub or FakePublisher()).publish,
        redis=redis or FakeRedis(), mode=lambda: mode,
    )
    return TestClient(app)


def _post(c, question="Ca si nao mo man?", identity=ME, **body):
    headers = {"X-Identity-Id": identity} if identity else {}
    return c.post(f"/v1/events/{EVENT}/assistant/messages", json={"question": question, **body}, headers=headers)


def test_Post_202VaXepHangDungHopDongChatJob() -> None:
    pub, redis = FakePublisher(), FakeRedis()
    r = _post(_client(pub=pub, redis=redis), queue_token="qt-1")

    assert r.status_code == 202
    data = r.json()
    assert data["status"] == "queued"
    job_id = data["job_id"]
    assert data["poll_url"] == f"/v1/assistant/messages/{job_id}"
    assert data["stream_url"] == f"/v1/assistant/messages/{job_id}/stream"
    assert data["quota"]["max"] == 10
    key, body, message_id = pub.jobs[0]
    assert key == "chat" and message_id == job_id
    assert set(body) == {"job_id", "event_id", "identity_id", "question", "queue_token", "enqueued_at"}
    assert body["event_id"] == EVENT and body["identity_id"] == ME and body["queue_token"] == "qt-1"
    # Chu job duoc ghi TRUOC khi publish de GET/SSE kiem quyen.
    assert redis.kv[f"ai:job:{{{job_id}}}:owner"] == ME
    assert redis.ttl[f"ai:job:{{{job_id}}}:owner"] == 300


def test_Post_VuotQuota_TraFaqVaCSKH_KhongXepHang() -> None:
    pub = FakePublisher()
    r = _post(_client(quota=FakeQuota(allowed=False, used=10), pub=pub))

    assert r.status_code == 200
    data = r.json()
    assert data["status"] == "quota_exceeded"
    assert "CSKH" in data["answer"]
    assert data["suggestions"] and all({"intent", "answer"} <= set(s) for s in data["suggestions"])
    assert all("{" not in s["answer"] for s in data["suggestions"])  # khong lo template chua dien
    assert pub.jobs == []


def test_Post_NacOFF_KhongXepHangKhongTruQuota() -> None:
    quota, pub = FakeQuota(), FakePublisher()
    r = _post(_client(quota=quota, pub=pub, mode=degradation.Mode.OFF))

    assert r.status_code == 200
    assert r.json()["status"] == "unavailable"
    assert "CSKH" in r.json()["answer"]
    assert quota.calls == [] and pub.jobs == []


def test_Post_ThieuIdentity_401() -> None:
    r = _post(_client(), identity=None)
    assert r.status_code == 401
    assert r.headers["content-type"].startswith("application/problem+json")


@pytest.mark.parametrize("question", ["", "   ", "x" * 501])
def test_Post_CauHoiKhongHopLe_422(question) -> None:
    pub = FakePublisher()
    r = _post(_client(pub=pub), question=question)
    assert r.status_code == 422
    assert r.headers["content-type"].startswith("application/problem+json")
    assert pub.jobs == []


def test_Post_BrokerSap_503KhongLoLoi() -> None:
    r = _post(_client(pub=FakePublisher(fail=True)))
    assert r.status_code == 503
    assert r.headers["content-type"].startswith("application/problem+json")
    assert "broker" not in r.text.lower()


def _seed_job(redis: FakeRedis, job_id: str, owner: str = ME, result: dict | None = None) -> None:
    redis.kv[f"ai:job:{{{job_id}}}:owner"] = owner
    if result is not None:
        redis.kv[result_key(job_id)] = json.dumps(result)


def test_Get_ChuaXong_202Pending() -> None:
    redis = FakeRedis()
    _seed_job(redis, "j-1")
    r = _client(redis=redis).get("/v1/assistant/messages/j-1", headers={"X-Identity-Id": ME})
    assert r.status_code == 202
    assert r.json() == {"status": "pending", "job_id": "j-1"}


def test_Get_Xong_200KemKetQua() -> None:
    redis = FakeRedis()
    _seed_job(redis, "j-2", result={"job_id": "j-2", "answer": "Ban khong mat cho", "source": "faq"})
    r = _client(redis=redis).get("/v1/assistant/messages/j-2", headers={"X-Identity-Id": ME})
    assert r.status_code == 200
    assert r.json()["status"] == "done"
    assert r.json()["answer"] == "Ban khong mat cho"


@pytest.mark.parametrize("identity", ["id-khac", None])
def test_Get_KhongPhaiChu_404(identity) -> None:
    redis = FakeRedis()
    _seed_job(redis, "j-3", result={"answer": "bi mat"})
    headers = {"X-Identity-Id": identity} if identity else {}
    r = _client(redis=redis).get("/v1/assistant/messages/j-3", headers=headers)
    assert r.status_code in (401, 404)
    assert "bi mat" not in r.text


def test_Get_KhongTonTai_404() -> None:
    r = _client().get("/v1/assistant/messages/khong-co", headers={"X-Identity-Id": ME})
    assert r.status_code == 404


def test_Stream_KhongPhaiChu_404() -> None:
    redis = FakeRedis()
    _seed_job(redis, "j-4")
    r = _client(redis=redis).get("/v1/assistant/messages/j-4/stream", headers={"X-Identity-Id": "id-khac"})
    assert r.status_code == 404


@pytest.mark.parametrize(("raw", "status"), [(b"khong phai json", 400), (b"\xff\xfe", 400), (b'["mang"]', 422)])
def test_Post_BodyHong_LoiRFC7807(raw, status) -> None:
    pub = FakePublisher()
    r = _client(pub=pub).post(f"/v1/events/{EVENT}/assistant/messages", content=raw,
                              headers={"X-Identity-Id": ME, "Content-Type": "application/json"})
    assert r.status_code == status
    assert r.headers["content-type"].startswith("application/problem+json")
    assert pub.jobs == []
