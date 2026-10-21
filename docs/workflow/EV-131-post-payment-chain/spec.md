# EV-131 / EV-133 / EV-155 — Thanh toán xong tự phát vé và gửi email (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-131 (relay outbox), EV-133 (phát vé), EV-155 (email xác nhận) |
| Kế hoạch | [12-ke-hoach-demo-backend.md](../../12-ke-hoach-demo-backend.md) — bước 2 (G2, G3) |
| Branch | `feature/EV-131-post-payment-chain`, chồng trên `feature/EV-104-compose-all-services` |
| Quy tắc | Bất biến 1 (ghi trạng thái + outbox cùng transaction), BR-O8 (vé ký HMAC, phát đúng một lần) |

## 1. Hiện trạng

Webhook đặt đơn `PAID` và ghi `order.paid` vào `outbox_events`, rồi dừng ở đó:

- payment không có relay; relay ticketing chỉ lấy tiền tố `ticketing.`, nên `order.paid` nằm mãi trong outbox;
- không ai consume `order.paid`: vé chỉ phát khi gọi tay `POST /v1/orders/{id}/issue-tickets`;
- notification tạo consumer rồi bỏ (`_ = NewNotificationEventConsumer(...)`), không kết nối RabbitMQ;
- message của payment/ticketing không có `recipient_email`, trong khi notification bắt buộc trường này: nối thẳng thì mọi message vào DLQ.

Hai lỗi ngầm phát hiện khi thiết kế:

1. **Relay kẹt vì key không có hàng nhận.** Publisher bật `mandatory`; `ticketing.hold.created` không có binding nào nên broker trả về,
   relay dừng ở dòng lỗi đầu tiên (giữ thứ tự) và **mọi** sự kiện ticketing phía sau, kể cả `ticket.issued`, kẹt vĩnh viễn.
2. **Mất routing key sau retry.** Hàng trễ dead-letter message về hàng gốc với routing key của hàng trễ. Consumer chọn nhánh theo
   `RoutingKey` (notification) sẽ thấy một key lạ sau lần thử lại đầu tiên và đẩy message vào DLQ.

## 2. Thiết kế

```
webhook ──tx──> orders=PAID + outbox order.paid
  payment relay ──> payment.events ──┬─> ticketing.order-paid.q ──> phát vé (idempotent) ──tx──> ISSUED + outbox ticketing.ticket.issued
                                     └─> notification.events.q ──> email xác nhận thanh toán
  ticketing relay ──> ticketing.events ──> notification.events.q ──> email vé
```

**`libs/go/mq`**
- `PaymentTopology()`: exchange topic `payment.events`.
- Mỗi exchange sự kiện có `<exchange>.sink.q` bind `#` (quorum, giữ 1000 bản mới nhất, `drop-head`): mọi routing key luôn có ít nhất
  một hàng nhận, relay không bao giờ kẹt vì một sự kiện chưa có consumer.
- Consumer có retry theo mẫu AI, nhưng hàng trễ dead-letter về **default exchange** với key = tên hàng gốc: message chỉ quay lại đúng
  consumer đã lỗi, không lặp sang consumer khác cùng nghe exchange nguồn.
  - `ticketing.order-paid.q` ← `payment.events/order.paid`; trễ 5s; DLQ `ticketing.dlq`.
  - `notification.events.q` ← `order.paid`, `payment.refunded`, `ticketing.ticket.issued`; trễ 10s; DLQ `notification.dlq`.
- Header `x-ef-routing-key` giữ routing key lúc publish lần đầu; `Delivery.RoutingKey` trả key này sau retry/DLQ.
- Topology tham chiếu chéo nên mỗi service khai báo `Merge` các topology nó chạm tới (exchange nguồn phải có trước binding).

**payment** — `internal/worker.OutboxRelay`: lọc `routing_key = ANY(order.paid, hold.released, payment.refunded)` (payment dùng chung
`aggregate_type 'order'` với ticketing nên lọc theo aggregate là không đủ), `FOR UPDATE SKIP LOCKED`, đánh `published_at` chỉ sau khi
broker ack, dừng ở dòng lỗi đầu tiên, `MessageID` = id outbox. Chạy khi có `RABBITMQ_URL` và Postgres.

**ticketing-worker** — `OrderPaidHandler` gọi đúng use case của `issue-tickets` (idempotent theo `order_id`). Lỗi vĩnh viễn (vào DLQ,
không retry): payload hỏng, đơn không tồn tại, đơn không còn `PAID` (vd đã hoàn tiền: không được phát vé). Lỗi khác retry tối đa 5 lần.
Worker dùng cùng `HMAC_QR_SECRET` với server để QR phát ở đây qua được `POST /v1/tickets/verify`.

**notification** — consumer thật trên `notification.events.q`. Message thiếu email/tên sự kiện thì tra Postgres
(`identities.email`, `events.title`) theo `identity_id`/`event_id`: PII không đi qua message bus. Lỗi tra cứu là lỗi tạm thời (retry);
không tìm thấy identity thì use case từ chối vì thiếu người nhận (DLQ).

**Mất kết nối RabbitMQ:** payment, notification thoát process, ticketing-worker dừng; compose (`restart: unless-stopped`) khởi động lại.
Relay/consumer chạy trên kênh đã đóng sẽ lỗi mãi mà không tự nối lại.

## 3. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Webhook thành công → trong ≤ 5 giây đơn `ISSUED`, có vé, không gọi tay `issue-tickets` |
| AC-2 | Relay payment chỉ phát dòng của payment, sang `payment.events`, giữ thứ tự id, broker lỗi thì thử lại dòng đó ở chu kỳ sau |
| AC-3 | `order.paid` giao lặp không tạo vé thứ hai |
| AC-4 | Lỗi tạm thời được retry và quay về đúng hàng gốc với routing key gốc; consumer khác không nhận bản lặp |
| AC-5 | Payload hỏng / đơn không tồn tại / đơn không còn `PAID` vào DLQ, không retry |
| AC-6 | Routing key chưa có consumer không làm kẹt relay |
| AC-7 | Khách nhận email xác nhận thanh toán và email vé dù message không mang `recipient_email` |
| AC-8 | QR phát từ worker được `POST /v1/tickets/verify` chấp nhận; QR sửa đổi bị từ chối |

## 4. Ngoài phạm vi

- Email vé hiện hiển thị chữ ký QR dạng chữ (template có sẵn), chưa có ảnh QR. Khách lấy `qr_payload` qua `POST /v1/orders/{id}/issue-tickets`
  (idempotent).
- Store chống gửi trùng của notification vẫn là bộ nhớ: khởi động lại giữa hai lần giao một message có thể gửi email lặp.
- `hold.released` của payment (huỷ đơn) chỉ vào sink: chưa có consumer trả kho Redis; job đối soát (EV-132) sửa lệch trong ~20 giây.
- `ticketing.hold.released.q` chưa có consumer nên message tích lại (có từ trước, không đổi ở đây).
