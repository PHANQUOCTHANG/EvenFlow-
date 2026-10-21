"""Consumer cho hang doi ai.moderation.q -- kiem duyet noi dung su kien (EV-120).

Consumer nay nhan cac job kiem duyet su kien khi Organizer nop su kien (POST /submit),
tien hanh quet 4 truc rui ro (BR-E4) bang Gemini, va goi callback ve event service
qua endpoint noi bo PATCH /internal/v1/events/{id}/ai-verdict.

Moi lan goi Gemini that deu qua limiter (app.gemini.guarded): kiem duyet dung chung han
muc voi tro ly cua khach dang cho, nen khong duoc goi "chen ngang".
"""

from __future__ import annotations

import functools
import json
import logging
from collections.abc import Callable
from typing import Any

import httpx

from app.domain.moderation import AiVerdict, ModerationJob, ModerationResult
from app.gemini.client import GeminiClient, RateLimited
from app.gemini.guarded import PermitDenied, guarded_generate
from app.mq.protocol import MAX_ATTEMPTS, PermanentError
from app.mq.runtime import Delivery

log = logging.getLogger(__name__)

# AI khong chay duoc sau moi lan thu: su kien khong duoc ket o PENDING_REVIEW, chuyen
# sang hang viec cua Moderator (BR-E4) voi ly do ro rang.
AI_UNAVAILABLE_REASON = "AI kiểm duyệt tạm thời không khả dụng, chuyển người duyệt"


class ModerationConsumer:
    def __init__(
        self,
        gemini: GeminiClient,
        api_base: str,
        client: httpx.AsyncClient | None = None,
        callback_fn: Callable[[str, ModerationResult], Any] | None = None,
        limiter: Any = None,
    ) -> None:
        self._gemini = gemini
        self._api_base = api_base.rstrip("/")
        self._client = client or httpx.AsyncClient(timeout=10.0)
        self._callback_fn = callback_fn
        self._generate = functools.partial(guarded_generate, limiter, gemini) if limiter is not None else None

    @staticmethod
    def parse(body: bytes) -> ModerationJob:
        data = json.loads(body.decode("utf-8"))
        return ModerationJob(
            job_id=str(data.get("job_id", "")),
            event_id=str(data.get("event_id", "")),
            event_title=str(data.get("event_title", "")),
            event_description=str(data.get("event_description", "")),
            enqueued_at=float(data.get("enqueued_at", 0.0)),
        )

    async def handle_message(self, body: bytes) -> ModerationResult:
        job = self.parse(body)

        log.info("Bat dau kiem duyet su kien %s (job=%s)", job.event_id, job.job_id)
        result = await self._gemini.scan_moderation(job, generate=self._generate)
        log.info(
            "Ket qua kiem duyet su kien %s: verdict=%s, score=%.2f",
            job.event_id,
            result.verdict.value,
            result.score,
        )
        await self._deliver(job.event_id, result)
        return result

    async def send_needs_human(self, body: bytes, reason: str = AI_UNAVAILABLE_REASON) -> None:
        job = self.parse(body)
        log.warning("Chuyen su kien %s sang nguoi duyet: %s", job.event_id, reason)
        await self._deliver(
            job.event_id,
            ModerationResult(score=0.5, verdict=AiVerdict.NEEDS_HUMAN, axes={}, reasons=[reason]),
        )

    async def _deliver(self, event_id: str, result: ModerationResult) -> None:
        if self._callback_fn is not None:
            res = self._callback_fn(event_id, result)
            if hasattr(res, "__await__"):
                await res
        else:
            await self._send_verdict_callback(event_id, result)

    def verdict_url(self, event_id: str) -> str:
        return f"{self._api_base}/internal/v1/events/{event_id}/ai-verdict"

    async def _send_verdict_callback(self, event_id: str, result: ModerationResult) -> None:
        url = self.verdict_url(event_id)
        payload = {
            "score": result.score,
            "verdict": result.verdict.value,
            "reasons": result.reasons,
            "axes": result.axes,
        }
        resp = await self._client.patch(url, json=payload)
        resp.raise_for_status()

    async def aclose(self) -> None:
        await self._client.aclose()


def make_handler(consumer: ModerationConsumer):
    """Noi ModerationConsumer vao runtime.

    Body khong doc duoc la loi vinh vien (DLQ ngay). Limiter chua cho phep / Gemini 429 /
    callback event service loi la loi tam (thu lai qua ai.moderation.retry, 30s). Lan cuoi
    van khong goi duoc Gemini thi chuyen nguoi duyet thay vi de su kien ket.
    """

    async def handler(delivery: Delivery) -> str | None:
        try:
            await consumer.handle_message(delivery.body)
        except (json.JSONDecodeError, UnicodeDecodeError, AttributeError) as exc:
            raise PermanentError(f"job kiem duyet hong: {type(exc).__name__}") from exc
        except (PermitDenied, RateLimited):
            if delivery.attempt < MAX_ATTEMPTS:
                raise
            await consumer.send_needs_human(delivery.body)
            return "needs_human"
        return None

    return handler
