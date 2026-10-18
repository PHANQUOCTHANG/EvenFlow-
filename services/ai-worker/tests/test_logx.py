"""Log JSON co trace_id, khong log noi dung cau hoi (AC-11)."""

from __future__ import annotations

import json
import logging

from app.logx import JsonFormatter, log_context, trace_id_from


def _format(record_msg: str, **extra) -> dict:
    record = logging.LogRecord("ai-worker", logging.INFO, __file__, 1, record_msg, None, None)
    for k, v in extra.items():
        setattr(record, k, v)
    return json.loads(JsonFormatter().format(record))


def test_trace_id_from_traceparent() -> None:
    tp = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
    assert trace_id_from({"traceparent": tp}) == "4bf92f3577b34da6a3ce929d0e0e4736"
    assert trace_id_from({"traceparent": tp.encode()}) == "4bf92f3577b34da6a3ce929d0e0e4736"


def test_trace_id_from_HongHoacThieu() -> None:
    assert trace_id_from(None) is None
    assert trace_id_from({}) is None
    assert trace_id_from({"traceparent": "rac"}) is None
    assert trace_id_from({"traceparent": "00-" + "0" * 32 + "-00f067aa0ba902b7-01"}) is None


def test_DongLogLaJsonCoNguCanhMessage() -> None:
    with log_context(queue="ai.chat.q", job_id="j-1", event_id="e-1", trace_id="t-1"):
        out = _format("xu ly xong", outcome="ack")

    assert out["msg"] == "xu ly xong"
    assert out["level"] == "INFO"
    assert out["queue"] == "ai.chat.q"
    assert out["job_id"] == "j-1"
    assert out["event_id"] == "e-1"
    assert out["trace_id"] == "t-1"
    assert out["outcome"] == "ack"


def test_NgoaiNguCanh_KhongCoTruongMessage() -> None:
    out = _format("khoi dong")
    assert "job_id" not in out


def test_KhongBaoGioGhiCauHoi() -> None:
    # PII / noi dung khach nhap khong duoc vao log, ke ca khi ai do lo truyen vao extra.
    out = _format("x", question="so dien thoai cua toi la 0909", answer="...")
    assert "question" not in out
    assert "answer" not in out


def test_LoiCoStackTrace() -> None:
    try:
        raise ValueError("hong")
    except ValueError:
        import sys

        record = logging.LogRecord("x", logging.ERROR, __file__, 1, "loi", None, sys.exc_info())
    out = json.loads(JsonFormatter().format(record))
    assert "ValueError: hong" in out["exc"]


def test_configure_LogUvicornCungLaJson() -> None:
    from app.logx import configure

    names = ("", "uvicorn", "uvicorn.error", "uvicorn.access")
    saved = {n: (logging.getLogger(n).handlers[:], logging.getLogger(n).propagate, logging.getLogger(n).level)
             for n in names}
    # Gia lap uvicorn: handler text rieng, khong propagate.
    logging.getLogger("uvicorn.access").handlers[:] = [logging.StreamHandler()]
    logging.getLogger("uvicorn.access").propagate = False
    try:
        configure()
        for n in names:
            hs = logging.getLogger(n).handlers
            assert len(hs) == 1 and isinstance(hs[0].formatter, JsonFormatter), n
        assert logging.getLogger("uvicorn.access").propagate is False  # khong in hai lan
    finally:
        for n, (hs, prop, lvl) in saved.items():
            lg = logging.getLogger(n)
            lg.handlers[:] = hs
            lg.propagate = prop
            lg.setLevel(lvl)


def test_LogConfigUvicorn_DungFormatterJson() -> None:
    # File nap bang `uvicorn --log-config` (Dockerfile): phai dictConfig duoc va dung JsonFormatter.
    import logging.config
    from pathlib import Path

    cfg = json.loads((Path(__file__).parents[1] / "app" / "logging.json").read_text("utf-8"))
    assert cfg["formatters"]["json"]["()"] == "app.logx.JsonFormatter"
    for name in ("uvicorn", "uvicorn.error", "uvicorn.access"):
        assert cfg["loggers"][name]["handlers"] == ["stdout"]
        assert cfg["loggers"][name]["propagate"] is False
    # Chi kiem ten lop resolve dung; khong ap len logging toan cuc cua pytest.
    resolved = logging.config.BaseConfigurator(cfg).resolve(cfg["formatters"]["json"]["()"])
    assert resolved is JsonFormatter


def test_KhongGhiColorMessageCuaUvicorn() -> None:
    out = _format("Uvicorn running", color_message="\x1b[1mUvicorn\x1b[0m")
    assert "color_message" not in out
