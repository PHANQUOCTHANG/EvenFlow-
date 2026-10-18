"""Giao thuc retry/DLQ dung chung voi libs/go/mq (docs/02 L5, EVF-7).

File nay THUAN: khong import aio-pika, khong biet broker. Go va Python cung doc va
ghi header x-ef-attempt tren cung mot hang doi, nen moi quyet dinh o day phai khop
tung dong voi `decide`, `attemptOf`, `copyHeaders` trong libs/go/mq/consumer.go.
Doi mot ben ma khong doi ben kia la message bi retry sai so lan, hoac vao DLQ som.
"""

from __future__ import annotations

import enum
from collections.abc import Mapping
from typing import Any

ATTEMPT_HEADER = "x-ef-attempt"
# Ly do vao DLQ -- de nguoi truc doc duoc ma khong phai lan log (Go khong ghi header nay;
# Go bo qua header la nen hai ben van tuong thich).
ERROR_HEADER = "x-ef-error"
MAX_ATTEMPTS = 3

_INT32_MAX = 2**31 - 1
_ERROR_MAX_LEN = 512

# Header do broker / publisher quan ly: khong duoc mang sang ban chuyen tiep.
_DROP_EXACT = frozenset({"x-death", "x-delivery-count", "x-ef-publish-id"})
_DROP_PREFIX = ("x-first-death-", "x-last-death-")


class PermanentError(Exception):
    """Loi khong the tu het khi thu lai (body hong, thieu truong...) -> DLQ ngay.

    Tuong duong `errors.Is(err, mq.ErrPermanent)` ben Go.
    """


class Action(enum.Enum):
    ACK = "ack"
    RETRY = "retry"
    DEAD_LETTER = "dead_letter"


def attempt_of(headers: Mapping[str, Any] | None) -> int:
    """Doc so lan xu ly; thieu, hong, < 1 hoac vuot int32 thi coi la 1."""
    if not headers:
        return 1
    raw = headers.get(ATTEMPT_HEADER)
    # bool la int trong Python (True=1, False=0): roi vao nhanh so nguyen va van ra 1.
    if isinstance(raw, bytes):
        raw = raw.decode("ascii", errors="replace")
    if isinstance(raw, str):
        try:
            raw = int(raw, 10)
        except ValueError:
            return 1
    if not isinstance(raw, int):
        return 1
    if raw < 1 or raw > _INT32_MAX:
        return 1
    return raw


def decide(err: BaseException | None, attempt: int, max_attempts: int = MAX_ATTEMPTS) -> Action:
    """Thanh cong -> ACK; loi vinh vien hoac het luot -> DEAD_LETTER; con lai -> RETRY."""
    if err is None:
        return Action.ACK
    if isinstance(err, PermanentError) or attempt >= max_attempts:
        return Action.DEAD_LETTER
    return Action.RETRY


def forward_headers(
    headers: Mapping[str, Any] | None, attempt: int, error: str | None = None
) -> dict[str, Any]:
    """Ban sao header cho ban chuyen tiep retry/DLQ (khong bao gio sua ban goc)."""
    out: dict[str, Any] = {}
    for k, v in (headers or {}).items():
        if k in _DROP_EXACT or k.startswith(_DROP_PREFIX):
            continue
        out[k] = v
    out[ATTEMPT_HEADER] = attempt
    if error is not None:
        out[ERROR_HEADER] = error[:_ERROR_MAX_LEN]
    return out
