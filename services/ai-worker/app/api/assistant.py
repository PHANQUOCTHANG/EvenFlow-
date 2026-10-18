"""API tro ly phia khach (EV-171).

API chi NHAN va XEP HANG (BR-A1): khong bao gio goi Gemini trong request. Cau tra loi toi
sau qua SSE hoac polling, doc tu hop dong ket qua cua consumer chat (EV-157):
`ai:job:{job_id}:result` + `PUBLISH ai:job:{job_id}`.

Khach KHONG BAO GIO nhan loi ky thuat thay cho cau tra loi: vuot han muc -> FAQ + CSKH;
tro ly tat (nac OFF) -> thong bao CSKH; job qua han -> cau tra loi du phong.
"""

from __future__ import annotations

import asyncio
import json
import logging
import time
import uuid
from collections.abc import AsyncIterator, Awaitable, Callable
from dataclasses import dataclass
from typing import Any

from fastapi import APIRouter, Header, Request
from fastapi.responses import JSONResponse, StreamingResponse

from app import metrics
from app.api.quota import QuotaDecision
from app.pipeline import degradation
from app.pipeline.faq import FALLBACK, STATIC_ANSWERS
from app.pipeline.results import result_channel, result_key

log = logging.getLogger(__name__)

MAX_QUESTION_CHARS = 500
OWNER_TTL_SECONDS = 300  # bang TTL cua ket qua (pipeline/results.py)
# TTL job chat la 60 giay (ai.chat.q): qua 65 giay ma chua co ket qua thi job da bi bo.
STREAM_TIMEOUT_SECONDS = 65.0
STREAM_PING_SECONDS = 15.0

CSKH = "Neu can ho tro them, vui long lien he CSKH (hotline 1900 6868, 08:00 - 22:00)."
QUOTA_EXCEEDED_ANSWER = (
    "Ban da hoi 10 cau trong 15 phut gan day. Trong luc cho, ban co the xem cac cau hoi "
    "thuong gap ben duoi. " + CSKH
)


@dataclass
class AssistantDeps:
    quota: Any                                       # AssistantQuota
    publish: Callable[..., Awaitable[None]]          # JobPublisher.publish
    redis: Any
    mode: Callable[[], degradation.Mode]
    stream_timeout_s: float = STREAM_TIMEOUT_SECONDS
    stream_ping_s: float = STREAM_PING_SECONDS


router = APIRouter()


def owner_key(job_id: str) -> str:
    return f"ai:job:{{{job_id}}}:owner"


def _problem(status: int, title: str, detail: str | None = None) -> JSONResponse:
    body: dict[str, Any] = {"type": "about:blank", "title": title, "status": status}
    if detail:
        body["detail"] = detail
    return JSONResponse(body, status_code=status, media_type="application/problem+json")


def _quota_json(q: QuotaDecision) -> dict[str, int]:
    return {"used": q.used, "max": q.limit, "reset_at": q.reset_at_ms}


def _suggestions() -> list[dict[str, str]]:
    # Chi template tinh da dien day du: template co {placeholder} can so lieu su kien.
    return [{"intent": k, "answer": v} for k, v in STATIC_ANSWERS.items() if "{" not in v]


def _deps(request: Request) -> AssistantDeps:
    return request.app.state.assistant


def _done(outcome: str, started: float) -> None:
    metrics.ASSISTANT_REQUESTS.labels(outcome).inc()
    metrics.ASSISTANT_ACCEPT_SECONDS.observe(time.perf_counter() - started)


@router.post("/v1/events/{event_id}/assistant/messages")
async def post_message(
    event_id: str,
    request: Request,
    x_identity_id: str | None = Header(default=None),
) -> JSONResponse:
    started = time.perf_counter()
    deps = _deps(request)
    if not x_identity_id:
        _done("unauthorized", started)
        return _problem(401, "Thieu danh tinh", "Can dang nhap de hoi tro ly.")

    # Tu doc body: loi JSON cung phai la RFC 7807 (docs/03 §7), khong phai {"detail": ...} mac dinh.
    try:
        payload = await request.json()
    except (ValueError, UnicodeDecodeError):
        _done("invalid", started)
        return _problem(400, "Body khong phai JSON UTF-8 hop le")
    if not isinstance(payload, dict):
        _done("invalid", started)
        return _problem(422, "Body phai la JSON object")

    question = payload.get("question")
    if not isinstance(question, str) or not question.strip() or len(question) > MAX_QUESTION_CHARS:
        _done("invalid", started)
        return _problem(422, "Cau hoi khong hop le", f"Cau hoi phai co tu 1 den {MAX_QUESTION_CHARS} ky tu.")
    queue_token = payload.get("queue_token") or ""
    if not isinstance(queue_token, str):
        _done("invalid", started)
        return _problem(422, "queue_token khong hop le")

    mode = deps.mode()
    if mode is degradation.Mode.OFF:
        # Khong tru quota: khach khong duoc phuc vu thi khong mat luot.
        _done("unavailable", started)
        notice = degradation.policy_for(mode).user_notice or CSKH
        return JSONResponse({"status": "unavailable", "answer": notice, "mode": str(mode)})

    job_id = str(uuid.uuid4())
    now = time.time()
    quota = await deps.quota.take(event_id, x_identity_id, job_id, int(now * 1000))
    if not quota.allowed:
        _done("quota_exceeded", started)
        return JSONResponse({
            "status": "quota_exceeded",
            "answer": QUOTA_EXCEEDED_ANSWER,
            "suggestions": _suggestions(),
            "quota": _quota_json(quota),
        })

    # Chu job ghi TRUOC khi publish: ket qua co the toi truoc ca khi request nay tra ve.
    await deps.redis.set(owner_key(job_id), x_identity_id, ex=OWNER_TTL_SECONDS)
    job = {
        "job_id": job_id,
        "event_id": event_id,
        "identity_id": x_identity_id,
        "question": question.strip(),
        "queue_token": queue_token,
        "enqueued_at": now,
    }
    try:
        await deps.publish("chat", job, message_id=job_id)
    except Exception:
        # Khong lo chi tiet ha tang ra ngoai; log du de truy vet.
        log.exception("xep hang cau hoi that bai", extra={"job_id": job_id, "event_id": event_id})
        _done("enqueue_failed", started)
        return _problem(503, "Tro ly tam thoi ban", "Vui long thu lai sau it giay. " + CSKH)

    _done("queued", started)
    return JSONResponse(
        {
            "status": "queued",
            "job_id": job_id,
            "poll_url": f"/v1/assistant/messages/{job_id}",
            "stream_url": f"/v1/assistant/messages/{job_id}/stream",
            "quota": _quota_json(quota),
        },
        status_code=202,
    )


async def _authorize(deps: AssistantDeps, job_id: str, identity: str | None) -> JSONResponse | None:
    if not identity:
        return _problem(401, "Thieu danh tinh")
    owner = await deps.redis.get(owner_key(job_id))
    if isinstance(owner, bytes):
        owner = owner.decode()
    if owner != identity:
        # 404 chu khong 403: khong cho nguoi khac biet job_id co ton tai.
        return _problem(404, "Khong tim thay cau hoi")
    return None


def _decode(raw: Any) -> dict[str, Any]:
    if isinstance(raw, bytes):
        raw = raw.decode()
    return json.loads(raw)


@router.get("/v1/assistant/messages/{job_id}")
async def get_message(job_id: str, request: Request, x_identity_id: str | None = Header(default=None)) -> JSONResponse:
    deps = _deps(request)
    if (denied := await _authorize(deps, job_id, x_identity_id)) is not None:
        return denied
    raw = await deps.redis.get(result_key(job_id))
    if raw is None:
        return JSONResponse({"status": "pending", "job_id": job_id}, status_code=202)
    return JSONResponse({"status": "done", **_decode(raw)})


def _sse(event: str, data: dict[str, Any]) -> str:
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n"


async def _stream(deps: AssistantDeps, job_id: str) -> AsyncIterator[str]:
    pubsub = deps.redis.pubsub()
    # SUBSCRIBE truoc roi moi GET: ket qua toi giua hai buoc van khong bi lo.
    await pubsub.subscribe(result_channel(job_id))
    try:
        if (raw := await deps.redis.get(result_key(job_id))) is not None:
            yield _sse("answer", _decode(raw))
            return
        loop = asyncio.get_running_loop()
        deadline = loop.time() + deps.stream_timeout_s
        next_ping = loop.time() + deps.stream_ping_s
        while loop.time() < deadline:
            msg = await pubsub.get_message(ignore_subscribe_messages=True, timeout=1.0)
            if msg is not None and msg.get("type") == "message":
                yield _sse("answer", _decode(msg["data"]))
                return
            if loop.time() >= next_ping:
                yield ": ping\n\n"
                next_ping = loop.time() + deps.stream_ping_s
        metrics.ASSISTANT_REQUESTS.labels("stream_timeout").inc()
        yield _sse("timeout", {"job_id": job_id, "answer": f"{FALLBACK}\n\n{CSKH}", "source": "timeout"})
    finally:
        await pubsub.unsubscribe(result_channel(job_id))
        await pubsub.aclose()


@router.get("/v1/assistant/messages/{job_id}/stream", response_model=None)
async def stream_message(
    job_id: str, request: Request, x_identity_id: str | None = Header(default=None)
) -> StreamingResponse | JSONResponse:
    deps = _deps(request)
    if (denied := await _authorize(deps, job_id, x_identity_id)) is not None:
        return denied
    return StreamingResponse(
        _stream(deps, job_id),
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )
