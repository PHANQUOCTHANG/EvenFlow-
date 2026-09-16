"""Singleflight -- gop cac cau hoi trung nhau dang cho (L4).

Tinh huong that: dung giay 20:00:05, mot nghin nguoi cung go "con bao lau nua".
Neu moi nguoi mot lan goi Gemini, ta dot sach quota trong 1 giay va 940 nguoi
nhan cau tra loi giong het nhau.

Voi singleflight: mot nguoi di goi, 999 nguoi con lai doi va dung chung ket qua.

Khac voi cache: cache xu ly cac cau hoi den SAU KHI da co cau tra loi;
singleflight xu ly cac cau hoi den TRONG LUC cau tra loi dang duoc tao.
Hai co che bu tru cho nhau, va lop nao thieu thi lop kia khong lap duoc.

Dung Redis chu khong phai bien trong process, vi cac request cua cung mot cau
hoi duoc chia deu ve nhieu replica worker khac nhau.
"""

from __future__ import annotations

import asyncio
import logging
import time

from redis.asyncio import Redis

log = logging.getLogger(__name__)

LEADER_TTL_SECONDS = 30  # phai dai hon timeout goi Gemini, nhung du ngan de mot
                         # worker chet khong khoa cau hoi do qua lau


class SingleFlight:
    def __init__(self, redis: Redis) -> None:
        self._redis = redis

    async def acquire(self, key: str) -> bool:
        """Thu gianh quyen la nguoi di goi. True = ban di goi, False = ban doi."""
        return bool(await self._redis.set(f"{key}:flight", "1", nx=True, ex=LEADER_TTL_SECONDS))

    async def wait(self, key: str, timeout: float = 8.0) -> str | None:
        """Doi ket qua cua nguoi dan dau.

        Poll thay vi pub/sub la co y: ket qua duoc ghi vao mot khoa binh thuong,
        nen nguoi den muon (sau khi ket qua da co) van lay duoc -- trong khi
        pub/sub thi da lo mat thong bao.
        """
        deadline = time.monotonic() + timeout
        delay = 0.05
        while time.monotonic() < deadline:
            if (val := await self._redis.get(f"{key}:result")) is not None:
                return val.decode() if isinstance(val, bytes) else val
            await asyncio.sleep(delay)
            delay = min(delay * 1.5, 0.5)  # backoff de khong quay Redis
        log.debug("singleflight het gio cho", extra={"key": key})
        return None

    async def publish(self, key: str, answer: str) -> None:
        await self._redis.setex(f"{key}:result", 60, answer)

    async def release(self, key: str) -> None:
        await self._redis.delete(f"{key}:flight")
