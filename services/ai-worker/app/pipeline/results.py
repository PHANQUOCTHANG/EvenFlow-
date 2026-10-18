"""Noi nhan ket qua cua tro ly -- hop dong voi API tro ly (EV-171).

SET truoc, PUBLISH sau: client SSE dang ky muon (sau khi ket qua da co) van doc duoc
tu khoa thuong, cung ly do singleflight.wait poll thay vi chi nghe pub/sub.
Hash tag {job_id} theo quy uoc Redis key (docs/03 §7).
"""

from __future__ import annotations

import json
from typing import Any

from redis.asyncio import Redis

# Du dai de khach mat mang vai phut quay lai van thay cau tra loi; du ngan de khong
# giu hang trieu cau tra loi sau mot dot mo ban.
RESULT_TTL_SECONDS = 300


def result_key(job_id: str) -> str:
    return f"ai:job:{{{job_id}}}:result"


def result_channel(job_id: str) -> str:
    return f"ai:job:{{{job_id}}}"


class RedisResultSink:
    def __init__(self, redis: Redis) -> None:
        self._redis = redis

    async def publish(self, job_id: str, payload: dict[str, Any]) -> None:
        data = json.dumps(payload, ensure_ascii=False)
        await self._redis.set(result_key(job_id), data, ex=RESULT_TTL_SECONDS)
        await self._redis.publish(result_channel(job_id), data)
