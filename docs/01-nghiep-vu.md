# EventFlow — Nghiệp vụ

## 1. Actor

| Actor | Mô tả | Quyền chính |
|---|---|---|
| **Khách (Buyer)** | Người mua vé | Xem sự kiện, xếp hàng, giữ ghế, thanh toán, hỏi trợ lý |
| **Ban tổ chức (Organizer)** | Chủ sự kiện | Tạo/sửa sự kiện, cấu hình hạng vé & lịch mở bán, xem doanh số |
| **Kiểm duyệt (Moderator)** | Nhân sự nền tảng | Duyệt/từ chối sự kiện, xử lý cờ AI |
| **Vận hành (Ops)** | SRE / Trực sự kiện | Chỉnh `admit_rate`, bật/tắt chế độ suy biến, xử lý sự cố |
| **AI Agent** | Gemini qua worker | Trợ lý khách, kiểm duyệt nội dung, phát hiện bất thường, viết báo cáo |

## 2. Luồng nghiệp vụ chính

### 2.1 Vòng đời sự kiện (Organizer)

```
DRAFT --submit--> PENDING_REVIEW --AI scan--> +-- AUTO_APPROVED --> SCHEDULED
                                              +-- NEEDS_HUMAN --> (Moderator) --> SCHEDULED | REJECTED
                                              +-- AUTO_REJECTED --> REJECTED

SCHEDULED --den T0--> ON_SALE --het ve/het gio--> SOLD_OUT | CLOSED --> COMPLETED
                                                                   \--> CANCELLED (hoan tien toan bo)
```

**Business rules:**

- **BR-E1** — Sự kiện chỉ chuyển `SCHEDULED` khi đã có ≥ 1 hạng vé với `quota > 0` và `price >= 0`.
- **BR-E2** — `sale_start_at` phải cách thời điểm duyệt ≥ 30 phút, để hệ thống kịp pre-warm hạ tầng.
- **BR-E3** — Sau khi `ON_SALE`, **cấm** giảm `quota` của hạng vé; chỉ được tăng (phát sự kiện `inventory.increased`).
- **BR-E4** — AI kiểm duyệt chấm 4 trục: nội dung cấm, sai lệch/lừa đảo, bản quyền hình ảnh, thông tin thiếu. Điểm rủi ro ≥ 0.8 → `AUTO_REJECTED`; 0.3–0.8 → `NEEDS_HUMAN`; < 0.3 → `AUTO_APPROVED`.
- **BR-E5** — Mọi quyết định của AI ghi `audit_log` kèm lý do và luôn có thể bị Moderator lật ngược.

### 2.2 Vòng đời phiên xếp hàng (Waiting Room)

```
            [ truoc T0 ]
Khach vao trang --> LOBBY (khong co so thu tu)
                      |
                      |  T0: xao tron ngau nhien (lottery) -> cap rank
                      v
                   QUEUED --admit controller--> ADMITTED (TTL 15 phut)
                      ^                              |
              [ sau T0: FIFO noi duoi ]              +-- mua xong --> COMPLETED
                                                     +-- het TTL  --> EXPIRED (phai xep lai)
                                                     +-- roi trang --> ABANDONED

QUEUED --rot ket noi > 90s--> (giu cho 5 phut) --qua han--> DROPPED
```

**Business rules:**

- **BR-Q1 (Công bằng)** — Mọi người vào **trước** `sale_start_at` được gom vào LOBBY **không có số thứ tự**. Đúng T0, hệ thống xáo trộn ngẫu nhiên toàn bộ LOBBY rồi mới cấp rank. Vào sớm 2 tiếng hay 2 giây đều có cơ hội như nhau → triệt tiêu động cơ bot bấm F5 và lợi thế đường truyền.
- **BR-Q2** — Sau T0, người mới vào được nối đuôi FIFO sau toàn bộ nhóm lottery.
- **BR-Q3 (1 người 1 chỗ)** — Token gắn với `identity_id` đã xác thực OTP. Join lại từ thiết bị/tab khác trả về **đúng token và rank cũ** (idempotent). Không có cách nhân bản chỗ.
- **BR-Q4** — Client nhận `poll_after_ms` do **server** quyết định theo rank (rank xa → 30s; gần lượt → 3s). Đây là van tự bảo vệ trước khách/bot tự ý poll dày.
- **BR-Q5** — `admit_rate` do controller tự điều chỉnh theo thông lượng checkout thực đo và tín hiệu backpressure; Ops có thể ghi đè thủ công (`manual_override`).
- **BR-Q6** — Khi hết vé, toàn bộ QUEUED nhận thông báo `SOLD_OUT` và được mời vào waitlist; không đẩy thêm ai vào checkout.
- **BR-Q7** — Suất admit có TTL 15 phút (10 phút giữ ghế + thời gian chọn vé). Hết TTL chưa thanh toán → EXPIRED, không được ưu tiên khi xếp lại.

### 2.3 Vòng đời đơn hàng (Ticketing + Payment)

```
(ADMITTED) chon ve
     |
     v  hold (Redis Lua + Postgres tx)
   HELD ------ TTL 10:00 ---------------------+
     |  khach bam thanh toan                  | het gio
     v                                        v
PAYMENT_PENDING --webhook OK--> PAID --> ISSUED     EXPIRED (tra kho ngay)
     |      |
     |      +--webhook FAIL / huy--> CANCELLED (tra kho)
     |
     +-- qua 15 phut khong co webhook --> reconcile job hoi cong thanh toan
                                          +-- da thu tien --> PAID   (tu chua)
                                          +-- chua thu     --> CANCELLED (tra kho)

ISSUED --organizer huy su kien--> REFUNDING --> REFUNDED
ISSUED --check-in--> CHECKED_IN (mot lan duy nhat)
```

**Business rules:**

- **BR-O1 (Không oversell — bất khả xâm phạm)** — Tổng vé ở trạng thái `HELD + PAID + ISSUED` của một hạng vé **không bao giờ** vượt `quota`. Hai lớp bảo vệ: Redis Lua atomic (chặn nhanh, không cho tải xuống DB) + Postgres `CHECK (available >= 0)` trong transaction (nguồn sự thật). Redis lệch số **không thể** gây oversell — chỉ có thể gây "báo hết vé sớm", và job đối soát sẽ sửa lại.
- **BR-O2** — Giữ ghế đúng **10:00** kể từ lúc hold thành công. Đồng hồ đếm ngược lấy `expires_at` từ server; client không tự tính.
- **BR-O3** — Một khách chỉ có **1 hold đang hoạt động** trên mỗi sự kiện. Tạo hold mới → hold cũ bị huỷ và trả kho ngay.
- **BR-O4** — Giới hạn mua: tối đa `max_per_order` (mặc định 4) vé/đơn và `max_per_identity` (mặc định 4) vé/người/sự kiện, tính gộp trên mọi đơn đã PAID.
- **BR-O5 (Idempotency)** — Mọi API ghi bắt buộc header `Idempotency-Key`. Gọi lại cùng key trong 24h trả **đúng response cũ**, không tạo đơn trùng.
- **BR-O6 (Webhook exactly-once)** — `payments.provider_txn_id` có UNIQUE index; webhook lặp lại là no-op trả 200. Webhook đến trước khi đơn tồn tại → lưu `payment_inbox` và replay sau.
- **BR-O7** — Trả kho phải **idempotent**: mỗi hold chỉ release đúng một lần (`WHERE released_at IS NULL`). Đây là chốt chặn chống double-release làm phồng kho → oversell.
- **BR-O8** — Vé phát hành gắn QR ký HMAC kèm `jti` chống sao chép; check-in đổi `ISSUED → CHECKED_IN` đúng một lần.

### 2.4 Trợ lý AI trong phòng chờ

```
Khach hoi --> API tra 202 + job_id (< 50ms)     [KHONG BAO GIO goi Gemini dong bo]
                |
                +-- 1. Rule/FAQ khop?        --> tra ngay, 0 token      (~40% luot)
                +-- 2. Cache ngu nghia hit?   --> tra ngay               (~45% luot)
                +-- 3. Cau y het dang in-flight? --> ghep chung (singleflight)
                +-- 4. Day vao RabbitMQ ai.chat.q
                         --> worker xin permit tu limiter toan cuc
                         --> goi Gemini
                              +-- het permit / 429 --> retry backoff qua delay-queue
                                                       --> qua nguong --> tra loi du phong tu FAQ

Ket qua day ve client qua SSE (hoac polling GET /assistant/messages/{job_id})
```

**Business rules:**

- **BR-A1** — Không request nào của người dùng **chờ đồng bộ** Gemini. API luôn trả `202 Accepted` kèm `job_id`.
- **BR-A2** — Hạn mức mỗi khách: 10 câu / 15 phút. Vượt → trả FAQ + gợi ý liên hệ CSKH.
- **BR-A3** — 4 hàng đợi tách biệt với consumer pool riêng, để job báo cáo nặng **không thể** làm nghẽn chat: `ai.chat.q` (ưu tiên cao, TTL 60s — quá hạn thì câu trả lời đã vô nghĩa), `ai.moderation.q`, `ai.anomaly.q`, `ai.report.q` (ưu tiên thấp).
- **BR-A4** — Trợ lý **chỉ** trả lời trong phạm vi: thông tin sự kiện, chính sách vé/hoàn tiền, hướng dẫn thao tác, tình trạng hàng chờ. Ngoài phạm vi → từ chối lịch sự. Cấm tuyệt đối: hứa giữ vé, đoán lượt bằng con số không lấy từ hệ thống, tiết lộ tồn kho chính xác.
- **BR-A5** — Mọi câu trả lời về **vị trí / ETA / tồn kho** phải lấy số liệu qua tool-call vào hệ thống thật; không để LLM tự bịa số.
- **BR-A6** — Ngân sách token theo sự kiện. Chạm 80% → cảnh báo Ops; chạm 100% → tự chuyển chế độ FAQ-only.
- **BR-A7** — Chống prompt injection: nội dung do người dùng/ban tổ chức nhập luôn được bọc trong khối dữ liệu và đánh dấu rõ là **dữ liệu, không phải chỉ thị**.

### 2.5 Anti-bot (chống vét vé)

Chấm điểm rủi ro `0..1`, quyết định theo tầng:

| Điểm | Hành động | Ghi chú |
|---|---|---|
| `< 0.30` | Cho qua | |
| `0.30 – 0.60` | Thử thách bước cao (Turnstile / OTP lại) | |
| `0.60 – 0.85` | **Làn chậm** (shadow) — vẫn xếp hàng nhưng ưu tiên thấp | Không báo cho bot biết nó bị phát hiện |
| `> 0.85` | Chặn + vô hiệu token | Ghi log để AI phân tích hậu kiểm |

**Tín hiệu (rule engine chạy in-path, < 5ms):**

| Rule | Tín hiệu |
|---|---|
| R1 | Phương sai khoảng cách giữa các request ≈ 0 (nhịp máy) |
| R2 | Thời gian từ lúc trang render tới click < 120ms (nhanh hơn ngưỡng sinh học) |
| R3 | Không có sự kiện chuột / cuộn / bàn phím trước khi submit |
| R4 | > N identity khác nhau chia sẻ cùng `device_fingerprint` hoặc cùng dải /24 trong 5 phút |
| R5 | User-Agent bất thường / dấu hiệu headless (thiếu WebGL, timezone lệch, ít font) |
| R6 | Cùng thẻ thanh toán dùng cho > 3 identity |
| R7 | Lặp lại chính xác một chuỗi thao tác chọn ghế qua nhiều phiên |

- **BR-B1** — Mọi quyết định chặn phải **giải thích được**: lưu danh sách rule đã kích hoạt, phục vụ khiếu nại và audit.
- **BR-B2** — AI **không** chặn trực tiếp. AI hậu kiểm trên log, tìm cụm hành vi mới và **đề xuất** rule; rule chỉ có hiệu lực sau khi người duyệt.

## 3. Ma trận quyền (RBAC)

| Hành động | Buyer | Organizer | Moderator | Ops |
|---|:-:|:-:|:-:|:-:|
| Xem sự kiện đã duyệt | ✅ | ✅ | ✅ | ✅ |
| Tạo/sửa sự kiện của mình | — | ✅ | — | — |
| Duyệt / từ chối sự kiện | — | — | ✅ | — |
| Xếp hàng & mua vé | ✅ | — | — | — |
| Chỉnh `admit_rate` | — | — | — | ✅ |
| Bật chế độ suy biến AI | — | — | — | ✅ |
| Xem báo cáo sau bán | — | ✅ (sự kiện của mình) | ✅ | ✅ |
| Gỡ chặn anti-bot | — | — | ✅ | ✅ |

## 4. Báo cáo sau đợt mở bán (AI sinh)

Khi `ON_SALE` kết thúc, job `ai.report.q` tổng hợp 6 phần:

1. **Thương mại** — doanh thu, tỉ lệ bán theo hạng vé, thời gian bán hết, giá trị đơn trung bình.
2. **Phễu chuyển đổi** — vào lobby → được admit → tạo hold → thanh toán → phát vé; chỉ ra điểm rơi lớn nhất.
3. **Kỹ thuật** — đỉnh rps, p99 theo mốc thời gian, `admit_rate` theo thời gian, số lần load-shedding, incident.
4. **Gian lận** — số phiên bị chặn, cụm bot phát hiện, rule mới đề xuất.
5. **Trải nghiệm** — top câu hỏi trong phòng chờ, tỉ lệ AI giải quyết trọn vẹn, chủ đề khiếu nại.
6. **Khuyến nghị** cho lần mở bán sau — số liệu hoá, có thể hành động ngay.

Đầu ra: bản Markdown + JSON số liệu, gửi Organizer & Ops, lưu vào bảng `sale_reports`.
