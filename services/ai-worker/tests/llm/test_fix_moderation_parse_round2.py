"""Re-review R2-2/R2-3: parse_moderation dong kin ca voi so qua lon va JSON long qua sau."""

from __future__ import annotations

import math

from app.llm.client import parse_moderation


def test_SoNguyenQuaLon_KhongNemLoi_TraNone() -> None:
    assert parse_moderation('{"score": ' + "9" * 400 + "}") is None


def test_JsonLongQuaSau_KhongNemLoi_TraNone() -> None:
    assert parse_moderation("[" * 100_000) is None
    assert parse_moderation('{"score": 0.1, "axes": ' + '{"a":' * 100_000 + "}") is None


def test_AmKhong_ThanhDuongKhong() -> None:
    result = parse_moderation('{"score": -0.0}')
    assert result is not None
    assert result.score == 0.0
    assert math.copysign(1.0, result.score) == 1.0
