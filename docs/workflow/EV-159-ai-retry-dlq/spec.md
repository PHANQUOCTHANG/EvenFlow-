# EV-159 — Retry qua hàng trễ, phân loại lỗi và alert DLQ (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-159 (mã backlog **EVF-82**) — 5 SP — P0 — Sprint 3 |
| Phụ thuộc | EV-157 (merge ở PR #67) — runtime consumer và giao thức retry/DLQ đã có |
| Branch | `feature/EV-159-ai-retry-dlq` (từ `develop` @ `23df148`) |
| Kiến trúc | [02-kien-truc.md](../../02-kien-truc.md) L7; topology `libs/go/mq.AITopology` (EVF-7) |
| Tier | Standard |

## 1. Bài toán

Jira: *"Là nền tảng, tôi muốn lỗi tạm thời được tự phục hồi mà không dồn tải vào Gemini."* AC: lỗi tạm thời tự phục
hồi; lỗi vĩnh viễn vào DLQ **và bắn alert**.

Về "5s/30s/2m" trong tiêu đề: docs/02 và EVF-7 (đã review, PR #52) chốt **mỗi công việc một hàng trễ riêng với TTL
cố định** (chat 5s, moderation 30s, anomaly 30s, report 2m) thay vì tăng dần theo lần thử — để chat không phải chờ 2
phút. Slice này giữ quyết định đó; topology và đường đi retry đã có từ EV-157.

## 2. Hiện trạng (đã kiểm trên `develop` @ `23df148`)

| # | Lỗ hổng | Hệ quả |
|---|---|---|
| H1 | Mọi lỗi handler trừ `PermanentError` đều được thử lại 3 lần | Lỗi không thể tự hết (callback event trả 404 / 409 / 422, Gemini trả 400 `INVALID_ARGUMENT`) vẫn chiếm 3 lượt; với Gemini là **đặt chỗ token 3 lần** — đúng điều Jira cấm ("không dồn tải vào Gemini") |
| H2 | Không có metric nào cho `ai.dlq`, không ai được báo khi message rơi vào đó | Job chết im lặng; sự kiện có thể nằm `PENDING_REVIEW` cho tới khi có người tình cờ nhìn queue |
| H3 | CI không kiểm `deploy/observability/alerts.yml` (không `promtool`, filter `deploy` không gồm thư mục này) | Một rule sai cú pháp làm Prometheus **bỏ cả file** — mọi alert khác (kể cả `OversellGuardTriggered`) mất theo mà không ai biết |

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | *(Jira)* Lỗi tạm thời (HTTP 5xx, 408, 425, 429, timeout, mất kết nối, limiter chưa cho phép, Gemini 429/5xx) tự phục hồi qua hàng trễ |
| AC-2 | *(Jira)* Lỗi vĩnh viễn vào DLQ ngay lần đầu **và bắn alert** |
| AC-3 | Lỗi vĩnh viễn = `PermanentError`, body không giải mã được, HTTP 4xx khác 408/425/429 (callback), Gemini `ClientError` 4xx khác 408/429. Phân loại thuần (không import httpx / google-genai) để test được không cần mạng |
| AC-4 | Metric `ai_dlq_depth` (gauge, lấy mẫu như các queue khác) và `ai_dead_lettered_total{queue, reason}` với `reason ∈ {permanent, exhausted}`; header `x-ef-error` ghi loại lỗi |
| AC-5 | Alert `AiDeadLetterQueueNotEmpty` (`ai_dlq_depth > 0` trong 1 phút, warning) và `AiDeadLetterSpike` (> 10 message/phút vào DLQ trong 5 phút, critical) |
| AC-6 | CI chạy `promtool check rules` và `promtool test rules` (test cho alert mới) mỗi khi `deploy/observability/**` đổi |

## 4. Thiết kế

- `app/mq/protocol.py`: `is_permanent(err)` đọc `status_code` của `err.response` (httpx) hoặc `err.code` (google-genai)
  bằng duck typing; `decide` dùng nó thay cho chỉ `isinstance(PermanentError)`. Khớp Go ở chỗ quan trọng: Go chỉ biết
  `ErrPermanent` vì handler Go tự bọc lỗi; Python thêm phân loại vì handler gọi thư viện ngoài trực tiếp.
- `app/mq/runtime.py`: khi `DEAD_LETTER` tăng `ai_dead_lettered_total{queue, reason}`; sampler lấy thêm độ sâu `ai.dlq`.
- `deploy/observability/alerts.yml` thêm 2 alert; `deploy/observability/alerts_test.yml` là unit test của `promtool`.
- `ci.yml`: filter `deploy` thêm `deploy/observability/**`; `validate-deploy` thêm bước `promtool`.

## 5. Ngoài phạm vi

- Kiểm duyệt gọi lại Gemini khi chỉ callback event lỗi 5xx (tốn quota lần hai) — cần lưu verdict tạm; ghi nợ.
- Replay từ DLQ (công cụ cho Ops) — EV-147 / runbook.
- Alertmanager / kênh nhận alert — compose chưa có Alertmanager; alert hiện trên Prometheus UI.
