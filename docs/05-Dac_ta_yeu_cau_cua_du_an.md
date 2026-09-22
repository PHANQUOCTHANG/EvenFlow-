# Tài liệu Đặc tả Yêu cầu Hệ thống (SRS) - EvenFlow

> [!NOTE]
> Tài liệu này mô tả chi tiết các yêu cầu chức năng và phi chức năng của hệ thống bán vé sự kiện EvenFlow, tập trung vào khả năng chịu tải đột biến và ứng dụng AI trong vận hành.

## 1. Tác nhân (Actors)
- **Khách (Buyer):** Người mua vé, tham gia hàng chờ, thanh toán và sử dụng trợ lý AI.
- **Ban tổ chức (Organizer):** Người tạo sự kiện, cấu hình vé, xem báo cáo doanh số.
- **Kiểm duyệt (Moderator):** Nhân viên nền tảng duyệt sự kiện, xử lý các cảnh báo từ AI và giải quyết khiếu nại (VD: bỏ chặn bot).
- **Vận hành (Ops):** Kỹ sư SRE, quản trị viên giám sát hệ thống, điều chỉnh tốc độ hàng chờ (`admit_rate`), xử lý sự cố.
- **AI Agent:** Hệ thống AI tự động (sử dụng Gemini API) để trả lời khách hàng, kiểm duyệt, phát hiện bất thường và lập báo cáo.

---

## 2. Yêu cầu Chức năng (Functional Requirements)

### 2.1 Quản lý Sự kiện
- **FR-E1:** Ban tổ chức có thể tạo, chỉnh sửa sự kiện, định nghĩa các hạng vé, giá vé và số lượng (quota). Sự kiện phải có ít nhất 1 hạng vé hợp lệ (quota > 0, price >= 0) để được lên lịch (`SCHEDULED`).
- **FR-E2:** Thời gian mở bán (`sale_start_at`) phải cách thời điểm được phê duyệt ít nhất 30 phút để hệ thống khởi động hạ tầng.
- **FR-E3:** Sau khi sự kiện mở bán (`ON_SALE`), không được phép giảm số lượng vé tổng (quota) của bất kỳ hạng vé nào, chỉ được phép tăng.
- **FR-E4:** Mọi sự kiện mới phải đi qua hệ thống kiểm duyệt tự động bởi AI. AI sẽ chấm điểm rủi ro (0-1).
  - `>= 0.8`: Tự động từ chối.
  - `0.3 - 0.8`: Cần Moderator duyệt tay.
  - `< 0.3`: Tự động duyệt (`AUTO_APPROVED`).
- **FR-E5:** Mọi quyết định của AI phải được ghi log và Moderator có quyền ghi đè (lật ngược quyết định).

### 2.2 Quản lý Phòng chờ Ảo (Waiting Room)
- **FR-W1 (Tính công bằng):** Người dùng truy cập trước giờ mở bán được đưa vào LOBBY (không có số thứ tự). Đúng giờ G, hệ thống xáo trộn ngẫu nhiên tất cả người trong LOBBY để cấp số thứ tự. Người vào sau giờ G sẽ xếp hàng tuần tự (FIFO) nối tiếp sau nhóm LOBBY.
- **FR-W2 (Bảo toàn vị trí):** Số thứ tự gắn với định danh người dùng (`identity_id`). Nếu người dùng refresh trang hoặc đăng nhập thiết bị khác, họ vẫn nhận đúng số thứ tự cũ, không thể nhân bản chỗ.
- **FR-W3 (Admit Controller):** Hệ thống tự động tính toán thông lượng cho khách vào mua vé (`admit_rate`) dựa trên sức khỏe thực tế của hệ thống thanh toán và database (P99 latency, DB Pool). Ops có quyền can thiệp ghi đè `admit_rate` thủ công.
- **FR-W4 (Hết vé):** Khi sự kiện báo hết vé, toàn bộ người trong hàng chờ phải nhận được thông báo chuyển sang danh sách chờ (Waitlist).

### 2.3 Đặt vé và Thanh toán
- **FR-O1 (Chống Oversell):** Đảm bảo tuyệt đối không bao giờ bán vượt quá số lượng vé (quota). Tổng vé ở trạng thái Giữ (HELD) + Đã thanh toán (PAID) + Đã phát hành (ISSUED) <= Quota.
- **FR-O2 (Giữ ghế):** Khi người dùng chọn vé thành công, ghế được giữ (hold) đúng 10 phút. Nếu quá hạn không thanh toán, vé sẽ tự động được thu hồi (release) về kho.
- **FR-O3 (Giới hạn mua):** Hệ thống giới hạn số lượng vé mua tối đa cho mỗi đơn hàng (mặc định 4) và mỗi người dùng (mặc định 4) trên cùng một sự kiện.
- **FR-O4 (Xử lý thanh toán):** Hỗ trợ webhook từ cổng thanh toán. Webhook phải được xử lý exactly-once. Nếu hệ thống không nhận được webhook sau 15 phút, một job đối soát sẽ kiểm tra lại với cổng thanh toán để tự động sửa trạng thái đơn.
- **FR-O5 (Phát hành vé):** Vé (ISSUED) phải chứa mã QR được ký HMAC với ID chống trùng lặp. Mỗi vé chỉ được check-in thành công 1 lần.

### 2.4 Trợ lý AI và Chat
- **FR-A1 (Phản hồi không đồng bộ):** User chat với AI phải nhận phản hồi qua SSE hoặc Polling, request gửi đi không bao giờ bắt kết nối HTTP chờ AI.
- **FR-A2 (Giới hạn chat):** Mỗi người dùng chỉ được hỏi trợ lý AI tối đa 10 câu / 15 phút. Vượt giới hạn, hệ thống trả lời bằng FAQ tĩnh.
- **FR-A3 (Phạm vi trả lời):** AI chỉ trả lời các thông tin liên quan đến sự kiện, chính sách, và thời gian chờ. Tuyệt đối không để AI tự tạo dữ liệu không có thật (ảo giác về tồn kho). Mọi thông tin vị trí, ETA phải lấy từ backend thật (tool-call).
- **FR-A4 (Chống Prompt Injection):** Các nội dung do Organizer nhập (mô tả sự kiện) phải được cô lập (sandbox) và được đánh dấu là Dữ liệu, không phải Chỉ thị để AI không bị thao túng.

### 2.5 Phát hiện và Chống Bot (Anti-bot)
- **FR-B1:** Hệ thống liên tục chấm điểm rủi ro các phiên truy cập dựa trên luật (khoảng cách request, tốc độ thao tác, User-Agent, thiết bị).
- **FR-B2:** Phân loại điểm rủi ro:
  - `< 0.3`: Cho qua.
  - `0.3 - 0.6`: Bật thử thách CAPTCHA / OTP.
  - `0.6 - 0.85`: Đẩy vào làn chậm (Shadow banning) - vẫn hiển thị xếp hàng nhưng bị đánh tụt ưu tiên.
  - `> 0.85`: Chặn hoàn toàn, vô hiệu hóa token xếp hàng.
- **FR-B3:** Mọi quyết định chặn phải ghi rõ rule nào đã được trigger để phục vụ khiếu nại.
- **FR-B4:** AI Agent hậu kiểm log hành vi định kỳ mỗi 30s để phân tích, tìm kiếm mẫu hành vi Bot mới và đề xuất Rule mới cho Moderator phê duyệt.

### 2.6 Báo cáo sau mở bán
- **FR-R1:** Khi đợt mở bán kết thúc, job AI tự động tạo một báo cáo bao gồm: Phân tích phễu thương mại, dữ liệu kỹ thuật, tình trạng gian lận, số liệu trải nghiệm khách hàng và các khuyến nghị.
- **FR-R2:** Báo cáo được xuất dưới định dạng JSON & Markdown và gửi đến Organizer + Ops.

---

## 3. Yêu cầu Phi chức năng (Non-Functional Requirements)

### 3.1 Hiệu năng (Performance) và Khả năng mở rộng (Scalability)
- **NFR-P1 (Chịu tải đỉnh):** CDN và Gateway phải chịu được tối thiểu 300,000 requests/giây ở các endpoint tĩnh và frontend.
- **NFR-P2 (Redis Scalability):** Hệ thống hàng chờ phải hỗ trợ shard dữ liệu theo Event ID, giúp tăng băng thông I/O tuyến tính trên Redis Cluster.
- **NFR-P3 (LLM Rate-limit Protection):** Global Token-Bucket Limiter phải chặn và làm phẳng mọi request tới LLM (Gemini) để đảm bảo không bao giờ vượt ngưỡng RPM và TPM giới hạn (VD: 60 RPM), thay vì bị dội lỗi 429.
- **NFR-P4 (Tải DB hằng số):** Database PostgreSQL (đường dẫn nóng như tạo Order, Update tồn kho) không bao giờ nhận quá 200 requests/giây (hoặc mức giới hạn cấu hình) bất kể hàng đợi bên ngoài lớn đến đâu. Đảm bảo bằng `admit_rate`.
- **NFR-P5 (Load shedding):** Khi hệ thống cạn kiệt tài nguyên, các endpoint phụ trợ sẽ trả mã 503 Retry-After, chỉ duy trì 100% SLA cho endpoint `queue/status` và `payment/webhook`.

### 3.2 Độ tin cậy (Reliability) và Tính sẵn sàng (Availability)
- **NFR-R1 (Oversell Prevention):** Database PostgreSQL đóng vai trò chốt chặn cuối cùng (schema check `available >= 0`). Dù Redis có lỗi hay sai lệch cache, hệ thống chỉ thà từ chối đơn hàng thay vì bán quá số vé (oversell).
- **NFR-R2 (Message Queue Reliability):** RabbitMQ chạy ở chế độ Quorum queue (3 nodes). Cho phép mất 1 node mà không làm mất trạng thái của hệ thống xử lý bất đồng bộ (AI, Notification, Outbox).
- **NFR-R3 (Graceful Degradation cho AI):** Nếu LLM API chết hoặc vượt ngân sách 80%, hệ thống tự động suy biến nhiều nấc (giảm số token đầu ra -> chuyển qua FAQ Template -> tắt Assistant chuyển sang CSKH). 

### 3.3 Bảo mật (Security)
- **NFR-S1 (Authentication):** Sử dụng JWT ngắn hạn (15 phút) cùng Refresh Token. Token của hàng chờ (queue token) được ký kèm `identity_id`, `device_id`, và `event_id`.
- **NFR-S2 (Tích hợp):** Webhook của thanh toán phải xác thực bằng chữ ký. Mọi API mang tính đột biến dữ liệu phải triển khai Idempotency Key an toàn trong 24 giờ để tránh replay attack.
- **NFR-S3 (Mã hóa):** Các dữ liệu nhạy cảm (PII - email, số điện thoại) phải mã hóa cấp độ cột ở database; log của hệ thống không được ghi PII ở dạng plain-text.

### 3.4 Khả năng quan trắc (Observability)
- **NFR-O1 (Metric & Alerting):** Hệ thống cần expose các business metric quan trọng: `waitingroom_admit_rate` (nếu = 0 quá 60s phải cảnh báo), `ticketing_oversell_guard_rejections` (cảnh báo mức Critical), `ai_queue_lag_seconds`.
- **NFR-O2 (Tracing):** Áp dụng OpenTelemetry xuyên suốt qua tất cả các microservices và RabbitMQ (truyền qua headers), giúp trace toàn bộ vòng đời của một order từ lúc join queue đến lúc xuất vé.
