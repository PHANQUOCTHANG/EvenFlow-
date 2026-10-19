# EV-154 (EVF-74) — Adversarial Code Review

## 1. Tổng quan Đánh giá (Review Summary)
- **Reviewer Role**: Senior Staff Engineer / Principal Architect
- **Target Branch**: `feature/EV-154-refund-cancelled-event`
- **Scope**: Toàn bộ thay đổi mã nguồn triển khai luồng hoàn tiền sự kiện bị huỷ theo EV-154.
- **Trạng thái**: **APPROVED** (100% Pass)

---

## 2. Checklist 4 Quy tắc Bất biến

| Quy tắc | Yêu cầu | Thực tế kiểm tra trong mã nguồn | Đánh giá |
|---|---|---|---|
| **Quy tắc 1: DB Transaction** | Mọi thay đổi dữ liệu phụ thuộc lẫn nhau phải trong 1 transaction; Không có goroutine ngầm trong tx; Outbox event nằm trong cùng tx | `ProcessRefundSuccessTx` cập nhật đồng thời bảng `orders` (`REFUNDED`), `payments` (`REFUNDED`), `tickets` (`REFUNDED`), và chèn bản ghi `outbox_events` (`payment.refunded`) vào duy nhất 1 database transaction. Không spawn bất kỳ goroutine nào trong transaction. | **ĐẠT** |
| **Quy tắc 2: Sync API** | Tuyệt đối không gọi 3rd party API (payment gateway) bên trong DB transaction; Phải có timeout | Trong `RefundCancelledEventUseCase.Execute`, lệnh gọi `gateway.RefundPayment(...)` được thực thi hoàn toàn ngoài DB transaction (sau khi `MarkOrderRefundingTx` đã commit). Lệnh gọi gateway sử dụng timeout 15s. | **ĐẠT** |
| **Quy tắc 3: Data Integrity & Invariants** | Không oversell; Khôi phục hạn mức mua vé; Chống race condition | Trạng thái `REFUNDED` được loại khỏi tập hợp `SettledOrderStatuses`, tự động giải phóng quota mua vé cho người mua hợp lệ (BR-O4). Database locking `FOR UPDATE` bảo vệ toàn vẹn trạng thái đơn hàng. | **ĐẠT** |
| **Quy tắc 4: Observability & Security** | Structured logging, trace_id, không log PII / credentials | Mọi log entry đều có cấu trúc (`slog`), mang `trace_id`, `event_id`, `order_id`. Thông tin nhạy cảm của khách hàng được che mờ qua `MaskPII()`, không có secret/token nào bị lộ. | **ĐẠT** |

---

## 3. Phân tích Chi tiết Kỹ thuật

### 3.1. Phân tích Kiến trúc Hexagonal-lite
- **Domain Layer (`domain/payment.go`)**: Thuần Go, không phụ thuộc framework, chứa đầy đủ logic nghiệp vụ (`IsRefundable`, `RefundBatchSummary`, `ProcessRefundParams`).
- **Port Layer (`port/gateway.go`, `port/repository.go`)**: Định nghĩa interface sạch sẽ, tách biệt hợp đồng nghiệp vụ với công nghệ hạ tầng.
- **Adapter Layer (`adapter/gateway`, `adapter/postgres`, `adapter/http`)**:
  - 4 cổng thanh toán (`Sandbox`, `VNPay`, `MoMo`, `Stripe`) đều hiện thực đầy đủ phương thức `RefundPayment` xử lý đặc thù mã lỗi riêng của từng cổng.
  - `PostgresRepository` và `MemoryRepository` tương thích 100% về mặt hành vi, hỗ trợ cô lập môi trường test tốc độ cao.
  - HTTP Handler chuẩn RESTful với đường dẫn `POST /v1/events/{eventID}/refund` kèm kiểm tra `Idempotency-Key`.

### 3.2. An toàn Đồng thời & Xử lý Ngoại lệ (Concurrency & Edge Cases)
1. **Trường hợp đơn hàng đã hoàn tiền trước đó (`AlreadyDone = true`)**:
   - `OrderView.IsRefundable()` trả về `false` đối với các đơn đã `REFUNDED` hoặc `CANCELLED`, use case bỏ qua một cách an toàn mà không gọi cổng gateway lần 2.
2. **Cổng thanh toán phản hồi lỗi hoặc timeout**:
   - Giao dịch hoàn tiền ghi nhận `failed` trong summary batch, gọi `ProcessRefundFailedTx` để rollback trạng thái về trạng thái ban đầu (`PAID`/`ISSUED`), cho phép retry đợt sau mà không làm kẹt đơn hàng ở trạng thái `REFUNDING`.
3. **Outbox Pattern đảm bảo Eventual Consistency**:
   - Sự kiện `payment.refunded` và `event.cancelled` được cam kết ghi nhận bền vững cùng transaction dữ liệu, Outbox Relay worker sẽ chịu trách nhiệm phát tán sang Message Broker.

---

## 4. Kết luận Review
Mã nguồn đáp ứng toàn diện các tiêu chí chất lượng cao nhất của dự án, tuân thủ nghiêm ngặt 4 Quy tắc bất biến và hoàn thành 100% 8 Acceptance Criteria của Task EV-154.

**Quyết định: APPROVE FOR MERGE**
