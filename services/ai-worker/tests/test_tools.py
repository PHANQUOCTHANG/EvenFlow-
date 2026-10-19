"""Unit tests for SystemTools and ToolError (EV-163)."""

from __future__ import annotations

import httpx
import pytest

from app.pipeline.faq import FALLBACK, FaqMatcher
from app.pipeline.tools import SystemTools, ToolError


@pytest.mark.asyncio
async def test_tools_queue_position_success():
    def handler(request: httpx.Request) -> httpx.Response:
        assert request.url.path == "/v1/events/evt-1/queue/status"
        assert request.headers["X-Queue-Token"] == "tok-abc-123"
        return httpx.Response(200, json={"state": "WAITING", "rank": 1500, "eta_seconds": 300})

    transport = httpx.MockTransport(handler)
    client = httpx.AsyncClient(transport=transport)
    tools = SystemTools("http://backend-api", client=client)

    res = await tools.queue_position("evt-1", "tok-abc-123")
    assert res["state"] == "WAITING"
    assert res["rank"] == 1500
    assert res["eta_seconds"] == 300


@pytest.mark.asyncio
async def test_tools_availability_bucket():
    def handler(request: httpx.Request) -> httpx.Response:
        if "few" in str(request.url):
            return httpx.Response(200, json={"bucket": "FEW"})
        return httpx.Response(200, json={})

    transport = httpx.MockTransport(handler)
    client = httpx.AsyncClient(transport=transport)
    tools = SystemTools("http://backend-api", client=client)

    # Specific bucket
    assert await tools.availability_bucket("event-few") == "FEW"
    # Fallback to MANY
    assert await tools.availability_bucket("event-default") == "MANY"


@pytest.mark.asyncio
async def test_tools_event_config():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"max_per_identity": 2, "hold_ttl_seconds": 300})

    transport = httpx.MockTransport(handler)
    client = httpx.AsyncClient(transport=transport)
    tools = SystemTools("http://backend-api", client=client)

    cfg = await tools.event_config("evt-test")
    assert cfg["max_per_identity"] == 2
    assert cfg["hold_minutes"] == 5


@pytest.mark.asyncio
async def test_tools_event_context():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "title": "Concert 2026",
                "venue": "My Dinh Stadium",
                "starts_at": "2026-11-01T19:00:00Z",
                "sale_start_at": "2026-10-20T10:00:00Z",
                "description": "Live music concert",
            },
        )

    transport = httpx.MockTransport(handler)
    client = httpx.AsyncClient(transport=transport)
    tools = SystemTools("http://backend-api", client=client)

    ctx = await tools.event_context("evt-test")
    assert "title: Concert 2026" in ctx
    assert "venue: My Dinh Stadium" in ctx
    assert "Live music concert" in ctx


@pytest.mark.asyncio
async def test_tools_http_errors_raise_tool_error():
    def handler(request: httpx.Request) -> httpx.Response:
        if "404" in str(request.url):
            return httpx.Response(404, json={"error": "Not Found"})
        if "500" in str(request.url):
            return httpx.Response(500, json={"error": "Server Error"})
        raise httpx.ConnectTimeout("Connection timed out")

    transport = httpx.MockTransport(handler)
    client = httpx.AsyncClient(transport=transport)
    tools = SystemTools("http://backend-api", client=client)

    # 404
    with pytest.raises(ToolError, match="404"):
        await tools.queue_position("evt-404", "tok")

    # 500
    with pytest.raises(ToolError, match="500"):
        await tools.availability_bucket("evt-500")

    # ConnectTimeout
    with pytest.raises(ToolError, match="Connection timed out"):
        await tools.event_config("evt-timeout")


@pytest.mark.asyncio
async def test_faq_matcher_catches_tool_error():
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(503, json={"error": "Service Unavailable"})

    transport = httpx.MockTransport(handler)
    client = httpx.AsyncClient(transport=transport)
    tools = SystemTools("http://backend-api", client=client)
    faq = FaqMatcher()

    # Live intent queue_position fails gracefully to FALLBACK instead of raising
    ans = await faq.match("toi dang o vi tri nao", "evt-1", "tok-1", tools)
    assert ans == FALLBACK

    # Live intent ticket_availability fails gracefully to FALLBACK
    ans2 = await faq.match("con ve khong", "evt-1", "tok-1", tools)
    assert ans2 == FALLBACK

    # Live intent event_info fails gracefully to FALLBACK
    ans3 = await faq.match("dien ra o dau", "evt-1", "tok-1", tools)
    assert ans3 == FALLBACK
