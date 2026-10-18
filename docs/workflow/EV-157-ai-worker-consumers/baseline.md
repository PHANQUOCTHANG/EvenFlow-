# EV-157 — Baseline (trạng thái trước khi hiện thực)

| Mục | Giá trị |
|---|---|
| Chụp lúc | 2026-10-17, `feature/EV-157-ai-worker-consumers` = `develop` @ `c241ea1`, chưa sửa code |
| Máy | Windows 11, Docker Desktop (engine 29.6.2), Python 3.13.15, uv 0.9.30 (khớp CI) |
| Lệnh | trong `services/ai-worker`: `uv sync --all-extras --locked`, `uv run ruff check .`, `uv run pytest -q` |

## 1. Kết quả

| Lệnh | Kết quả |
|---|---|
| `uv sync --all-extras --locked` | OK — lockfile khớp `pyproject.toml` |
| `uv run ruff check .` | **All checks passed** |
| `uv run pytest -q` | **4 passed** (`tests/test_moderation_consumer.py`: ngưỡng verdict + 3 nhánh mock) |

Phiên bản đã cài (dùng để code đúng API): aio-pika 10.1.0, aiormq 7.2.1, google-genai 2.29.0, redis 8.1.0,
fastapi 0.142.4, testcontainers 4.15.0, pytest 9.1.1, pytest-asyncio 1.4.0, prometheus-client 0.26.0, httpx 0.28.1.

## 2. Test có thể bị ảnh hưởng

| File | Kiểm gì | Ảnh hưởng dự kiến |
|---|---|---|
| `tests/test_moderation_consumer.py` | `calculate_verdict`, `ModerationConsumer.handle_message` với Gemini mock | Không được gãy; `GeminiClient()` không tham số vẫn phải dựng được ở chế độ mock |
| `libs/go/mq` `TestDefinitionsJSON_KhopTopologyCode` | `definitions.json` khớp `AITopology()` | Slice này **không** sửa `definitions.json`; chỉ đọc |

Không có test nào cho `main.py`, limiter, degradation, FAQ, cache, singleflight trước slice này.

## 3. Ngưỡng

- Không có cổng coverage Python trong CI; mục tiêu tự đặt: mọi AC có test (xem `traceability.md`), `mq/protocol.py`
  phủ mọi nhánh.
- Unit test không cần mạng hay Docker; tích hợp đặt marker `integration`, chạy riêng.
