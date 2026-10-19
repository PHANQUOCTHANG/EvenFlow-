"""Unit tests cho tang FAQ (EV-160)."""

from __future__ import annotations

from unittest.mock import AsyncMock

from app.pipeline.faq import FALLBACK, FaqMatcher


def test_FaqMatcher_Detect_Intents() -> None:
    matcher = FaqMatcher()

    # Old intents
    assert matcher.detect("tôi ở vị trí nào?") is not None
    assert matcher.detect("tôi ở vị trí nào?") .name == "queue_position"
    assert matcher.detect("còn vé không shop") .name == "ticket_availability"
    assert matcher.detect("tắt trình duyệt có mất chỗ không") .name == "lose_spot"
    assert matcher.detect("giữ ghế bao lâu vậy") .name == "hold_duration"
    assert matcher.detect("mua được mấy vé?") .name == "max_tickets"
    assert matcher.detect("có được hoàn vé không") .name == "refund_policy"
    assert matcher.detect("thanh toán bằng momo được ko") .name == "payment_methods"
    assert matcher.detect("vào sớm có lợi không") .name == "fairness"

    # New intents (EV-160)
    assert matcher.detect("sự kiện diễn ra ở đâu thế?") .name == "event_info"
    assert matcher.detect("ngày tổ chức là khi nào?") .name == "event_info"
    assert matcher.detect("thanh toán thất bại thì làm sao") .name == "payment_failure"
    assert matcher.detect("bị từ chối thẻ rồi") .name == "payment_failure"
    assert matcher.detect("dùng 2 tài khoản mua được không") .name == "multiple_accounts"
    assert matcher.detect("chồng tôi mua thêm có bị sao ko") .name == "multiple_accounts"

    # Unrecognized question
    assert matcher.detect("thời tiết hôm nay thế nào") is None


async def test_FaqMatcher_Match_StaticAnswers() -> None:
    matcher = FaqMatcher()
    tools = AsyncMock()

    # payment_failure (new)
    res = await matcher.match("lỗi thanh toán rồi bạn ơi", "e-1", "tok-1", tools)
    assert res is not None
    assert "10 phut de ban thu lai" in res

    # multiple_accounts (new)
    res2 = await matcher.match("dùng 2 tài khoản để mua vé", "e-1", "tok-1", tools)
    assert res2 is not None
    assert "moi ca nhan" in res2


async def test_FaqMatcher_Match_EventInfo() -> None:
    matcher = FaqMatcher()
    tools = AsyncMock()
    tools.event_context = AsyncMock(return_value="title: Concert A\nvenue: My Dinh")

    res = await matcher.match("sự kiện diễn ra ở đâu?", "e-1", "tok-1", tools)
    assert res is not None
    assert "Thong tin su kien:\ntitle: Concert A\nvenue: My Dinh" in res
    tools.event_context.assert_awaited_once_with("e-1")

    # Empty context returns FALLBACK
    tools.event_context = AsyncMock(return_value="")
    res_empty = await matcher.match("địa điểm sự kiện ở đâu?", "e-1", "tok-1", tools)
    assert res_empty == FALLBACK


async def test_FaqMatcher_Match_TemplateFormattingSafe() -> None:
    matcher = FaqMatcher()
    tools = AsyncMock()

    # Successful format
    tools.event_config = AsyncMock(return_value={"max_per_identity": 4})
    res = await matcher.match("mua tối đa mấy vé?", "e-1", "tok-1", tools)
    assert res is not None
    assert "4 ve" in res

    # Exception during event_config -> fallback to raw template instead of crashing
    tools.event_config = AsyncMock(side_effect=RuntimeError("connection error"))
    res_err = await matcher.match("mua tối đa mấy vé?", "e-1", "tok-1", tools)
    assert res_err is not None
    assert "{max_per_identity}" in res_err


async def test_FaqMatcher_Match_UnmatchedReturnsNone() -> None:
    matcher = FaqMatcher()
    tools = AsyncMock()
    res = await matcher.match("cho tôi hỏi chuyện riêng", "e-1", "tok-1", tools)
    assert res is None


def test_FaqMatcher_FallbackNotice() -> None:
    matcher = FaqMatcher()
    assert matcher.fallback(None) == FALLBACK
    notice = "Hệ thống đang bảo trì"
    assert matcher.fallback(notice) == f"{FALLBACK}\n\n{notice}"
