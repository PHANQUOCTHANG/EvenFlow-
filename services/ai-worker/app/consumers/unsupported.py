"""Handler cho cong viec chua hien thuc: anomaly (EV-167), report (EV-168).

Vao DLQ ngay voi ly do ro rang thay vi ack (mat message) hay nack (vong nong). Khi
handler that ra doi, Ops replay tu ai.dlq.
"""

from __future__ import annotations

from app.mq.protocol import PermanentError
from app.mq.runtime import Delivery, Handler


def make_handler(key: str, issue: str) -> Handler:
    async def handler(delivery: Delivery) -> str | None:
        raise PermanentError(f"cong viec '{key}' chua co handler ({issue})")

    return handler
