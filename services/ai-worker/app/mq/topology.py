"""Khai bao topology AI luc khoi dong (AC-4).

Nguon chuan la code Go: libs/go/mq.AITopology(). deploy/rabbitmq/definitions.json la
ban sao, va test Go `TestDefinitionsJSON_KhopTopologyCode` so khop tung tham so. Python
doc ban sao do thay vi chep lai lan thu ba.

Vi sao worker tu khai bao: RabbitMQ trong compose chi mount file, KHONG nap no (EVF-7
plan-review T7). Tu volume trong, publish vao ai.direct ma chua co queue la message mat.

Khai bao cung tham so la idempotent. Khac tham so thi broker tra PRECONDITION_FAILED
va worker chet ngay -- dung y: chay voi topology lech con te hon khong chay.
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any


@dataclass(frozen=True)
class Exchange:
    name: str
    kind: str
    durable: bool


@dataclass(frozen=True)
class Queue:
    name: str
    durable: bool
    arguments: dict[str, Any] = field(default_factory=dict)


@dataclass(frozen=True)
class Binding:
    source: str
    destination: str
    routing_key: str


@dataclass(frozen=True)
class Topology:
    exchanges: tuple[Exchange, ...]
    queues: tuple[Queue, ...]
    bindings: tuple[Binding, ...]


def _is_ai(name: str) -> bool:
    return name.startswith("ai.")


def load_ai_topology(path: Path) -> Topology:
    """Doc phan ai.* cua definitions.json. Thieu file hoac khong co phan AI -> loi."""
    data = json.loads(Path(path).read_text("utf-8"))

    exchanges = tuple(
        Exchange(e["name"], e["type"], bool(e.get("durable", False)))
        for e in data.get("exchanges", [])
        if _is_ai(e["name"])
    )
    queues = tuple(
        Queue(q["name"], bool(q.get("durable", False)), dict(q.get("arguments", {})))
        for q in data.get("queues", [])
        if _is_ai(q["name"])
    )
    bindings = tuple(
        Binding(b["source"], b["destination"], b["routing_key"])
        for b in data.get("bindings", [])
        if _is_ai(b["source"]) and b.get("destination_type", "queue") == "queue"
    )
    if not exchanges or not queues:
        raise ValueError(f"{path}: khong co exchange/queue ai.* nao")
    return Topology(exchanges, queues, bindings)


async def declare(channel: Any, topo: Topology) -> None:
    """Khai bao theo thu tu exchange -> queue -> binding; loi boc kem ten doi tuong."""
    exchanges: dict[str, Any] = {}
    for e in topo.exchanges:
        try:
            exchanges[e.name] = await channel.declare_exchange(e.name, type=e.kind, durable=e.durable)
        except Exception as exc:
            raise RuntimeError(f"khai bao exchange {e.name}: {exc}") from exc

    queues: dict[str, Any] = {}
    for q in topo.queues:
        try:
            queues[q.name] = await channel.declare_queue(
                q.name, durable=q.durable, arguments=q.arguments
            )
        except Exception as exc:
            raise RuntimeError(f"khai bao queue {q.name}: {exc}") from exc

    for b in topo.bindings:
        try:
            await queues[b.destination].bind(exchanges[b.source], routing_key=b.routing_key)
        except Exception as exc:
            raise RuntimeError(
                f"binding {b.source} -> {b.destination} ({b.routing_key}): {exc}"
            ) from exc
