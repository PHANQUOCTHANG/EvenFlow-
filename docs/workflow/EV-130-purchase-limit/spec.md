# EV-130 — Giới hạn mua: max_per_order và max_per_identity (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-130 (mã backlog **EVF-35**, issue 1306) — 5 SP — Highest — Sprint 1 |
| Kế hoạch tổng | [09-ke-hoach-hoan-thien-backend.md](../../09-ke-hoach-hoan-thien-backend.md) — Giai đoạn 2, việc "Giới hạn mua" và điểm chờ chốt #5 |
| Branch | `feature/EV-130-purchase-limit` (từ `develop` @ `f443e26`, sau PR #58) |
| Quy tắc nghiệp vụ | BR-O4 (giới hạn mua), BR-O3 (một hold đang hoạt động) — [01-nghiep-vu.md](../../01-nghiep-vu.md) |

## 1. Bài toán

Mỗi người được mua tối đa `max_per_order` vé trong một đơn (mặc định 4) và `max_per_identity` vé gộp trên mọi đơn của
một sự kiện (mặc định 4). Đây là lớp chống gom vé: một tài khoản không được ôm hết vé hot trong vài giây mở bán. Giống
BR-O1, quy tắc phải đúng **dưới đồng thời**, không chỉ khi gọi lần lượt.

## 2. Hiện trạng trước slice (đã kiểm trên `develop` @ `f443e26`)

Code đã có phép kiểm (`create_hold.go`, `CountPurchased`), nhưng có ba lỗi:

| # | Lỗi | Hệ quả | Bằng chứng |
|---|---|---|---|
| L1 | Lớp kiểm đếm cả đơn `HELD` của chính khách và chạy **trước** cổng Redis | Khách đang giữ 3 vé bấm mua lại 2 → 422 "vượt giới hạn" thay vì nhận lại hold cũ (trái BR-O3) | `TestCreateHold_BRO4_DangGiuGheBamMuaLaiThiNhanLaiHoldCu` đỏ trên code cũ |
| L2 | Phép kiểm chạy **ngoài transaction** | Giữa lúc đếm và lúc ghi, đơn cũ chuyển `PAYMENT_PENDING → PAID` và `Gate.Consume` xoá hold khỏi ZSET → Redis không còn chặn → khách giữ được **5–6 vé / hạn mức 4** | `TestPurchaseLimit_ThanhToanXenGiuaKhongVuotHanMuc` đỏ 3/3 với đột biến M1b (= hành vi `develop`) |
| L3 | BR-O4 ghi "gộp mọi đơn đã PAID" nhưng code đếm `HELD`; frontend cần `max_selectable` do server tính nhưng không API nào trả; hai lỗi 422 không phân biệt được | UI không cho chọn số lượng; không hiện đúng thông báo | `ticket-tier-card.tsx` prop `maxSelectable`, `hold-client.ts` gộp 422 thành `LIMIT_EXCEEDED` |

## 3. Quyết định nghiệp vụ (chốt điểm chờ #5 của docs/09)

Đơn **chiếm hạn mức**: `HELD`, `PAYMENT_PENDING`, `PAID`, `ISSUED`, `REFUNDING`. Đơn `EXPIRED`, `CANCELLED`,
`REFUNDED` **trả lại** hạn mức. `REFUNDING` vẫn chiếm vì tiền chưa hoàn xong (nếu hoàn thất bại, vé vẫn của khách).
Định nghĩa nằm một chỗ: `domain.QuotaOrderStatuses` / `domain.SettledOrderStatuses`.

## 4. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Số lượng ≤ 0 hoặc > `max_per_order` bị từ chối, không chạm tồn kho |
| AC-2 | Tổng vé chiếm hạn mức + số lượng mới > `max_per_identity` bị từ chối, không chạm tồn kho |
| AC-3 | Khách đang có hold còn hạn bấm mua lại **không bao giờ** nhận lỗi vượt hạn mức — nhận lại đúng hold đó (BR-O3) |
| AC-4 | Hạn mức đúng dưới đồng thời: thanh toán xen giữa, nhiều tab, không có thứ tự nào cho một người vượt `max_per_identity` |
| AC-5 | Bị lớp chốt từ chối thì kho Redis được trả đúng một lần, không tính là "Redis lệch" |
| AC-6 | Client phân biệt được hai lỗi 422 bằng `type`: `/problems/quantity-not-allowed`, `/problems/per-identity-limit` |
| AC-7 | `GET /v1/events/{id}/purchase-limit` trả `max_per_order`, `max_per_identity`, `used`, `max_selectable`, `has_active_hold`; không cache dùng chung |

## 5. Thiết kế: hai lớp, như chống oversell

| Lớp | Ở đâu | Đếm gì | Vai trò |
|---|---|---|---|
| Nhanh | `CreateHold`, trước Redis | `SettledOrderStatuses` (PAID, ISSUED, REFUNDING) | Chặn sớm, rẻ. Không đếm đơn đang hoạt động → bấm lại đi tới nhánh `ALREADY_HOLDING` (AC-3). Nếu vẫn vượt: tra hold đang có, có thì trả lại (chỉ tốn truy vấn trên nhánh bị từ chối) |
| Chốt | `PersistHold`, **trong transaction**, trước khi trừ kho | `QuotaOrderStatuses` | Nguồn sự thật (AC-4). Vượt → rollback `ErrPerIdentityLimit`; app trả kho Redis (AC-5) |

**Vì sao lớp chốt không cần khoá thêm.** Mọi đơn thành PAID đều từng là đơn đang hoạt động, và unique index
`uq_active_order_per_identity_event` chỉ cho mỗi người một đơn như vậy. Câu đếm trong transaction luôn thấy đơn đó
(ở `HELD`/`PAYMENT_PENDING` hoặc `PAID`); còn một đơn đang hoạt động mới chưa commit thì lệnh `INSERT orders` đụng
unique index. Ngoại lệ duy nhất: đưa đơn `EXPIRED` lên `PAID` — xem §6.

`MaxPerIdentity <= 0` trong `PersistHoldParams` bị từ chối (fail-closed): schema có `CHECK (max_per_identity > 0)`
nên 0 chỉ có thể là nơi gọi quên truyền.

API hạn mức: một truy vấn (hạn mức + tổng vé + có hold), `max_selectable = min(max_per_order, max_per_identity − used)`,
bằng 0 khi đang có hold (BR-O3: đặt giữ lúc đó chỉ nhận lại hold cũ). Danh tính tạm lấy `X-Identity-Id` như
`createHold` tới khi gateway cấp JWT (giai đoạn 3). Thêm `httpx.WriteProblemType` (tương thích ngược) để ghi `type`.

## 6. Ràng buộc để lại cho giai đoạn khác

- **Giai đoạn 6 (thanh toán):** webhook đến muộn **không được** đưa đơn `EXPIRED` lên `PAID` mà không kiểm lại hạn
  mức trong cùng transaction; vượt thì hoàn tiền. Đây là đường duy nhất lách được lớp chốt.
- **Frontend:** `hold-client.ts` nên đọc `type` để tách `LIMIT_EXCEEDED` thành hai thông báo, và gọi
  `GET /purchase-limit` để điền `maxSelectable` / `hasActiveHoldElsewhere` (EV-185). Không làm trong slice này.

## 7. Ngoài phạm vi — nợ phát hiện khi làm

**Đua hold BR-O3 (có từ trước EV-130).** Tab A giữ ghế ở Redis thành công nhưng chưa commit Postgres; tab B được Redis
báo `ALREADY_HOLDING`, `GetActiveHold` chưa thấy gì → `ErrOrderNotFound` → handler trả **503** và ghi log ERROR.
Bất biến không bị ảnh hưởng, khách thử lại là được, nhưng mỗi khách mở nhiều tab lúc mở bán sinh log ERROR giả và
503. Stress test ghi nhận 16–119 lần / 5 giây với 8 tab. Đề xuất issue riêng: trả 409 "đang xử lý, thử lại" kèm
`Retry-After`, hoặc chờ ngắn rồi đọc lại.
