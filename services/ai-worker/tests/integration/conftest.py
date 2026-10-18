"""Ha tang that cho test tich hop: RabbitMQ 3.13 (giong compose) va Redis 7.

Chay: `uv run pytest -m integration`. Khong co Docker thi FAIL (khong skip) -- mot cong
skip toan bo van xanh ma khong ai biet (cung ly do TESTFLAGS=-v ben Go).
"""

from __future__ import annotations

import asyncio
from collections.abc import AsyncIterator, Iterator
from pathlib import Path

import aio_pika
import pytest
from redis.asyncio import Redis
from testcontainers.core.container import DockerContainer
from testcontainers.core.wait_strategies import LogMessageWaitStrategy

from app.mq.topology import declare, load_ai_topology

DEFINITIONS = Path(__file__).resolve().parents[4] / "deploy" / "rabbitmq" / "definitions.json"
TOPOLOGY = load_ai_topology(DEFINITIONS)


@pytest.fixture(scope="session")
def rabbit_url() -> Iterator[str]:
    c = (
        DockerContainer("rabbitmq:3.13-management-alpine")
        .with_exposed_ports(5672)
        .waiting_for(LogMessageWaitStrategy("Server startup complete").with_startup_timeout(120))
    )
    with c:
        yield f"amqp://guest:guest@{c.get_container_host_ip()}:{c.get_exposed_port(5672)}/"


@pytest.fixture(scope="session")
def redis_url() -> Iterator[str]:
    c = (
        DockerContainer("redis:7-alpine")
        .with_exposed_ports(6379)
        .waiting_for(LogMessageWaitStrategy("Ready to accept connections").with_startup_timeout(60))
    )
    with c:
        yield f"redis://{c.get_container_host_ip()}:{c.get_exposed_port(6379)}/0"


async def _connect(url: str) -> aio_pika.abc.AbstractRobustConnection:
    last: Exception | None = None
    for _ in range(30):
        try:
            return await aio_pika.connect_robust(url)
        except Exception as exc:  # noqa: BLE001 - broker vua in log san sang nhung cong chua mo
            last = exc
            await asyncio.sleep(1)
    raise RuntimeError(f"khong ket noi duoc rabbitmq: {last}")


@pytest.fixture
async def connection(rabbit_url: str) -> AsyncIterator[aio_pika.abc.AbstractRobustConnection]:
    conn = await _connect(rabbit_url)
    async with conn.channel() as ch:
        await declare(ch, TOPOLOGY)
        # Moi test bat dau tu hang trong: test truoc de lai message thi ket qua sai.
        for q in TOPOLOGY.queues:
            queue = await ch.declare_queue(q.name, passive=True)
            await queue.purge()
    try:
        yield conn
    finally:
        await conn.close()


@pytest.fixture
async def redis(redis_url: str) -> AsyncIterator[Redis]:
    r = Redis.from_url(redis_url)
    await r.flushdb()
    try:
        yield r
    finally:
        await r.aclose()
