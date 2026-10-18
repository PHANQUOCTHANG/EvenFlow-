"""Log JSON co ngu canh message (quy tac bat bien 4, docs/07 §5).

Moi dong log trong luc xu ly mot message tu dong mang `queue`, `job_id`, `event_id`,
`trace_id` qua contextvar -- ke ca log viet o tang pipeline khong biet gi ve message.
`trace_id` lay tu header `traceparent` (W3C) ma otelx ben Go gan khi publish, nen mot
cau hoi co the lan tu gateway toi worker trong Jaeger va Grafana.
"""

from __future__ import annotations

import contextlib
import contextvars
import json
import logging
import re
from collections.abc import Iterator, Mapping
from datetime import UTC, datetime
from typing import Any

_CONTEXT: contextvars.ContextVar[dict[str, Any] | None] = contextvars.ContextVar(
    "ai_log_context", default=None
)

_TRACEPARENT = re.compile(r"^[0-9a-f]{2}-([0-9a-f]{32})-[0-9a-f]{16}-[0-9a-f]{2}$")

# Thuoc tinh co san cua LogRecord: khong chep vao JSON.
_STANDARD = frozenset(vars(logging.LogRecord("", 0, "", 0, "", None, None))) | {
    "message",
    "asctime",
    "color_message",  # uvicorn gan ban co ma mau ANSI; `msg` da du
}
# Noi dung khach nhap / tro ly tra loi: KHONG BAO GIO vao log (PII, BR-A7).
_FORBIDDEN = frozenset({"question", "answer", "prompt", "body", "event_description"})


def trace_id_from(headers: Mapping[str, Any] | None) -> str | None:
    if not headers:
        return None
    raw = headers.get("traceparent")
    if isinstance(raw, bytes):
        raw = raw.decode("ascii", errors="replace")
    if not isinstance(raw, str):
        return None
    m = _TRACEPARENT.match(raw.strip().lower())
    if m is None or m.group(1) == "0" * 32:
        return None
    return m.group(1)


@contextlib.contextmanager
def log_context(**fields: Any) -> Iterator[None]:
    merged = {**(_CONTEXT.get() or {}), **{k: v for k, v in fields.items() if v not in (None, "")}}
    token = _CONTEXT.set(merged)
    try:
        yield
    finally:
        _CONTEXT.reset(token)


class JsonFormatter(logging.Formatter):
    def format(self, record: logging.LogRecord) -> str:
        out: dict[str, Any] = {
            "ts": datetime.fromtimestamp(record.created, UTC).isoformat(timespec="milliseconds"),
            "level": record.levelname,
            "logger": record.name,
            "msg": record.getMessage(),
        }
        out.update(_CONTEXT.get() or {})
        for k, v in vars(record).items():
            if k in _STANDARD or k in _FORBIDDEN or k.startswith("_"):
                continue
            out[k] = v
        if record.exc_info:
            out["exc"] = self.formatException(record.exc_info)
        return json.dumps(out, ensure_ascii=False, default=str)


# uvicorn tu gan handler text rieng cho cac logger nay (propagate=False) luc khoi dong,
# truoc khi lifespan chay -- phai thay lai, neu khong log khoi dong / truy cap van la text.
_UVICORN_LOGGERS = ("uvicorn", "uvicorn.error", "uvicorn.access")


def configure(level: int = logging.INFO) -> None:
    handler = logging.StreamHandler()
    handler.setFormatter(JsonFormatter())
    root = logging.getLogger()
    root.handlers[:] = [handler]
    root.setLevel(level)
    for name in _UVICORN_LOGGERS:
        lg = logging.getLogger(name)
        lg.handlers[:] = [handler]
        lg.propagate = False
