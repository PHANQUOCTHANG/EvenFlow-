# EV-128 — Middleware `Idempotency-Key` (`libs/go/idem`) (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-128 (mã backlog **EVF-33**, issue 1304) — 5 SP — Highest |
| Kế hoạch tổng | [09-ke-hoach-hoan-thien-backend.md](../../09-ke-hoach-hoan-thien-backend.md) — Giai đoạn 1, việc W4 |
| Branch | `feature/EV-128-idempotency-key` (từ `develop` @ `f0f368b`, sau khi PR #56 merge) |
| Quy tắc nghiệp vụ | BR-O5 — [01-nghiep-vu.md](../../01-nghiep-vu.md) |

## 1. Bối cảnh

Trước slice này, handler tạo hold chỉ kiểm header có tồn tại (thiếu thì 400) rồi bỏ qua giá trị. Không có gì được
lưu, nên "gọi lại cùng key trả đúng response cũ" chưa đúng. Bấm lại nút mua không tạo đơn trùng chỉ nhờ BR-O3
(trả lại hold đang có), không phải nhờ BR-O5; các API ghi sau này (thanh toán, huỷ hold) sẽ không có che chắn đó.

Bảng `idempotency_keys` đã có từ `migrations/0001_core.sql`; slice **không thêm migration**.

## 2. Tiêu chí nghiệm thu

Ba AC đầu là AC của Jira; AC-4..AC-8 là yêu cầu kỹ thuật rút ra khi thiết kế.

| AC | Nội dung |
|---|---|
| AC-1 | Thiếu header, rỗng, quá 255 ký tự, hoặc chứa ký tự ngoài ASCII in được: **400**, handler không chạy |
| AC-2 | Gọi lại cùng key (cùng người, cùng nội dung) trong 24 giờ: trả **đúng từng byte** response cũ, status cũ, kèm `Idempotent-Replayed: true`; nghiệp vụ không chạy lại |
| AC-3 | Hai request đồng thời cùng key: đúng một handler chạy; request còn lại nhận **409** (kèm `Retry-After`) hoặc response cũ |
| AC-4 | Cùng key nhưng khác nội dung hoặc khác người gọi: **422**, không bao giờ trả response của người khác |
| AC-5 | Response 5xx và panic không được nhớ: key bị xoá để lần thử lại chạy thật |
| AC-6 | Client ngắt kết nối giữa chừng vẫn lưu được response; request bị tiếp quản sau lease không ghi đè kết quả của chủ mới |
| AC-7 | Postgres lỗi ở bước giữ key: **503**, handler không chạy |
| AC-8 | Ticketing: `POST /v1/events/{id}/holds` đi qua middleware; không đăng ký được route ghi mà thiếu middleware |

## 3. Quyết định

| # | Câu hỏi | Chọn | Lý do |
|---|---|---|---|
| D1 | Nhánh | Tách từ `develop` sau PR #56 | Test tích hợp dùng `testx.Postgres` |
| D2 | Request đồng thời | 409 + `Retry-After: 1` + `type` riêng, không chờ | Không giữ goroutine và kết nối Postgres (pool 25) để chờ |
| D3 | Lưu gì | 2xx và 4xx; 5xx thì xoá key | Lưu 503 sẽ kẹt khách vào 503 suốt 24 giờ |
| D4 | Lỗi Postgres lúc `Begin` | 503, không chạy handler | Chạy tiếp là ghi mà không có bảo vệ |
| D5 | Request chết giữa chừng | Sau 30 giây (lease) request khác tiếp quản | Không cần thêm cột; an toàn với hold nhờ BR-O3 |

## 4. Thiết kế

- `Store` (`Begin` / `Complete` / `Abandon`), `PostgresStore`, `Middleware(store, opts...)`. Gắn theo từng route ghi.
- `Begin` là **một** câu `INSERT ... ON CONFLICT DO UPDATE ... WHERE` (khoá hết hạn hoặc quá lease); không giành
  được thì `SELECT` để phân loại thành phát lại / đang xử lý / khác nội dung.
- Quyền sở hữu là `created_at` của dòng: `Complete` và `Abandon` chỉ tác dụng nếu còn khớp, nên chủ cũ trả muộn
  không ghi đè người tiếp quản.
- Hash = SHA-256 của method, path, query, định danh, body, có tiền tố độ dài để không nhập nhằng ranh giới.
- Response lưu `{"content_type", "body"}` với body là base64 vì JSONB sắp xếp lại khoá và bỏ khoảng trắng.
- Ghi kết quả dùng `context.WithoutCancel` + timeout 5 giây.
- Định danh lấy qua `WithIdentity`; ticketing dùng `X-Identity-Id` cho tới khi có JWT (giai đoạn 3).

## 5. Ngoài phạm vi

- Dọn key hết hạn (giai đoạn 2, mục "Dọn dẹp").
- Lưu key trong cùng transaction với nghiệp vụ.
- Áp cho payment và waitingroom (chưa có handler ghi).
- Định danh lấy từ JWT (giai đoạn 3); `/metrics` cho `idem`.
- Route stream (SSE): middleware đệm toàn bộ response nên cố ý không hỗ trợ.
