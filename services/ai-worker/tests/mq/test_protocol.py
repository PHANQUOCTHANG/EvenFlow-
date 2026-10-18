"""Giao thuc retry/DLQ phai khop libs/go/mq/consumer.go (AC-5).

Hai ben (Go va Python) cung doc va ghi header x-ef-attempt tren cung mot hang doi,
nen bang quyet dinh o day la ban sao cua TestDecide / TestAttemptOf ben Go.
"""

from __future__ import annotations

import pytest

from app.mq.protocol import (
    ATTEMPT_HEADER,
    ERROR_HEADER,
    Action,
    PermanentError,
    attempt_of,
    decide,
    forward_headers,
)


@pytest.mark.parametrize(
    ("headers", "want"),
    [
        (None, 1),
        ({}, 1),
        ({ATTEMPT_HEADER: 2}, 2),
        ({ATTEMPT_HEADER: 3}, 3),
        ({ATTEMPT_HEADER: "2"}, 2),
        ({ATTEMPT_HEADER: b"2"}, 2),
        ({ATTEMPT_HEADER: 0}, 1),
        ({ATTEMPT_HEADER: -4}, 1),
        ({ATTEMPT_HEADER: 2**31}, 1),
        ({ATTEMPT_HEADER: "abc"}, 1),
        ({ATTEMPT_HEADER: 2.0}, 1),  # Go khong nhan so thuc, Python cung khong
        ({ATTEMPT_HEADER: True}, 1),  # bool la int trong Python: True = 1
        ({ATTEMPT_HEADER: False}, 1),  # False = 0 -> < 1 -> 1
    ],
)
def test_attempt_of_KhopGo(headers, want) -> None:
    assert attempt_of(headers) == want


@pytest.mark.parametrize(
    ("err", "attempt", "want"),
    [
        (None, 1, Action.ACK),
        (None, 3, Action.ACK),
        (RuntimeError("tam thoi"), 1, Action.RETRY),
        (RuntimeError("tam thoi"), 2, Action.RETRY),
        (RuntimeError("tam thoi"), 3, Action.DEAD_LETTER),
        (RuntimeError("tam thoi"), 9, Action.DEAD_LETTER),
        (PermanentError("vinh vien"), 1, Action.DEAD_LETTER),
    ],
)
def test_decide_BangQuyetDinh(err, attempt, want) -> None:
    assert decide(err, attempt) == want


def test_decide_LoiConCuaPermanentCungVaoDLQ() -> None:
    class SaiDinhDang(PermanentError):
        pass

    assert decide(SaiDinhDang("json hong"), 1) == Action.DEAD_LETTER


def test_forward_headers_BoHeaderCuaBrokerVaDatAttempt() -> None:
    src = {
        "traceparent": "00-abc-def-01",
        "x-death": [{"count": 1}],
        "x-delivery-count": 2,
        "x-first-death-queue": "ai.chat.q",
        "x-last-death-reason": "expired",
        "x-ef-publish-id": "p-1",
        ATTEMPT_HEADER: 1,
    }
    out = forward_headers(src, 2)

    assert out == {"traceparent": "00-abc-def-01", ATTEMPT_HEADER: 2}
    # Ban goc khong bi sua: handler va ban chuyen tiep khong duoc dung chung dict.
    assert src[ATTEMPT_HEADER] == 1
    assert "x-death" in src


def test_forward_headers_GhiLyDoKhiVaoDLQ() -> None:
    out = forward_headers({}, 3, error="PermanentError: json hong")
    assert out[ATTEMPT_HEADER] == 3
    assert out[ERROR_HEADER] == "PermanentError: json hong"


def test_forward_headers_LyDoQuaDaiBiCat() -> None:
    out = forward_headers({}, 1, error="x" * 5000)
    assert len(out[ERROR_HEADER]) <= 512


def test_forward_headers_NoneAnToan() -> None:
    assert forward_headers(None, 1) == {ATTEMPT_HEADER: 1}
