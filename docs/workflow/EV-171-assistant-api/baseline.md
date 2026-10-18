# EV-171 — Baseline

| Mục | Giá trị |
|---|---|
| Chụp lúc | 2026-10-19, `feature/EV-171-assistant-api` = `develop` @ `e1c911d` (sau PR #67, #68, #69) |
| Máy | Windows 11, Docker Desktop 29.6.2, Python 3.13.15, uv 0.9.30 |

| Lệnh (`services/ai-worker`) | Kết quả |
|---|---|
| `uv run ruff check .` | All checks passed |
| `uv run pytest -q` | 151 passed, 14 deselected |
| `uv run pytest -m integration -q` | 14 passed (CI của PR #69: 14 passed) |

Chưa có endpoint trợ lý nào; `apps/web/src/lib/assistant-client.ts` mô phỏng `jobId`, stream và quota phía trình duyệt.
