"""Ket qua chat cho API tro ly doc (AC-6)."""

from __future__ import annotations

import json

from app.pipeline.results import RESULT_TTL_SECONDS, RedisResultSink


class FakeRedis:
    def __init__(self) -> None:
        self.ops: list[tuple] = []

    async def set(self, key, value, ex=None):
        self.ops.append(("set", key, value, ex))

    async def publish(self, channel, message):
        self.ops.append(("publish", channel, message))


async def test_GhiTruocRoiMoiPublish() -> None:
    r = FakeRedis()
    payload = {"job_id": "j-1", "answer": "Ban khong mat cho", "source": "faq"}

    await RedisResultSink(r).publish("j-1", payload)

    assert [op[0] for op in r.ops] == ["set", "publish"]
    _, key, value, ex = r.ops[0]
    assert key == "ai:job:{j-1}:result"
    assert ex == RESULT_TTL_SECONDS == 300
    assert json.loads(value) == payload
    assert r.ops[1][1] == "ai:job:{j-1}"
    assert json.loads(r.ops[1][2]) == payload


async def test_TiengVietGiuNguyen() -> None:
    r = FakeRedis()
    await RedisResultSink(r).publish("j-2", {"answer": "Còn vé"})
    assert "Còn vé" in r.ops[0][2]
