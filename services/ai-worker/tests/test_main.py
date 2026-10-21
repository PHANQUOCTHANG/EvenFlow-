"""Lap rap 4 pool tu cau hinh (AC-3, AC-9)."""

from __future__ import annotations

import pytest

from app.config import Settings
from app.main import build_specs
from app.mq.protocol import PermanentError
from app.mq.runtime import Delivery


async def _chat(d: Delivery) -> None:
    return None


async def _mod(d: Delivery) -> None:
    return None


def test_BonPoolDungSoConsumer() -> None:
    s = Settings(_env_file=None, chat_consumers=20, moderation_consumers=5,
                 anomaly_consumers=2, report_consumers=1)
    specs = build_specs(s, chat=_chat, moderation=_mod)

    assert [(p.key, p.queue, p.concurrency) for p in specs] == [
        ("chat", "ai.chat.q", 20),
        ("moderation", "ai.moderation.q", 5),
        ("anomaly", "ai.anomaly.q", 2),
        ("report", "ai.report.q", 1),
    ]
    assert specs[0].handler is _chat
    assert specs[1].handler is _mod


@pytest.mark.parametrize(("key", "issue"), [("anomaly", "EV-167"), ("report", "EV-168")])
async def test_ChuaCoHandler_VaoDLQVoiLyDo(key, issue) -> None:
    specs = {p.key: p for p in build_specs(Settings(_env_file=None), chat=_chat, moderation=_mod)}
    d = Delivery(body=b"{}", headers={}, message_id=None, routing_key=key, attempt=1)

    with pytest.raises(PermanentError, match=issue):
        await specs[key].handler(d)


def test_PoolBangKhongVanCoSpecDeRuntimeBoQua() -> None:
    specs = build_specs(Settings(_env_file=None, anomaly_consumers=0, report_consumers=0),
                        chat=_chat, moderation=_mod)
    assert [p.concurrency for p in specs if p.key in ("anomaly", "report")] == [0, 0]


def test_ApiTroLyDuocGanVaoApp() -> None:
    from app.main import app

    # FastAPI 0.14x boc router con (_IncludedRouter): doc tu OpenAPI thay vi app.routes.
    paths = set(app.openapi()["paths"])
    assert "/v1/events/{event_id}/assistant/messages" in paths
    assert "/v1/assistant/messages/{job_id}" in paths
    assert "/v1/assistant/messages/{job_id}/stream" in paths


async def test_ConsumerKiemDuyetGoiEventApiBase_KhongQuaGateway() -> None:
    from app.gemini.client import GeminiClient
    from app.main import build_moderation_consumer

    s = Settings(_env_file=None, event_api_base="http://event-svc:9000/")
    consumer = build_moderation_consumer(s, GeminiClient(api_key="", mock=True), limiter=None)
    try:
        assert consumer.verdict_url("e1") == "http://event-svc:9000/internal/v1/events/e1/ai-verdict"
    finally:
        await consumer.aclose()
