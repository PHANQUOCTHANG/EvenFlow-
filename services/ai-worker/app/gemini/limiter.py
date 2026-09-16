"""Bo dieu tiet toan cuc cho Gemini API.

Bai toan: 30.000 nguoi trong phong cho, ~5.000 cau hoi moi phut, trong khi
Gemini chi cho 60 request va 1 trieu token moi phut. Chenh lech ~80 lan.

Cach lam SAI pho bien: goi that, gap 429, roi retry. Duoi tai cao cach nay sup
do -- moi worker deu retry cung luc, tao ra dung cai dinh vua bi tu choi.

Cach lam o day: KHONG BAO GIO cham tran. Moi worker phai xin phep mot bo dem
dung chung tren Redis truoc khi goi. Bo dem nay la toan cuc, nen tang so replica
worker khong lam vo han muc -- dieu ma limiter cuc bo trong tung process khong
the dam bao.

Diem then chot thu hai: DAT CHO TOKEN TRUOC khi goi. So token that chi biet sau
khi goi xong, nen ta uoc luong (prompt + max_output_tokens), tru truoc, roi hoan
lai phan chenh. Uoc luong thua thi mat mot chut thong luong; uoc luong thieu thi
cham tran TPM -- nen luon uoc luong thua.
"""

from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import dataclass
from pathlib import Path

from redis.asyncio import Redis

log = logging.getLogger(__name__)

_SCRIPT_PATH = Path(__file__).parent.parent / "scripts" / "limiter.lua"


@dataclass(frozen=True)
class LimiterConfig:
    """Han muc cua Gemini. Dat THAP hon han muc that khoang 10-15%.

    Bien an toan nay bu cho: dong ho cac may lech nhau, uoc luong token sai so,
    va cac loi goi khong di qua limiter (retry o tang thap, health check...).
    """

    rpm: int = 55          # han that 60
    tpm: int = 900_000     # han that 1.000.000
    max_wait_ms: int = 30_000


class Permit:
    """Ket qua xin phep."""

    __slots__ = ("granted", "wait_ms", "reserved_tokens")

    def __init__(self, granted: bool, wait_ms: int = 0, reserved_tokens: int = 0) -> None:
        self.granted = granted
        self.wait_ms = wait_ms
        self.reserved_tokens = reserved_tokens


class GlobalRateLimiter:
    """Token bucket RPM + TPM dung chung giua moi replica ai-worker."""

    def __init__(self, redis: Redis, cfg: LimiterConfig | None = None) -> None:
        self._redis = redis
        self._cfg = cfg or LimiterConfig()
        self._sha: str | None = None
        self._lock = asyncio.Lock()

    async def _script(self) -> str:
        if self._sha is None:
            async with self._lock:
                if self._sha is None:
                    self._sha = await self._redis.script_load(_SCRIPT_PATH.read_text("utf-8"))
        return self._sha

    async def acquire(self, est_tokens: int) -> Permit:
        """Xin phep goi Gemini.

        Tra ve Permit(granted=False, wait_ms=N) khi chua du han muc. Goi y cho
        worker: KHONG sleep trong worker -- hay nack message ve queue voi do tre
        wait_ms. Sleep se giu message khoi queue va lam consumer khac chet doi
        trong khi minh dung khong.
        """
        sha = await self._script()
        now = int(time.time() * 1000)
        granted, wait_ms = await self._redis.evalsha(
            sha, 2, "ai:rl:req", "ai:rl:tok",
            "acquire", now, self._cfg.rpm, self._cfg.tpm, est_tokens,
        )
        if int(granted) == 1:
            return Permit(True, 0, est_tokens)
        return Permit(False, min(int(wait_ms), self._cfg.max_wait_ms))

    async def refund(self, reserved: int, actual: int) -> None:
        """Hoan lai phan token da dat cho nhung khong dung den.

        Vi luon uoc luong thua, buoc nay lay lai duoc dang ke thong luong --
        thuong 20-40% quota TPM trong thuc te.
        """
        delta = reserved - actual
        if delta <= 0:
            return
        sha = await self._script()
        await self._redis.evalsha(
            sha, 2, "ai:rl:req", "ai:rl:tok",
            "refund", int(time.time() * 1000), self._cfg.rpm, self._cfg.tpm, delta,
        )


def estimate_tokens(prompt: str, max_output_tokens: int) -> int:
    """Uoc luong token, co chu y uoc THUA.

    ~4 ky tu mot token voi tieng Anh; tieng Viet co dau ton hon nen dung he so
    3 de an toan. Cong them toan bo max_output_tokens vi day la truong hop xau
    nhat co the xay ra.
    """
    return len(prompt) // 3 + max_output_tokens + 256
