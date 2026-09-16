"""Consumer cho hang doi ai.chat.q -- tro ly phong cho.

Day la noi 7 lop giam tai trong docs/02 duoc lap rap lai. Thu tu cac lop la co
chu y: lop re nhat dat truoc, lop dat nhat dat sau, va moi lop deu co the tra
loi tron ven ma khong can lop sau.

  L2 FAQ          -> 0 token, ~40% cau hoi
  L3 cache        -> 0 token, ~45% phan con lai
  L4 singleflight -> gop cac cau y het dang cho
  L6 limiter      -> xin phep truoc khi cham Gemini
  L7 suy bien     -> luon con duong lui, khach khong bao gio thay loi
"""

from __future__ import annotations

import json
import logging
import time
from dataclasses import dataclass

from app.gemini.limiter import GlobalRateLimiter, estimate_tokens
from app.pipeline import degradation
from app.pipeline.faq import FaqMatcher
from app.pipeline.semantic_cache import SemanticCache
from app.pipeline.singleflight import SingleFlight

log = logging.getLogger(__name__)


@dataclass
class ChatJob:
    job_id: str
    event_id: str
    identity_id: str
    question: str
    queue_token: str
    enqueued_at: float


class Retry(Exception):
    """Bao cho consumer nack message va thu lai sau delay_ms."""

    def __init__(self, delay_ms: int) -> None:
        super().__init__(f"thu lai sau {delay_ms}ms")
        self.delay_ms = delay_ms


class ChatConsumer:
    def __init__(
        self,
        limiter: GlobalRateLimiter,
        cache: SemanticCache,
        faq: FaqMatcher,
        flight: SingleFlight,
        gemini,        # app.gemini.client.GeminiClient
        tools,         # app.pipeline.tools.SystemTools
        signals,       # callable -> degradation.Signals
        publish_result,  # callable(job_id, payload) -> None
    ) -> None:
        self._limiter = limiter
        self._cache = cache
        self._faq = faq
        self._flight = flight
        self._gemini = gemini
        self._tools = tools
        self._signals = signals
        self._publish = publish_result

    async def handle(self, raw: bytes) -> None:
        job = ChatJob(**json.loads(raw))

        # Message qua han bi bo, CO Y (BR-A3). Cau tra loi den sau khi khach da
        # duoc admit vao mua ve la vo gia tri -- giu lai chi ton quota va lam
        # nhung nguoi con dang cho phai doi lau hon.
        age = time.time() - job.enqueued_at
        if age > 60:
            log.info("bo job qua han", extra={"job_id": job.job_id, "age_s": round(age, 1)})
            return

        mode = degradation.decide(self._signals())
        policy = degradation.policy_for(mode)

        # ---- L2: FAQ, khong ton token ----
        # Tra loi FAQ duoc dien so lieu THAT tu he thong (BR-A5), khong phai so
        # do mo hinh tu nghi ra.
        if (answer := await self._faq.match(job.question, job.event_id, job.queue_token, self._tools)):
            await self._emit(job, answer, source="faq", mode=mode)
            return

        if not policy.call_llm or not policy.allow_open_questions:
            await self._emit(job, self._faq.fallback(policy.user_notice), source="degraded", mode=mode)
            return

        # ---- L3: cache ngu nghia ----
        if (hit := await self._cache.get(job.event_id, job.question)):
            await self._emit(job, hit, source="cache", mode=mode)
            return

        # ---- L4: singleflight ----
        # 1.000 nguoi hoi cung mot cau trong cung mot giay chi tao ra DUNG MOT
        # lan goi Gemini. Nhung nguoi con lai doi ket qua cua lan goi do.
        key = self._cache.key(job.event_id, job.question)
        leader = await self._flight.acquire(key)
        if not leader:
            if (shared := await self._flight.wait(key, timeout=8.0)):
                await self._emit(job, shared, source="singleflight", mode=mode)
                return
            # Nguoi dan dau that bai hoac qua lau -> tu minh lam tiep.

        try:
            answer = await self._call_gemini(job, policy)
        except Retry:
            raise
        except Exception:
            # Gemini hong -> van tra loi duoc bang FAQ. Khach khong bao gio thay
            # loi ky thuat (BR-A1).
            log.exception("goi gemini that bai", extra={"job_id": job.job_id})
            await self._emit(job, self._faq.fallback(None), source="fallback", mode=mode)
            return
        finally:
            await self._flight.release(key)

        await self._cache.put(job.event_id, job.question, answer)
        await self._flight.publish(key, answer)
        await self._emit(job, answer, source="llm", mode=mode)

    async def _call_gemini(self, job: ChatJob, policy: degradation.Policy) -> str:
        # ---- L6: xin phep TRUOC khi goi ----
        prompt = await self._gemini.build_prompt(job, self._tools)
        est = estimate_tokens(prompt, policy.max_output_tokens)

        permit = await self._limiter.acquire(est)
        if not permit.granted:
            # KHONG sleep o day. Nack ve queue voi do tre chinh xac de consumer
            # nay ranh tay xu ly job khac (co the la job tra duoc tu cache).
            raise Retry(permit.wait_ms)

        actual_tokens = 0
        try:
            result = await self._gemini.generate(
                prompt, max_output_tokens=policy.max_output_tokens,
            )
            actual_tokens = result.total_tokens
            return result.text
        finally:
            # Hoan lai phan dat cho thua -- ke ca khi goi that bai (luc do
            # actual_tokens = 0 nen hoan toan bo). Neu bo qua buoc nay, han muc
            # se ro ri dan cho toi khi limiter chan het moi thu.
            await self._limiter.refund(permit.reserved_tokens, actual_tokens)

    async def _emit(self, job: ChatJob, answer: str, *, source: str, mode: str) -> None:
        await self._publish(job.job_id, {
            "job_id": job.job_id,
            "answer": answer,
            "source": source,       # de do ti le cache hit va hieu qua tung lop
            "mode": str(mode),
            "latency_ms": int((time.time() - job.enqueued_at) * 1000),
        })
