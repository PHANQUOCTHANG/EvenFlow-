"""Khai bao topology AI tu definitions.json (AC-4).

Test doc DUNG file cua repo: neu ai do sua definitions.json (va test Go so khop
voi AITopology van xanh) thi worker Python tu dong theo, khong co ban thu ba.
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from app.mq.topology import Binding, Exchange, Queue, declare, load_ai_topology

REPO_DEFINITIONS = Path(__file__).resolve().parents[4] / "deploy" / "rabbitmq" / "definitions.json"


def test_load_ai_topology_ChiLayPhanAI() -> None:
    topo = load_ai_topology(REPO_DEFINITIONS)

    assert [e.name for e in topo.exchanges] == ["ai.direct", "ai.retry", "ai.dlx"]
    assert all(e.kind == "direct" and e.durable for e in topo.exchanges)
    assert len(topo.queues) == 9  # 4 hang goc + 4 hang tre + ai.dlq
    assert all(q.name.startswith("ai.") for q in topo.queues)
    assert len(topo.bindings) == 12  # moi cong viec: direct, retry, dlx
    assert all(b.source.startswith("ai.") for b in topo.bindings)


def test_load_ai_topology_GiuNguyenThamSoQueue() -> None:
    topo = load_ai_topology(REPO_DEFINITIONS)
    by_name = {q.name: q for q in topo.queues}

    assert by_name["ai.chat.q"].arguments == {"x-queue-type": "quorum", "x-message-ttl": 60000}
    retry = by_name["ai.report.retry"].arguments
    assert retry["x-message-ttl"] == 120000
    assert retry["x-dead-letter-exchange"] == "ai.direct"
    assert retry["x-dead-letter-routing-key"] == "report"
    assert retry["x-dead-letter-strategy"] == "at-least-once"
    assert retry["x-overflow"] == "reject-publish"


def test_load_ai_topology_ThieuFileThiLoi(tmp_path: Path) -> None:
    with pytest.raises(FileNotFoundError):
        load_ai_topology(tmp_path / "khong-co.json")


def test_load_ai_topology_KhongCoPhanAIThiLoi(tmp_path: Path) -> None:
    f = tmp_path / "d.json"
    f.write_text(json.dumps({"exchanges": [], "queues": [], "bindings": []}), "utf-8")
    with pytest.raises(ValueError, match="ai"):
        load_ai_topology(f)


class _FakeExchange:
    def __init__(self, name: str) -> None:
        self.name = name


class _FakeQueue:
    def __init__(self, log: list, name: str) -> None:
        self._log = log
        self.name = name

    async def bind(self, exchange, routing_key: str) -> None:
        self._log.append(("bind", exchange.name, self.name, routing_key))


class _FakeChannel:
    def __init__(self) -> None:
        self.log: list = []
        self._exchanges: dict[str, _FakeExchange] = {}

    async def declare_exchange(self, name, type, durable):
        self.log.append(("exchange", name, type, durable))
        self._exchanges[name] = _FakeExchange(name)
        return self._exchanges[name]

    async def declare_queue(self, name, durable, arguments):
        self.log.append(("queue", name, durable, arguments))
        return _FakeQueue(self.log, name)


async def test_declare_ThuTuExchangeQueueBinding() -> None:
    topo = load_ai_topology(REPO_DEFINITIONS)
    ch = _FakeChannel()

    await declare(ch, topo)

    kinds = [entry[0] for entry in ch.log]
    # Moi exchange truoc moi queue, moi queue truoc moi binding.
    assert kinds == ["exchange"] * 3 + ["queue"] * 9 + ["bind"] * 12
    assert ("bind", "ai.dlx", "ai.dlq", "chat") in ch.log
    assert ("queue", "ai.chat.q", True, {"x-queue-type": "quorum", "x-message-ttl": 60000}) in ch.log


async def test_declare_LoiBocTenDoiTuong() -> None:
    class _Hong(_FakeChannel):
        async def declare_queue(self, name, durable, arguments):
            if name == "ai.chat.q":
                raise RuntimeError("PRECONDITION_FAILED")
            return await super().declare_queue(name, durable, arguments)

    topo = load_ai_topology(REPO_DEFINITIONS)
    with pytest.raises(RuntimeError, match="ai.chat.q"):
        await declare(_Hong(), topo)


def test_dataclass_bat_bien() -> None:
    # Topology la gia tri: khong ai sua duoc sau khi doc.
    with pytest.raises(AttributeError):
        Exchange("a", "direct", True).name = "b"  # type: ignore[misc]
    assert Queue("q", True, {}).durable
    assert Binding("s", "d", "k").routing_key == "k"
