# EV-159 — Baseline

| Mục | Giá trị |
|---|---|
| Chụp lúc | 2026-10-19, `feature/EV-159-ai-retry-dlq` = `develop` @ `23df148` (sau PR #67) |
| Máy | Windows 11, Docker Desktop 29.6.2, Python 3.13.15, uv 0.9.30 |

| Lệnh (`services/ai-worker`) | Kết quả |
|---|---|
| `uv run ruff check .` | All checks passed |
| `uv run pytest -q` | 113 passed, 13 deselected |
| `uv run pytest -m integration -q` | 13 passed (CI Linux của PR #67: 13 passed) |

Test có thể bị ảnh hưởng: `tests/mq/test_protocol.py` (bảng `decide`), `tests/mq/test_runtime.py` (DLQ), tích hợp
`HetLuot_VaoDLQ…`, `LoiVinhVien_VaoDLQNgay`. `deploy/observability/alerts.yml` chưa từng được kiểm bằng `promtool`.
