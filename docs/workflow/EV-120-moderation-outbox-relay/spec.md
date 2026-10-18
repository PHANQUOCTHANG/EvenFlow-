# EV-120 (bugfix) — Job kiểm duyệt AI chưa bao giờ tới worker (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-120 (mã backlog **EVF-23**) — sửa lỗi sau khi story đã merge (PR #59) |
| Branch | `bugfix/EV-120-moderation-outbox-relay`, **xếp chồng** lên `feature/EV-157-ai-worker-consumers` (cần worker EV-157 để có đường đi đầu-cuối) |
| Phát hiện bởi | EV-157 spec, mục O1, O2; O4, O5 phát hiện khi đọc mã để sửa |
| Quy tắc | Bất biến 1 (mọi thay đổi trong transaction), BR-E4 (kiểm duyệt AI), [03 §7](../../03-cau-truc-src.md) (event MQ) |
| Tier | **Critical**: chạm relay outbox của ticketing |

## 1. Hiện trạng (đã kiểm trên `develop` @ `c241ea1`)

| # | Lỗi | Hệ quả |
|---|---|---|
| O1 | `enqueue_moderation.go` ghi outbox với routing key `ai.moderation.q` (tên queue). Binding của `ai.direct` là `moderation` (`libs/go/mq.AITopology`). Test `TestEnqueueModerationUseCase` **khẳng định đúng key sai này** | Có relay thì message vẫn bị broker trả về: không job kiểm duyệt nào vào hàng |
| O2 | Event service không có outbox relay | Job nằm yên trong `outbox_events`; sự kiện kẹt ở `PENDING_REVIEW` |
| O4 | `outbox_events` dùng chung mọi service (một database). Relay ticketing (EV-131) lấy **mọi** dòng chưa phát, đẩy hết vào `ticketing.events`, và dừng lô ở dòng lỗi đầu tiên | Một dòng `moderation` của event → không có binding trên `ticketing.events` → `ErrUnroutable` → mọi chu kỳ kẹt đúng dòng đó → **toàn bộ outbox ticketing ngừng** (kể cả `hold.created`). Chưa lộ chỉ vì event service chưa chạy trong compose |
| O6 | Payment (EV-151/153, merge sau khi slice bắt đầu) ghi `order.paid`, `hold.released` với **cùng** aggregate type `order` như ticketing | Trên `develop` @ `2079585`, webhook thanh toán đầu tiên làm relay ticketing kẹt đúng như O4 — lỗi đang có thật. Lọc theo `aggregate_type` (bản sửa đầu của slice) **không đủ** |
| O5 | Handler `SubmitEvent` gọi `enqueueModUC.Execute` **sau** khi transition đã lưu, ghi bằng kết nối riêng (không chung transaction), và **nuốt lỗi** (`_ = …`) | Ghi outbox hỏng → sự kiện đã `PENDING_REVIEW` nhưng không có job → kẹt mãi. Vi phạm bất biến 1 |

## 2. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Job kiểm duyệt dùng routing key `moderation`, aggregate type `event`; payload đúng hợp đồng `ModerationJob` của ai-worker (`job_id`, `event_id`, `event_title`, `event_description`, `enqueued_at`) |
| AC-2 | Chuyển `PENDING_REVIEW` và ghi job nằm trong **một** transaction: hỏng giữa chừng thì không ghi gì cả; lỗi trả về cho người gọi, không nuốt |
| AC-3 | Event service có relay: chỉ lấy dòng `aggregate_type = 'event'`, phát qua `ai.direct` với publisher confirm, đánh dấu `published_at` sau khi broker xác nhận; `MessageID` = id outbox, `Timestamp` = `created_at` (ai-worker đo lag) |
| AC-4 | Routing key không có tuyến → bỏ qua dòng đó (log lỗi), **không** chặn dòng hợp lệ phía sau |
| AC-5 | Relay ticketing chỉ lấy dòng có routing key `ticketing.*` (quy ước `<domain>.<entity>.<action>`); dòng của service khác — kể cả payment dùng chung aggregate type `order` — không bị phát, không bị đánh dấu, không làm kẹt dòng của ticketing |
| AC-6 | Dòng của service khác nhiều hơn một lô đứng trước không làm relay event "đói" (không bao giờ tới lượt dòng của mình) |
| AC-7 | Không phá EVF-39: `TestNoOversell` xanh |

## 3. Thiết kế

- **Port** `EventRepository.UpdateWithOutbox(ctx, evt, msgs ...OutboxMessage)`: adapter Postgres mở một transaction,
  `UPDATE events` rồi `INSERT outbox_events` từng message, commit một lần. Bỏ `OutboxRepository` và
  `EnqueueModerationUseCase` (ghi rời, không transaction).
- **App** `SubmitForReview` tạo `ModerationJobMessage(evt, now, jobID)` và gọi `UpdateWithOutbox`. Hằng số
  `OutboxAggregateEvent = "event"`, `ModerationRoutingKey = "moderation"` ở `app/moderation_job.go`.
- **Relay** `internal/worker/outbox_relay.go` (database/sql): cùng mô hình với relay ticketing — `FOR UPDATE SKIP
  LOCKED`, đánh dấu sau confirm, dừng lô khi publish lỗi để giữ thứ tự — cộng lọc `aggregate_type` và bảng định
  tuyến `routing key → exchange`. Chạy trong `cmd/server` khi có `RABBITMQ_URL` (quy ước giống ticketing worker);
  không có thì cảnh báo, job vẫn nằm an toàn trong outbox.
- **Relay ticketing**: thêm `OutboxRelayConfig.RoutingKeyPrefix` (mặc định `ticketing.`, rỗng cũng về mặc định —
  không bao giờ là "mọi dòng"), lọc `starts_with(routing_key, $2)`. Bản đầu lọc `aggregate_type`; đổi sau khi
  rebase lên `develop` có EV-151 (O6).
- **Hệ quả cho payment**: dòng `order.paid` / `hold.released` của payment giờ không còn bị relay ticketing đẩy
  nhầm (và làm kẹt), nhưng cũng **chưa có ai phát** — payment cần relay riêng (đúng quy ước tên
  `payment.*`). Việc của nhóm thanh toán; ghi ở §4.

Không thêm migration (không cần cột mới). Không đụng `libs/go/outbox` của PR #55 — quyết định gộp hai bản outbox
(X1 trong kế hoạch) vẫn để đội chốt; lọc `aggregate_type` là điều kiện cần cho bất kỳ bản nào.

## 4. Ngoài phạm vi

- Relay cho payment và đổi routing key payment sang `payment.order.paid` / `payment.hold.released` (O6) — trước
  bản sửa này các dòng đó làm kẹt outbox ticketing; sau bản sửa chúng nằm chờ relay của payment.
- Đưa event service vào `docker-compose` (X3 — cần Dockerfile, Trivy, smoke).
- Lỗi lint có sẵn trên `develop` của event (`rows.Close` không kiểm lỗi, gofmt `domain/event.go`,
  `domain/ticket_tier.go`); CI hiện chỉ chạy golangci-lint cho ticketing và waitingroom.
