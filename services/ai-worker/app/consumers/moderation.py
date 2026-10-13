"""Consumer cho hang doi ai.moderation.q -- kiem duyet noi dung su kien (EV-120).

Consumer nay nhan cac job kiem duyet su kien khi Organizer nop su kien (POST /submit),
tien hanh quet 4 truc rui ro (BR-E4) bang Gemini, va goi callback ve event service
qua endpoint noi bo PATCH /internal/v1/events/{id}/ai-verdict.
"""

from __future__ import annotations

import json
import logging
from collections.abc import Callable
from typing import Any

import httpx

from app.domain.moderation import ModerationJob, ModerationResult
from app.gemini.client import GeminiClient

log = logging.getLogger(__name__)


class ModerationConsumer:
    def __init__(
        self,
        gemini: GeminiClient,
        api_base: str,
        client: httpx.AsyncClient | None = None,
        callback_fn: Callable[[str, ModerationResult], Any] | None = None,
    ) -> None:
        self._gemini = gemini
        self._api_base = api_base.rstrip("/")
        self._client = client or httpx.AsyncClient(timeout=10.0)
        self._callback_fn = callback_fn

    async def handle_message(self, body: bytes) -> ModerationResult:
        data = json.loads(body.decode("utf-8"))
        job = ModerationJob(
            job_id=str(data.get("job_id", "")),
            event_id=str(data.get("event_id", "")),
            event_title=str(data.get("event_title", "")),
            event_description=str(data.get("event_description", "")),
            enqueued_at=float(data.get("enqueued_at", 0.0)),
        )

        log.info("Bat dau kiem duyet su kien %s (job=%s)", job.event_id, job.job_id)
        result = await self._gemini.scan_moderation(job)
        log.info(
            "Ket qua kiem duyet su kien %s: verdict=%s, score=%.2f",
            job.event_id,
            result.verdict.value,
            result.score,
        )

        if self._callback_fn is not None:
            res = self._callback_fn(job.event_id, result)
            if hasattr(res, "__await__"):
                await res
        else:
            await self._send_verdict_callback(job.event_id, result)

        return result

    async def _send_verdict_callback(self, event_id: str, result: ModerationResult) -> None:
        url = f"{self._api_base}/internal/v1/events/{event_id}/ai-verdict"
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
