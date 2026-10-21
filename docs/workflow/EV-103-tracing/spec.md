# EV-103 — Một yêu cầu, một trace xuyên service (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-103 (dùng `otelx` ở identity, event, gateway; chứng minh một trace xuyên ≥ 3 service trong Jaeger) |
| Kế hoạch | [12-ke-hoach-demo-backend.md](../../12-ke-hoach-demo-backend.md) — việc tùy chọn |
| Branch | `feature/EV-103-tracing-gateway-identity-event`, chồng trên `feature/EV-135-ticket-check-in` |

## 1. Hiện trạng

Chỉ waitingroom, ticketing, ticketing-worker xuất trace. Gateway không truyền `traceparent` sang upstream nên mỗi service tự mở trace riêng. Các lời gọi
đồng bộ trong hệ thống chỉ sâu hai tầng (gateway → một service), nên **không thể** có trace ≥ 3 service nếu chỉ dựa vào HTTP. Đường dài nhất là bất đồng bộ
(webhook thanh toán → phát vé → email) và nó đứt ở mọi mắt xích: cột `outbox_events.trace_id` chứa mã request (không phải trace OpenTelemetry), relay không
truyền gì sang RabbitMQ.

## 2. Thiết kế

- `otelx.InjectHTTP`: ghi `traceparent` vào header. Gateway gọi ở bước `Rewrite` của reverse proxy; `otelx.HTTP("gateway")` đặt sau CORS (sau mọi
  middleware tạo bản sao request, theo ghi chú trong `otelx.HTTP`) và trước AccessLog.
- identity, event, payment, notification gọi `otelx.Init`; identity, event, payment bọc handler bằng `otelx.HTTP`. Compose đặt `OTEL_EXPORTER_OTLP_ENDPOINT`
  cho 4 service này.
- **Qua outbox:** `otelx.Traceparent(ctx)` / `otelx.ContextWithTraceparent(ctx, tp)`.
  - Webhook của payment ghi `traceparent` của request vào `outbox_events.trace_id` (thay mã request).
  - Relay payment và ticketing khôi phục span cha từ `trace_id` trước khi `Publish`; `libs/go/mq` đã tự tiêm `traceparent` vào header AMQP và consumer tự nối tiếp.
  - Consumer `order.paid` của ticketing ghi `traceparent` của span consume vào outbox của `ticketing.ticket.issued`, và relay ticketing làm tương tự: notification nối vào cùng trace.
  - Giá trị cũ không phải traceparent (mã request ghi trước thay đổi này) bị bỏ qua, không lỗi.

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Gateway chuyển tiếp `traceparent` cùng trace ID với request đến, span cha là span của gateway; client không gửi thì gateway khởi tạo trace |
| AC-2 | `traceparent` lưu vào outbox khôi phục được ở tiến trình khác; chuỗi rỗng hoặc sai định dạng không tạo span giả |
| AC-3 | Relay payment publish dưới đúng trace của dòng outbox; dòng cũ (mã request) vẫn phát bình thường |
| AC-4 | Trên Jaeger có trace xuyên ≥ 4 service: gateway → payment → ticketing-worker → notification |

## 4. Còn hở

- ai-worker (Python) chưa gắn tracing nên luồng kiểm duyệt AI (event → RabbitMQ → ai-worker → event) đứt ở ai-worker; relay của event-service chưa lưu traceparent.
- Gateway tin `traceparent` do client gửi (chỉ ảnh hưởng quan sát). Production nên đặt lấy mẫu `OTEL_TRACES_SAMPLER=parentbased_traceidratio` thấp.
