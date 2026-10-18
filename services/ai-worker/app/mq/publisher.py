"""Publish job AI tu API (EV-171): co xac nhan cua broker, khong mat im lang."""

from __future__ import annotations

import asyncio
import json
from datetime import UTC, datetime
from typing import Any

import aio_pika

AI_EXCHANGE = "ai.direct"


class JobPublisher:
    """Mot channel publish rieng (khong dung chung channel cua pool consumer).

    publisher_confirms + mandatory: publish chi tra ve khi broker da nhan va co queue
    nhan -- tra 202 cho khach ma job khong toi duoc hang thi khach doi mai.
    """

    def __init__(self, connection: Any, exchange: str = AI_EXCHANGE) -> None:
        self._conn = connection
        self._exchange_name = exchange
        self._channel: Any = None
        self._exchange: Any = None
        self._lock = asyncio.Lock()

    async def _ex(self) -> Any:
        if self._channel is None or self._channel.is_closed:
            async with self._lock:
                if self._channel is None or self._channel.is_closed:
                    self._channel = await self._conn.channel(publisher_confirms=True, on_return_raises=True)
                    self._exchange = await self._channel.get_exchange(self._exchange_name, ensure=False)
        return self._exchange

    async def publish(self, key: str, body: dict[str, Any], *, message_id: str) -> None:
        exchange = await self._ex()
        await exchange.publish(
            aio_pika.Message(
                json.dumps(body, ensure_ascii=False).encode(),
                content_type="application/json",
                delivery_mode=aio_pika.DeliveryMode.PERSISTENT,
                message_id=message_id,
                # ai-worker do ai_queue_lag_seconds tu timestamp nay.
                timestamp=datetime.now(UTC),
            ),
            routing_key=key,
            mandatory=True,
        )

    async def close(self) -> None:
        if self._channel is not None and not self._channel.is_closed:
            await self._channel.close()
