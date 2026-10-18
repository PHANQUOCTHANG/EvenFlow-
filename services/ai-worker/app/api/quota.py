"""Han muc 10 cau / 15 phut moi khach (BR-A2), tren Redis de dung chung giua cac replica."""

from __future__ import annotations

import asyncio
from dataclasses import dataclass
from pathlib import Path

from redis.asyncio import Redis

_SCRIPT = Path(__file__).parent.parent / "scripts" / "quota.lua"

QUOTA_LIMIT = 10
QUOTA_WINDOW_MS = 15 * 60 * 1000


@dataclass(frozen=True)
class QuotaDecision:
    allowed: bool
    used: int
    limit: int
    reset_at_ms: int


def quota_key(event_id: str, identity: str) -> str:
    # Hash tag theo su kien (docs/03 §7): moi khoa cua mot su kien nam cung shard.
    return f"ai:quota:{{{event_id}}}:{identity}"


class AssistantQuota:
    def __init__(self, redis: Redis, limit: int = QUOTA_LIMIT, window_ms: int = QUOTA_WINDOW_MS) -> None:
        self._redis = redis
        self._limit = limit
        self._window = window_ms
        self._sha: str | None = None
        self._lock = asyncio.Lock()

    async def _script(self) -> str:
        if self._sha is None:
            async with self._lock:
                if self._sha is None:
                    self._sha = await self._redis.script_load(_SCRIPT.read_text("utf-8"))
        return self._sha

    async def take(self, event_id: str, identity: str, member: str, now_ms: int) -> QuotaDecision:
        """Lay mot cau trong han muc. allowed=False thi KHONG ghi gi."""
        allowed, used, reset = await self._redis.evalsha(
            await self._script(), 1, quota_key(event_id, identity),
            now_ms, self._window, self._limit, member,
        )
        return QuotaDecision(bool(int(allowed)), int(used), self._limit, int(reset))
