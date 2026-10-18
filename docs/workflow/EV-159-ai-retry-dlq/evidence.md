# EV-159 — Bằng chứng

Ngày chạy: 2026-10-19. Máy: Windows 11, Docker Desktop 29.6.2, Python 3.13.15, uv 0.9.30.
Nhánh `feature/EV-159-ai-retry-dlq` từ `develop` @ `23df148`. Review và thử phá do người hiện thực tự làm — cần 1
reviewer khi mở PR.

## 1. Kết quả

| Kiểm | Kết quả |
|---|---|
| `uv run ruff check .` | All checks passed |
| `uv run pytest -q` | **151 passed** (baseline 113) |
| `uv run pytest -m integration -q` (RabbitMQ 3.13 + Redis 7) | **14 passed** (baseline 13) |
| `promtool check rules alerts.yml` (`prom/prometheus:v3.5.0`) | SUCCESS: 10 rules — **lần đầu file này được kiểm** |
| `promtool test rules alerts_test.yml` | SUCCESS |
| `.github/scripts/tests/test-workflows.sh` | 137 PASS, 0 FAIL |
| actionlint 1.7.7 | sạch |

## 2. Hành vi trên broker thật

`KiemDuyet_Callback404_VaoDLQNgayVaDoSauDLQ`: event service trả 404 cho callback → **1 lần gọi duy nhất**, message
vào `ai.dlq` với `x-ef-attempt = 1`, `x-ef-error = "permanent: HTTPStatusError…"`, `ai_dlq_depth ≥ 1`,
`ai_dead_lettered_total{queue="ai.moderation.q", reason="permanent"} ≥ 1`. Trước slice: 3 lần gọi qua
`ai.moderation.retry` (30 giây × 2) rồi mới vào DLQ, không có metric, không alert.

Lỗi tạm thời tự phục hồi (AC-1) vẫn được giữ bởi `LoiTam_TuPhucHoiQuaHangTre` (lỗi lần 1, 2 → thành công lần 3 qua
`ai.chat.retry`).

## 3. Thử phá

| # | Đột biến | Kết quả | Bắt bởi |
|---|---|---|---|
| E1 | Coi 429 là lỗi vĩnh viễn | Đỏ | `test_is_permanent_LoiTamThoiThiThuLai[429]` |
| E2 | `decide` chỉ xét `PermanentError` (như trước slice) | Đỏ | `test_decide_LoiVinhVienTuThuVienVaoDLQNgay` (và test tích hợp 404) |
| E3 | Đếm DLQ trước khi broker xác nhận bản sao | Đỏ | `test_DLQ_ChuyenTiepLoi_KhongDem` |
| E4 | Bỏ xét chuỗi `__cause__` | Đỏ | `test_is_permanent_LoiBocTrongCause` |
| E5 | Alert DLQ kêu ngay (`for: 0m`) | Đỏ | `promtool test rules` (case 30s phải im) |
| E6 | Ngưỡng spike 100/phút | Đỏ | `promtool test rules` (20/phút phải kêu) |
| E7 | Sampler không đo `ai.dlq` | Đỏ | `test_Sampler_DoSauDLQ` |

## 4. Giới hạn

- Compose chưa có Alertmanager: alert hiển thị trên Prometheus (`:9090/alerts`), chưa gửi đi đâu.
- Kiểm duyệt: callback event lỗi 5xx sau khi đã có verdict thì lần thử lại gọi Gemini lần nữa (tốn quota) — cần lưu
  verdict tạm, ghi nợ trong spec §5.
