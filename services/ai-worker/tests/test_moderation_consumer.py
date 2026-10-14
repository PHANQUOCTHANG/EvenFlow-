from __future__ import annotations

import json
from unittest.mock import AsyncMock

import pytest

from app.consumers.moderation import ModerationConsumer
from app.domain.moderation import (
    AiVerdict,
    calculate_verdict,
)
from app.gemini.client import GeminiClient


def test_calculate_verdict_thresholds() -> None:
    # BR-E4 thresholds:
    # < 0.3 -> AUTO_APPROVED
    assert calculate_verdict(0.0) == AiVerdict.AUTO_APPROVED
    assert calculate_verdict(0.15) == AiVerdict.AUTO_APPROVED
    assert calculate_verdict(0.299) == AiVerdict.AUTO_APPROVED

    # 0.3 - 0.8 -> NEEDS_HUMAN
    assert calculate_verdict(0.3) == AiVerdict.NEEDS_HUMAN
    assert calculate_verdict(0.55) == AiVerdict.NEEDS_HUMAN
    assert calculate_verdict(0.8) == AiVerdict.NEEDS_HUMAN

    # > 0.8 -> AUTO_REJECTED
    assert calculate_verdict(0.801) == AiVerdict.AUTO_REJECTED
    assert calculate_verdict(0.95) == AiVerdict.AUTO_REJECTED
    assert calculate_verdict(1.0) == AiVerdict.AUTO_REJECTED


@pytest.mark.asyncio
async def test_moderation_consumer_auto_approved() -> None:
    gemini = GeminiClient()
    callback_mock = AsyncMock()

    consumer = ModerationConsumer(
        gemini=gemini,
        api_base="http://testserver",
        callback_fn=callback_mock,
    )

    payload = {
        "job_id": "job-123",
        "event_id": "evt-001",
        "event_title": "Đêm nhạc Trịnh Công Sơn",
        "event_description": "Đêm nhạc acoustic tôn vinh các nhạc phẩm trữ tình bất hủ.",
        "enqueued_at": 1700000000.0,
    }
    raw = json.dumps(payload).encode("utf-8")

    result = await consumer.handle_message(raw)

    assert result.verdict == AiVerdict.AUTO_APPROVED
    assert result.score < 0.3
    callback_mock.assert_awaited_once_with("evt-001", result)


@pytest.mark.asyncio
async def test_moderation_consumer_auto_rejected() -> None:
    gemini = GeminiClient()
    callback_mock = AsyncMock()

    consumer = ModerationConsumer(
        gemini=gemini,
        api_base="http://testserver",
        callback_fn=callback_mock,
    )

    payload = {
        "job_id": "job-456",
        "event_id": "evt-002",
        "event_title": "Hội thảo vũ khí tự chế",
        "event_description": "Hướng dẫn chế tạo vũ khí nguy hiểm bị cấm.",
        "enqueued_at": 1700000000.0,
    }
    raw = json.dumps(payload).encode("utf-8")

    result = await consumer.handle_message(raw)

    assert result.verdict == AiVerdict.AUTO_REJECTED
    assert result.score >= 0.8
    callback_mock.assert_awaited_once_with("evt-002", result)


@pytest.mark.asyncio
async def test_moderation_consumer_needs_human() -> None:
    gemini = GeminiClient()
    callback_mock = AsyncMock()

    consumer = ModerationConsumer(
        gemini=gemini,
        api_base="http://testserver",
        callback_fn=callback_mock,
    )

    payload = {
        "job_id": "job-789",
        "event_id": "evt-003",
        "event_title": "Biểu diễn ca nhạc nghi vấn bản quyền",
        "event_description": "Thông tin chưa rõ ràng và incomplete venue details.",
        "enqueued_at": 1700000000.0,
    }
    raw = json.dumps(payload).encode("utf-8")

    result = await consumer.handle_message(raw)

    assert result.verdict == AiVerdict.NEEDS_HUMAN
    assert 0.3 <= result.score <= 0.8
    callback_mock.assert_awaited_once_with("evt-003", result)
