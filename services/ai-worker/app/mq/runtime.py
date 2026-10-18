"""Runtime 4 pool consumer tach biet (BR-A3, AC-1, AC-3).

Moi hang doi mot channel rieng, consumer co prefetch = so consumer cua pool. RabbitMQ
ap basic.qos (global=false) cho TUNG consumer, nen mot job report chay 2 phut chi chiem
slot cua pool report -- khong the lay slot cua chat. Channel rieng them cach ly loi:
publish chuyen tiep hong lam dong channel thi chi pool do bi anh huong.

Cac pool chung mot event loop, nen handler PHAI async va khong chan loop. Viec nang
CPU (tong hop so lieu bao cao) chay qua asyncio.to_thread.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import logging
import time
from collections.abc import Awaitable, Callable, Mapping
from dataclasses import dataclass, field
from datetime import datetime
from typing import Any

import aio_pika

from app import metrics
from app.logx import log_context, trace_id_from
from app.mq.protocol import Action, attempt_of, decide, forward_headers, is_permanent

log = logging.getLogger(__name__)

RETRY_EXCHANGE = "ai.retry"
DLX_EXCHANGE = "ai.dlx"
DLQ = "ai.dlq"
SAMPLE_INTERVAL_SECONDS = 5.0


def queue_name(key: str) -> str:
    return f"ai.{key}.q"


@dataclass(frozen=True)
class Delivery:
    """Message dua cho handler. `headers` la ban sao: handler sua khong anh huong ban chuyen tiep."""

    body: bytes
    headers: dict[str, Any]
    message_id: str | None
    routing_key: str | None
    attempt: int
    payload: dict[str, Any] = field(default_factory=dict)


# Handler tra ve None (= "ack") hoac mot nhan ket qua de dem rieng, vd "expired".
Handler = Callable[[Delivery], Awaitable[str | None]]
Forward = Callable[[Any, str, str, dict[str, Any]], Awaitable[None]]


@dataclass(frozen=True)
class PoolSpec:
    key: str           # ten cong viec: chat | moderation | anomaly | report
    concurrency: int   # so handler chay song song = prefetch cua channel
    handler: Handler

    @property
    def queue(self) -> str:
        return queue_name(self.key)


def _payload(body: bytes) -> dict[str, Any]:
    try:
        data = json.loads(body)
    except (ValueError, UnicodeDecodeError):
        return {}
    return data if isinstance(data, dict) else {}


def _enqueued_at(timestamp: Any, payload: Mapping[str, Any]) -> float | None:
    if isinstance(timestamp, datetime):
        return timestamp.timestamp()
    if isinstance(timestamp, int | float) and not isinstance(timestamp, bool) and timestamp > 0:
        return float(timestamp)
    raw = payload.get("enqueued_at")
    if isinstance(raw, int | float) and not isinstance(raw, bool) and raw > 0:
        return float(raw)
    return None


async def process(msg: Any, spec: PoolSpec, forward: Forward, stopping: Callable[[], bool]) -> str:
    """Xu ly mot message va tra ve nhan ket qua. Khop handleDelivery cua libs/go/mq."""
    queue = spec.queue
    headers = dict(msg.headers or {})
    payload = _payload(msg.body)
    attempt = attempt_of(headers)

    with log_context(
        queue=queue,
        job_id=payload.get("job_id"),
        event_id=payload.get("event_id"),
        trace_id=trace_id_from(headers),
        attempt=attempt,
    ):
        # Dang tat: message con nam trong hang noi bo -> tra lai broker, khong xu ly do dang.
        if stopping():
            await msg.nack(requeue=True)
            return _done(queue, "requeue")

        if (enq := _enqueued_at(getattr(msg, "timestamp", None), payload)) is not None:
            metrics.QUEUE_LAG.labels(queue).set(max(0.0, time.time() - enq))

        delivery = Delivery(
            body=msg.body,
            headers=dict(headers),
            message_id=getattr(msg, "message_id", None),
            routing_key=getattr(msg, "routing_key", None),
            attempt=attempt,
            payload=payload,
        )

        started = time.monotonic()
        err: Exception | None = None
        label: str | None = None
        try:
            label = await spec.handler(delivery)
        except Exception as exc:  # noqa: BLE001 - moi loi handler deu phai duoc dinh tuyen retry/DLQ
            err = exc
        finally:
            metrics.JOB_DURATION.labels(queue).observe(time.monotonic() - started)

        if err is not None and stopping():
            await msg.nack(requeue=True)
            return _done(queue, "requeue")

        action = decide(err, attempt)
        if action is Action.ACK:
            await msg.ack()
            return _done(queue, label or "ack")

        dead_reason: str | None = None
        if action is Action.RETRY:
            exchange, next_attempt, reason = RETRY_EXCHANGE, attempt + 1, None
            log.warning("loi tam thoi, thu lai", extra={"error": repr(err)})
        else:
            dead_reason = "permanent" if is_permanent(err) else "exhausted"
            exchange, next_attempt = DLX_EXCHANGE, attempt
            reason = f"{dead_reason}: {type(err).__name__}: {err}"
            log.error("chuyen vao DLQ", extra={"error": reason, "dead_reason": dead_reason})

        try:
            await forward(msg, exchange, spec.key, forward_headers(headers, next_attempt, reason))
        except Exception:
            # Ack ban goc khi ban sao chua toi broker = mat message. Tra lai hang.
            log.exception("chuyen tiep that bai, tra message lai hang")
            await msg.nack(requeue=True)
            return _done(queue, "requeue")

        if dead_reason is not None:
            # Dem SAU khi broker xac nhan ban sao: message chua toi DLQ thi alert khong duoc keu.
            metrics.DEAD_LETTERED.labels(queue, dead_reason).inc()
        await msg.ack()
        return _done(queue, action.value)


def _done(queue: str, outcome: str) -> str:
    metrics.JOBS.labels(queue, outcome).inc()
    return outcome


def amqp_forwarder(channel: Any) -> Forward:
    """Publish ban sao giu body va property (khop forwardMessage cua Go), cho broker xac nhan."""

    async def forward(msg: Any, exchange_name: str, routing_key: str, headers: dict[str, Any]) -> None:
        exchange = await channel.get_exchange(exchange_name, ensure=False)
        copy = aio_pika.Message(
            msg.body,
            headers=headers,
            content_type=msg.content_type,
            content_encoding=msg.content_encoding,
            delivery_mode=aio_pika.DeliveryMode.PERSISTENT,
            priority=msg.priority,
            correlation_id=msg.correlation_id,
            reply_to=msg.reply_to,
            message_id=msg.message_id,
            timestamp=msg.timestamp,
            type=msg.type,
            app_id=msg.app_id,
        )
        # mandatory: khong queue nao nhan thi broker tra ve -> on_return_raises nem loi.
        await exchange.publish(copy, routing_key=routing_key, mandatory=True)

    return forward


@dataclass
class _Pool:
    spec: PoolSpec
    channel: Any = None
    queue: Any = None
    consumer_tag: str | None = None
    buffer: asyncio.Queue = field(default_factory=asyncio.Queue)
    workers: list[asyncio.Task] = field(default_factory=list)
    in_flight: int = 0


class ConsumerRuntime:
    def __init__(self, connection: Any, specs: list[PoolSpec], *, grace_seconds: float) -> None:
        self._connection = connection
        self._pools = [_Pool(s) for s in specs if s.concurrency > 0]
        self._grace = grace_seconds
        self._stopping = False
        self._sampler: asyncio.Task | None = None
        self._sample_channel: Any = None

    @property
    def keys(self) -> list[str]:
        return [p.spec.key for p in self._pools]

    def in_flight(self, key: str) -> int:
        return next((p.in_flight for p in self._pools if p.spec.key == key), 0)

    async def start(self) -> None:
        for pool in self._pools:
            pool.channel = await self._connection.channel(publisher_confirms=True, on_return_raises=True)
            await pool.channel.set_qos(prefetch_count=pool.spec.concurrency)
            # passive: topology da khai bao luc khoi dong; queue khong ton tai la loi cau hinh.
            pool.queue = await pool.channel.declare_queue(pool.spec.queue, passive=True)
            forward = amqp_forwarder(pool.channel)
            pool.workers = [
                asyncio.create_task(self._work(pool, forward), name=f"{pool.spec.queue}#{i}")
                for i in range(pool.spec.concurrency)
            ]
            pool.consumer_tag = await pool.queue.consume(pool.buffer.put)
            log.info("pool san sang", extra={"queue": pool.spec.queue, "concurrency": pool.spec.concurrency})
        self._sampler = asyncio.create_task(self._sample_loop(), name="ai-queue-sampler")

    async def _work(self, pool: _Pool, forward: Forward) -> None:
        while True:
            msg = await pool.buffer.get()
            if msg is None:  # tin hieu dung
                return
            pool.in_flight += 1
            try:
                await process(msg, pool.spec, forward, lambda: self._stopping)
            except Exception:
                # process da tu xu ly loi handler; toi day la loi ack/nack (mat ket noi).
                # Broker se giao lai message khi channel dong -- khong de mot worker chet.
                log.exception("loi ack/nack", extra={"queue": pool.spec.queue})
            finally:
                pool.in_flight -= 1

    async def sample_once(self) -> None:
        """Cap nhat do sau hang doi; hang trong va khong viec dang chay thi lag = 0.

        Dung channel RIENG: khai bao passive mot queue khong ton tai lam broker dong channel
        (404). Tren channel cua pool, dieu do se giet luon pool dang nhan message.
        """
        for pool in self._pools:
            try:
                if self._sample_channel is None or self._sample_channel.is_closed:
                    self._sample_channel = await self._connection.channel()
                q = await self._sample_channel.declare_queue(pool.spec.queue, passive=True)
            except Exception:
                log.warning("khong doc duoc do sau hang doi", extra={"queue": pool.spec.queue}, exc_info=True)
                self._sample_channel = None
                continue
            count = q.declaration_result.message_count
            metrics.QUEUE_DEPTH.labels(pool.spec.queue).set(count)
            if count == 0 and pool.in_flight == 0 and pool.buffer.empty():
                metrics.QUEUE_LAG.labels(pool.spec.queue).set(0)
        try:
            if self._sample_channel is None or self._sample_channel.is_closed:
                self._sample_channel = await self._connection.channel()
            dlq = await self._sample_channel.declare_queue(DLQ, passive=True)
            metrics.DLQ_DEPTH.set(dlq.declaration_result.message_count)
        except Exception:
            log.warning("khong doc duoc do sau ai.dlq", exc_info=True)
            self._sample_channel = None

    async def _sample_loop(self) -> None:
        while True:
            await asyncio.sleep(SAMPLE_INTERVAL_SECONDS)
            await self.sample_once()

    async def stop(self) -> None:
        """Ngung nhan, cho viec dang chay toi da grace giay; message chua ack quay lai hang."""
        self._stopping = True
        if self._sampler is not None:
            self._sampler.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await self._sampler

        for pool in self._pools:
            if pool.queue is not None and pool.consumer_tag is not None:
                with contextlib.suppress(Exception):
                    await pool.queue.cancel(pool.consumer_tag)
            for _ in pool.workers:
                pool.buffer.put_nowait(None)

        workers = [w for p in self._pools for w in p.workers]
        if workers:
            _, pending = await asyncio.wait(workers, timeout=self._grace)
            for task in pending:
                task.cancel()
            if pending:
                log.warning("het thoi gian cho, huy viec dang chay", extra={"cancelled": len(pending)})
                await asyncio.gather(*pending, return_exceptions=True)

        for ch in [p.channel for p in self._pools] + [self._sample_channel]:
            if ch is not None:
                with contextlib.suppress(Exception):
                    await ch.close()
