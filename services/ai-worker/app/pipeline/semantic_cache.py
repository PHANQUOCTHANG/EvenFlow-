"""Cache ngu nghia -- lop giam tai lon nhat cua tro ly AI (L3).

Trong mot phong cho, cau hoi lap lai den muc kho tin: "con bao lau toi luot
toi", "tat trinh duyet co mat cho khong", "con ve hang A khong". Vai chuc y
dinh khac nhau duoc dien dat theo hang nghin cach.

Cache theo chuoi ky tu chi bat duoc cac ban sao y het. Cache theo NGU NGHIA bat
duoc ca cac cach dien dat khac nhau cua cung mot y -- va do la noi phan lon
luu luong nam.

Hai tang, co chu y:
  1. Exact hash  -- 0 lan goi embedding, bat ~30% (nguoi ta hay copy cau goi y)
  2. Embedding   -- 1 lan goi embedding (re hon goi LLM rat nhieu), bat phan con lai

Nguong 0.93 la chat. Ha nguong se tang ti le hit nhung bat dau tra loi lech y
khach -- va mot cau tra loi sai ve "con ve khong" thi dat hon nhieu so voi mot
lan goi LLM.
"""

from __future__ import annotations

import hashlib
import json
import logging
import re
import unicodedata

from redis.asyncio import Redis

log = logging.getLogger(__name__)

SIMILARITY_THRESHOLD = 0.93
TTL_SECONDS = 900  # 15 phut: du dai de phu mot dot mo ban, du ngan de khong
                   # tra lai thong tin ton kho da cu


def normalize(text: str) -> str:
    """Chuan hoa de tang ti le trung o tang exact-hash.

    Bo dau, ha chu thuong, gom khoang trang, bo dau cau. "Con ve khong?" va
    "còn vé không" tro thanh cung mot khoa.
    """
    text = unicodedata.normalize("NFD", text.lower())
    text = "".join(c for c in text if unicodedata.category(c) != "Mn")
    text = re.sub(r"[^\w\s]", " ", text)
    return re.sub(r"\s+", " ", text).strip()


class SemanticCache:
    def __init__(self, redis: Redis, embedder) -> None:
        self._redis = redis
        self._embedder = embedder

    def key(self, event_id: str, question: str) -> str:
        """Khoa cache LUON gom event_id.

        Dung chung cache giua cac su kien se lam tro ly tra loi khach bang thong
        tin cua concert khac -- loi kho phat hien va rat mat mat.
        """
        h = hashlib.sha256(normalize(question).encode()).hexdigest()[:16]
        return f"ai:cache:{{{event_id}}}:{h}"

    async def get(self, event_id: str, question: str) -> str | None:
        # Tang 1: exact hash, khong ton gi.
        if (hit := await self._redis.get(self.key(event_id, question))) is not None:
            return hit.decode() if isinstance(hit, bytes) else hit

        # Tang 2: lang gieng gan nhat theo embedding.
        try:
            vec = await self._embedder.embed(question)
        except Exception:
            # Embedder hong khong duoc lam chet ca luong -- chi mat cache.
            log.exception("embed that bai, bo qua cache ngu nghia")
            return None

        neighbor = await self._nearest(event_id, vec)
        if neighbor is None:
            return None
        score, answer = neighbor
        if score < SIMILARITY_THRESHOLD:
            return None

        log.debug("cache ngu nghia hit", extra={"score": round(score, 3)})
        return answer

    async def put(self, event_id: str, question: str, answer: str) -> None:
        await self._redis.setex(self.key(event_id, question), TTL_SECONDS, answer)
        try:
            vec = await self._embedder.embed(question)
        except Exception:
            return
        await self._redis.execute_command(
            "HSET", f"ai:vec:{{{event_id}}}", self.key(event_id, question),
            json.dumps({"v": vec, "a": answer}),
        )
        await self._redis.expire(f"ai:vec:{{{event_id}}}", TTL_SECONDS)

    async def _nearest(self, event_id: str, vec: list[float]) -> tuple[float, str] | None:
        """Tim lang gieng gan nhat.

        TODO(EVF-84): thay bang Redis Vector Search (FT.SEARCH KNN). Ban quet
        tuyen tinh nay chi chap nhan duoc khi so muc nho; voi mot su kien lon no
        se tro thanh nut co chai cua chinh worker.
        """
        raw = await self._redis.hgetall(f"ai:vec:{{{event_id}}}")
        if not raw:
            return None

        best: tuple[float, str] | None = None
        for payload in raw.values():
            try:
                item = json.loads(payload)
            except (TypeError, ValueError):
                continue
            score = _cosine(vec, item["v"])
            if best is None or score > best[0]:
                best = (score, item["a"])
        return best


def _cosine(a: list[float], b: list[float]) -> float:
    if len(a) != len(b):
        return 0.0
    dot = sum(x * y for x, y in zip(a, b))
    na = sum(x * x for x in a) ** 0.5
    nb = sum(y * y for y in b) ** 0.5
    if na == 0 or nb == 0:
        return 0.0
    return dot / (na * nb)
