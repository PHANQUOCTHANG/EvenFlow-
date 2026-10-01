# EventFlow — Backlog Jira

Project key: **EVF**. File import: [`jira-import.csv`](jira-import.csv) — sinh tự động cùng nguồn với tài liệu này, hai file luôn khớp nhau.

**Quy mô:** 10 epic · 89 story/task · **461 SP** · 7 sprint × 2 tuần.

## Quy ước

| Hạng mục | Quy ước |
|---|---|
| SP | Story point, thang Fibonacci (1, 2, 3, 5, 8). Hạng mục > 8 SP phải tách nhỏ trước khi vào sprint. |
| Độ ưu tiên | **P0** → Highest: chặn đường tới hạn · **P1** → High: cần cho MVP · **P2** → Medium: hoàn thiện |
| Loại issue | **Epic** · **Story** (có giá trị với người dùng, viết dạng *Là… tôi muốn… để…*) · **Task** (kỹ thuật/kiểm thử) |
| Mã backlog | `EVF-n` trong tài liệu là mã tham chiếu cố định, ghi ở cuối mô tả mỗi issue. Key thật do Jira cấp khi import có thể khác. |
| Nhánh / commit | `feat/EVF-123-mo-ta` · `feat(ticketing): EVF-123 ...` (dùng key Jira thật) |
| Tiêu chí nghiệm thu | Mỗi issue có AC đo được — không dùng các cụm mơ hồ như "hoạt động tốt". |

Mỗi issue trong Jira có mô tả đầy đủ các mục: *User story* (với Story) · *Bối cảnh* · *Phạm vi công việc* · *Tiêu chí nghiệm thu* · *Phụ thuộc* · *Hiện trạng mã nguồn* (nếu đã có khung) · *Tham chiếu* (BR, tài liệu).

### Definition of Ready (DoR) — điều kiện kéo issue vào sprint

- Có AC đo được và đã được PO/Tech Lead thống nhất.
- Đã ước lượng SP, ≤ 8 SP.
- Mọi issue phụ thuộc đã xong hoặc nằm trong cùng sprint và được xếp làm trước.

### Definition of Done (DoD) — áp dụng cho mọi issue

- Thoả toàn bộ AC; có test tự động chứng minh từng AC khi khả thi.
- Code review được ít nhất 1 người duyệt; CI xanh (lint, unit, integration, Trivy).
- Coverage `domain/` và `app/` ≥ 70%; không phá vỡ EVF-39 (test chống oversell).
- Có metric/log/trace cho luồng mới; không log PII dạng thường.
- Cập nhật tài liệu (OpenAPI, docs/, runbook) nếu hành vi thay đổi.
- Đã chạy được trên môi trường local (`make up`) và staging.

---

## E1 — Nền móng & Hạ tầng · 34 SP

**Mục tiêu:** Dựng monorepo, thư viện dùng chung, môi trường local, CI/CD và observability để mọi đội phát triển song song trên cùng một chuẩn.

**Rủi ro chính:** Nền móng chậm sẽ kéo lùi toàn bộ đường tới hạn — phải xong trong Sprint 0.

| Mã | Loại | Tiêu đề | SP | P | Sprint | Phụ thuộc |
|---|---|---|---:|---|---|---|
| EVF-1 | Story | Khởi tạo monorepo: go.work, pnpm workspace, uv, Makefile | 3 | P0 | S0 | — |
| EVF-2 | Story | libs/go/httpx: router, middleware và lỗi chuẩn RFC 7807 | 5 | P0 | S0 | EVF-1 |
| EVF-3 | Story | libs/go/otelx: OpenTelemetry trace + Prometheus metric, lan truyền qua AMQP header | 5 | P0 | S0 | EVF-1 |
| EVF-4 | Story | docker-compose local đầy đủ hạ tầng và service | 5 | P0 | S0 | EVF-1 |
| EVF-5 | Story | Khung migration goose + schema khởi tạo | 3 | P0 | S0 | EVF-4 |
| EVF-6 | Story | CI pipeline: lint → unit → integration → build → Trivy scan | 5 | P0 | S0 | EVF-1 |
| EVF-7 | Story | libs/go/mq: publisher + consumer có retry/DLQ, khai báo topology | 5 | P0 | S0 | EVF-3 |
| EVF-8 | Task | Template service Go (hexagonal) + generator tạo service mới | 3 | P1 | S0 | EVF-2 |

**Tiêu chí hoàn thành epic:**

- `make up` khởi động toàn hệ, `make smoke` chạy qua 1 luồng mua vé
- Một trace duy nhất xuyên ≥ 3 service trong Jaeger
- CI chặn PR khi lint/test/scan fail hoặc coverage domain/app < 70%

---

## E2 — Định danh & Bảo mật · 26 SP

**Mục tiêu:** Xác thực người dùng chắc chắn (OTP), cấp token an toàn và gắn identity ↔ thiết bị làm nền cho quy tắc 1 người 1 chỗ và anti-bot.

**Rủi ro chính:** Lỗ hổng xác thực cho phép nhân bản chỗ trong hàng chờ, phá vỡ tính công bằng.

| Mã | Loại | Tiêu đề | SP | P | Sprint | Phụ thuộc |
|---|---|---|---:|---|---|---|
| EVF-10 | Story | Đăng ký / đăng nhập, băm mật khẩu argon2id | 5 | P0 | S0 | EVF-2, EVF-5 |
| EVF-11 | Story | Xác thực OTP qua SMS/email trước khi xếp hàng | 5 | P0 | S1 | EVF-10 |
| EVF-12 | Story | JWT ngắn hạn + refresh token xoay vòng, thu hồi được | 5 | P0 | S0 | EVF-10 |
| EVF-13 | Story | Đăng ký device fingerprint, liên kết identity ↔ device | 3 | P1 | S1 | EVF-10 |
| EVF-14 | Story | Phân quyền RBAC: Buyer / Organizer / Moderator / Ops | 5 | P1 | S1 | EVF-12 |
| EVF-15 | Task | Mã hoá PII ở tầng cột + lọc PII khỏi log | 3 | P1 | S1 | EVF-10 |

**Tiêu chí hoàn thành epic:**

- Chưa verify OTP thì không xếp hàng được (BR-Q3)
- Ma trận quyền ở docs/01 được phủ bằng test
- Log không chứa PII dạng thường

---

## E3 — Sự kiện & Kiểm duyệt · 31 SP

**Mục tiêu:** Cho Organizer tạo và vận hành sự kiện theo đúng state machine, có kiểm duyệt AI + con người trước khi mở bán.

**Rủi ro chính:** Trang sự kiện chạm DB tại T0 sẽ làm sập Postgres trước cả khi hàng chờ hoạt động.

| Mã | Loại | Tiêu đề | SP | P | Sprint | Phụ thuộc |
|---|---|---|---:|---|---|---|
| EVF-20 | Story | CRUD sự kiện + hạng vé, state machine DRAFT → … → COMPLETED | 8 | P0 | S1 | EVF-5, EVF-14 |
| EVF-21 | Story | Cấu hình lịch mở bán, ràng buộc T0 ≥ thời điểm duyệt + 30 phút | 3 | P0 | S1 | EVF-20 |
| EVF-22 | Story | Khoá không cho giảm quota sau khi ON_SALE | 3 | P0 | S1 | EVF-20, EVF-30 |
| EVF-23 | Story | Đẩy job kiểm duyệt AI khi submit, nhận kết quả bất đồng bộ | 5 | P1 | S4 | EVF-20, EVF-89 |
| EVF-24 | Story | Hàng đợi việc cho Moderator: duyệt / từ chối / lật ngược quyết định AI | 5 | P1 | S4 | EVF-23, EVF-14 |
| EVF-25 | Story | Trang sự kiện công khai render ISR, phục vụ từ CDN | 5 | P0 | S2 | EVF-20 |
| EVF-26 | Task | Upload ảnh sự kiện lên object storage + sinh biến thể | 2 | P2 | S6 | EVF-20 |

**Tiêu chí hoàn thành epic:**

- Mọi chuyển trạng thái sai bị từ chối kèm lỗi rõ ràng
- Mọi quyết định AI có audit_log và lật ngược được
- Trang sự kiện 0 query DB khi cache hit

---

## E4 — Ticketing & Chống oversell · 62 SP

**Mục tiêu:** Giữ ghế, đặt vé và phát vé dưới tải cao với bất biến tuyệt đối: KHÔNG BAO GIỜ bán quá quota (BR-O1).

**Rủi ro chính:** Epic rủi ro cao nhất. Một lần oversell là sự cố nghiêm trọng về uy tín và pháp lý.

| Mã | Loại | Tiêu đề | SP | P | Sprint | Phụ thuộc |
|---|---|---|---:|---|---|---|
| EVF-30 | Story | **Schema tồn kho phân mảnh 32 bucket/hạng vé + CHECK (available >= 0)** | 5 | P0 | S0 | EVF-5 |
| EVF-31 | Story | **Redis Lua inventory gate: hold atomic, giảm tồn, ghi ZSET hết hạn** | 8 | P0 | S1 | EVF-30 |
| EVF-32 | Story | **Ghi hold vào Postgres trong transaction, chọn bucket ngẫu nhiên + fallback vòng tròn** | 8 | P0 | S1 | EVF-31 |
| EVF-33 | Story | **Middleware Idempotency-Key (libs/go/idem) cho mọi API ghi** | 5 | P0 | S0 | EVF-2, EVF-5 |
| EVF-34 | Story | **Hold sweeper: quét ZSET mỗi 1s, trả kho idempotent** | 8 | P0 | S1 | EVF-32 |
| EVF-35 | Story | Giới hạn mua: max_per_order và max_per_identity gộp mọi đơn | 5 | P0 | S1 | EVF-32 |
| EVF-36 | Story | Transactional outbox + relay ra RabbitMQ | 5 | P0 | S0 | EVF-7 |
| EVF-37 | Story | Job đối soát tồn kho Redis ↔ Postgres mỗi 10s, export metric inventory_drift | 5 | P0 | S1 | EVF-31, EVF-32 |
| EVF-38 | Story | Phát hành vé + QR ký HMAC có jti | 5 | P1 | S3 | EVF-71 |
| EVF-39 | Task | **Test đồng thời: 10.000 goroutine mua 100 vé → đúng 100 đơn, 0 oversell** | 5 | P0 | S1 | EVF-32, EVF-34 |
| EVF-40 | Task | API check-in: đổi ISSUED → CHECKED_IN đúng một lần | 3 | P2 | S6 | EVF-38 |

**Tiêu chí hoàn thành epic:**

- EVF-39 chạy 200 lần trong CI: 0 oversell, 0 under-sell
- Metric inventory_drift về 0 trong ≤ 20s sau khi cố ý làm lệch
- Không double-release khi chạy 2 sweeper song song

---

## E5 — Phòng chờ ảo · 60 SP

**Mục tiêu:** Xếp hàng công bằng và biến đỉnh tải vô hạn thành dòng tải hằng số xuống checkout bằng admit controller.

**Rủi ro chính:** Controller sai thì hoặc DB sập (admit quá nhanh) hoặc khách chờ vô ích (admit quá chậm).

| Mã | Loại | Tiêu đề | SP | P | Sprint | Phụ thuộc |
|---|---|---|---:|---|---|---|
| EVF-50 | Story | **Queue token ký, gắn identity + device + event, join idempotent** | 5 | P0 | S2 | EVF-11, EVF-13 |
| EVF-51 | Story | **join.lua: O(1), một round-trip Redis, shard theo event** | 5 | P0 | S2 | EVF-50 |
| EVF-52 | Story | **LOBBY trước T0 + lottery xáo trộn công bằng tại T0** | 8 | P0 | S2 | EVF-51 |
| EVF-53 | Story | Nối đuôi FIFO sau T0 | 3 | P0 | S2 | EVF-52 |
| EVF-54 | Story | GET /queue/status: rank, ETA, poll_after_ms thích nghi + ETag | 8 | P0 | S2 | EVF-51 |
| EVF-55 | Story | **Admit controller AIMD, bầu leader qua Redis lock có fencing** | 8 | P0 | S2 | EVF-54 |
| EVF-56 | Story | admit.lua: ZPOPMIN theo lô → tập admitted, TTL 15 phút | 5 | P0 | S2 | EVF-55 |
| EVF-57 | Story | SSE push "tới lượt bạn" + fallback polling | 5 | P0 | S2 | EVF-56 |
| EVF-58 | Story | Heartbeat + giữ chỗ 5 phút khi rớt kết nối | 3 | P1 | S2 | EVF-54 |
| EVF-59 | Story | Xử lý SOLD_OUT: thông báo hàng chờ, chuyển sang waitlist | 3 | P1 | S6 | EVF-57 |
| EVF-60 | Story | Ops API + UI: xem/ghi đè admit_rate, tạm dừng khẩn cấp | 5 | P1 | S5 | EVF-55, EVF-14 |
| EVF-61 | Task | Jitter phía client 0..5s tại T0 | 2 | P0 | S2 | EVF-51 |

**Tiêu chí hoàn thành epic:**

- k6: 100k VU join trong 5s, p99 < 300 ms
- Kiểm định thống kê: thời điểm join trước T0 không tương quan với rank
- admit_rate tự giảm ≤ 10s khi checkout chậm, phục hồi ≤ 60s

---

## E6 — Thanh toán · 34 SP

**Mục tiêu:** Thu tiền chính xác một lần cho mỗi đơn, tự chữa đơn treo và hoàn tiền khi sự kiện huỷ.

**Rủi ro chính:** Webhook trùng hoặc mất gây thu tiền 2 lần hoặc thu tiền mà không phát vé.

| Mã | Loại | Tiêu đề | SP | P | Sprint | Phụ thuộc |
|---|---|---|---:|---|---|---|
| EVF-70 | Story | Adapter cổng thanh toán (VNPay/Momo/Stripe) + sandbox mock | 8 | P0 | S3 | EVF-32 |
| EVF-71 | Story | **Webhook exactly-once: UNIQUE provider_txn_id, xác minh chữ ký, chống replay** | 8 | P0 | S3 | EVF-70, EVF-36 |
| EVF-72 | Story | payment_inbox cho webhook đến sớm + replay | 5 | P1 | S5 | EVF-71 |
| EVF-73 | Story | Job đối soát: đơn quá 15 phút không có webhook → hỏi cổng thanh toán, tự chữa | 5 | P0 | S3 | EVF-71 |
| EVF-74 | Story | Luồng hoàn tiền khi sự kiện bị huỷ | 5 | P2 | S6 | EVF-71, EVF-38 |
| EVF-75 | Task | Email/SMS xác nhận + đính kèm vé qua notification-svc | 3 | P1 | S5 | EVF-38 |

**Tiêu chí hoàn thành epic:**

- Bắn lại cùng webhook 100 lần → đúng 1 đơn PAID
- Không còn đơn ở trạng thái lửng lơ quá 15 phút
- Chạy local hoàn toàn bằng mock

---

## E7 — AI (Gemini) · 89 SP

**Mục tiêu:** Trả lời hàng nghìn câu hỏi/phút trong phòng chờ và hỗ trợ kiểm duyệt, phát hiện bất thường, báo cáo — không bao giờ để lỗi 429 tới người dùng.

**Rủi ro chính:** Limiter sai khiến trợ lý chết đúng lúc cần nhất; LLM bịa số gây hiểu lầm cho khách.

| Mã | Loại | Tiêu đề | SP | P | Sprint | Phụ thuộc |
|---|---|---|---:|---|---|---|
| EVF-80 | Story | Khung ai-worker: FastAPI + 4 consumer RabbitMQ tách biệt | 8 | P0 | S3 | EVF-7 |
| EVF-81 | Story | **Global token-bucket limiter RPM + TPM trên Redis Lua, đặt chỗ token trước khi gọi** | 8 | P0 | S3 | EVF-80 |
| EVF-82 | Story | Retry backoff qua delay-queue 5s/30s/2m + DLQ | 5 | P0 | S3 | EVF-80 |
| EVF-83 | Story | Tầng FAQ/intent trả lời 0 token (~50 mẫu) | 5 | P0 | S3 | EVF-80 |
| EVF-84 | Story | Cache ngữ nghĩa bằng embedding trên Redis, ngưỡng 0.93, scope theo sự kiện | 8 | P0 | S4 | EVF-80 |
| EVF-85 | Story | Singleflight gộp câu hỏi trùng đang in-flight | 3 | P1 | S4 | EVF-84 |
| EVF-86 | Story | Tool-call lấy số liệu thật (vị trí, ETA, tình trạng vé) — cấm bịa số | 5 | P0 | S4 | EVF-80, EVF-54 |
| EVF-87 | Story | 4 nấc suy biến NORMAL / SAVING / FAQ_ONLY / OFF + công tắc cho Ops | 5 | P0 | S4 | EVF-83 |
| EVF-88 | Story | Ngân sách token theo sự kiện: cảnh báo 80%, tự khoá 100% | 3 | P1 | S5 | EVF-81, EVF-87 |
| EVF-89 | Story | Kiểm duyệt sự kiện: structured JSON output, chấm 4 trục, ngưỡng định tuyến | 8 | P1 | S4 | EVF-81 |
| EVF-90 | Story | Phát hiện bất thường: gom cụm thống kê trước, LLM chỉ diễn giải và đề xuất rule | 8 | P1 | S6 | EVF-81, EVF-104 |
| EVF-91 | Story | Báo cáo sau bán 6 phần: SQL tính số, LLM viết | 8 | P1 | S6 | EVF-81 |
| EVF-92 | Story | Chống prompt injection + lọc đầu ra | 5 | P0 | S4 | EVF-80 |
| EVF-93 | Task | Bộ eval prompt: gold set 200 câu, chạy khi đổi prompt/model | 5 | P2 | S6 | EVF-83, EVF-89 |
| EVF-94 | Story | API trợ lý phía user: 202 + job_id, SSE trả kết quả, quota 10 câu / 15 phút | 5 | P0 | S3 | EVF-80 |

**Tiêu chí hoàn thành epic:**

- 5.000 câu/phút × 10 phút với hạn mức 60 RPM → gemini_429_total == 0, p95 < 4s, cache hit ≥ 80%
- Gemini sập hoàn toàn → khách vẫn nhận câu trả lời FAQ
- 0/50 payload injection vượt rào

---

## E8 — Anti-bot · 29 SP

**Mục tiêu:** Phát hiện và xử lý bot vét vé bằng luật hành vi chạy in-path, giải thích được mọi quyết định.

**Rủi ro chính:** False positive cao sẽ chặn nhầm khách thật ngay trong giờ mở bán.

| Mã | Loại | Tiêu đề | SP | P | Sprint | Phụ thuộc |
|---|---|---|---:|---|---|---|
| EVF-100 | Story | Thu thập tín hiệu client (nhịp, tương tác, fingerprint) gửi kèm request | 5 | P0 | S4 | EVF-13 |
| EVF-101 | Story | **Rule engine R1–R7 chạy in-path < 5 ms** | 8 | P0 | S4 | EVF-100 |
| EVF-102 | Story | Chấm điểm rủi ro + 4 tầng hành động (qua / thử thách / làn chậm / chặn) | 5 | P0 | S4 | EVF-101 |
| EVF-103 | Story | Tích hợp Turnstile ở bước join và bước thử thách nâng cao | 3 | P1 | S4 | EVF-102 |
| EVF-104 | Story | bot_decisions giải thích được + UI khiếu nại/gỡ chặn | 5 | P1 | S4 | EVF-102, EVF-14 |
| EVF-105 | Task | Quy trình duyệt rule do AI đề xuất | 3 | P2 | S6 | EVF-90 |

**Tiêu chí hoàn thành epic:**

- Bot script mẫu bị chặn ≥ 95%, false positive < 1%
- Rule engine p99 < 5ms
- Mọi quyết định chặn truy ra được danh sách rule đã kích hoạt

---

## E9 — Frontend · 49 SP

**Mục tiêu:** Giao diện Next.js cho Buyer, Organizer và Ops — nhanh, tôn trọng tín hiệu điều tiết từ server và rõ ràng khi hệ thống suy biến.

**Rủi ro chính:** Client tự poll dày hoặc tự tính giờ sẽ phá cơ chế điều tiết tải và giữ ghế.

| Mã | Loại | Tiêu đề | SP | P | Sprint | Phụ thuộc |
|---|---|---|---:|---|---|---|
| EVF-110 | Story | Khung Next.js App Router + design system + dark mode | 5 | P0 | S0 | EVF-1 |
| EVF-111 | Story | Trang sự kiện ISR + đồng hồ đếm ngược theo giờ server | 5 | P0 | S2 | EVF-25, EVF-110 |
| EVF-112 | Story | UI phòng chờ: vị trí, ETA, tiến trình, polling thích nghi + SSE | 8 | P0 | S2 | EVF-54, EVF-57, EVF-110 |
| EVF-113 | Story | Chọn vé + đồng hồ giữ ghế 10:00 theo expires_at của server | 8 | P0 | S3 | EVF-32, EVF-33, EVF-112 |
| EVF-114 | Story | Luồng thanh toán + trang kết quả (kể cả hold hết hạn giữa chừng) | 5 | P0 | S3 | EVF-70, EVF-113 |
| EVF-115 | Story | Panel chat trợ lý: streaming, FAQ chips, trạng thái suy biến | 5 | P1 | S5 | EVF-94, EVF-87 |
| EVF-116 | Story | Console Organizer: tạo sự kiện, trạng thái duyệt, doanh số realtime | 5 | P1 | S6 | EVF-20, EVF-24 |
| EVF-117 | Story | War room cho Ops: phễu realtime, admit rate, tồn kho, AI lag | 5 | P1 | S5 | EVF-55, EVF-125 |
| EVF-118 | Task | Accessibility (WCAG AA) + i18n vi/en | 3 | P2 | S6 | EVF-110 |

**Tiêu chí hoàn thành epic:**

- Client luôn tuân thủ poll_after_ms và expires_at của server
- E2E Playwright luồng mua vé xanh trong CI
- axe-core 0 lỗi nghiêm trọng

---

## E10 — Vận hành, chịu tải & phát hành · 47 SP

**Mục tiêu:** Đưa hệ thống lên Kubernetes, chứng minh chịu tải 10× bằng load test + chaos drill và sẵn sàng trực sự kiện.

**Rủi ro chính:** Dựa vào autoscale tại T0 là quá muộn (pod khởi động 30–60s) — phải pre-warm.

| Mã | Loại | Tiêu đề | SP | P | Sprint | Phụ thuộc |
|---|---|---|---:|---|---|---|
| EVF-120 | Story | K8s manifest mọi service: probe, resource, PDB, anti-affinity | 8 | P0 | S5 | EVF-4 |
| EVF-121 | Story | HPA theo custom metric (rps, queue_depth) qua Prometheus adapter | 5 | P0 | S5 | EVF-120 |
| EVF-122 | Story | **CronJob pre-warm: scale lên mức đỉnh tại T0 − 30 phút** | 5 | P0 | S5 | EVF-121, EVF-21 |
| EVF-123 | Story | Load shedding có phân lớp ưu tiên + circuit breaker giữa service | 5 | P0 | S5 | EVF-2 |
| EVF-124 | Story | **Bộ k6: 100k VU join trong 5s, 30 phút chờ, đợt checkout** | 8 | P0 | S5 | EVF-56, EVF-120 |
| EVF-125 | Story | Dashboard Grafana + alert rule cho 9 metric nghiệp vụ | 5 | P0 | S5 | EVF-3 |
| EVF-126 | Story | Chaos drill: giết Redis primary, Gemini 429 100%, Postgres failover | 5 | P1 | S5 | EVF-124 |
| EVF-127 | Task | Runbook trực sự kiện + checklist đóng băng deploy | 3 | P1 | S5 | EVF-125 |
| EVF-128 | Task | E2E Playwright luồng đầy đủ | 3 | P1 | S5 | EVF-114 |

**Tiêu chí hoàn thành epic:**

- Load test 10× đạt toàn bộ SLO ở docs/00
- Chaos drill: không oversell, không mất đơn
- Runbook đã diễn tập ít nhất 1 lần

---

## Kế hoạch sprint

Vận tốc giả định ~70 SP/sprint (đội 4–5 người), sprint 2 tuần.

| Sprint | Chủ đề | SP | Định nghĩa hoàn thành sprint |
|---|---|---:|---|
| S0 | Nền móng | 64 | `make up` chạy toàn hệ, trace xuyên ≥ 3 service, CI chặn PR lỗi |
| S1 | Lõi nghiệp vụ & chống oversell | 69 | EVF-39 xanh 200/200 lần: 10.000 goroutine mua 100 vé → đúng 100 đơn |
| S2 | Phòng chờ ảo | 70 | k6: 100k VU join < 300 ms p99, admit rate ổn định, lottery qua kiểm định thống kê |
| S3 | Thanh toán & nền tảng AI | 70 | Webhook exactly-once; 5.000 câu hỏi/phút, 0 lỗi 429 tới user |
| S4 | Anti-bot, kiểm duyệt & AI trợ lý | 70 | Bot script bị chặn ≥ 95%, false positive < 1%; 0/50 payload injection vượt rào |
| S5 | Chịu tải & vận hành | 73 | Load test 10× đạt SLO, chaos drill pass, runbook diễn tập xong |
| S6 | Ổn định & hoàn thiện | 45 | Báo cáo AI tự sinh, hạng mục P2 xong, không còn bug P0/P1 mở |
| **Tổng** | | **461** | **7 sprint · 14 tuần** |

- S0 nhẹ hơn để đội ổn định môi trường; S6 để trống ~25 SP làm vùng đệm sửa lỗi sau load test và chaos drill.
- Nếu buộc phải phát hành sau 6 sprint: cắt toàn bộ P2 (21 SP) và dời EVF-90, EVF-91, EVF-116 sang sau phát hành — không ảnh hưởng luồng mua vé.

## Tổng hợp theo epic

| Epic | SP | Sprint |
|---|---:|---|
| E1 Nền móng & Hạ tầng | 34 | S0 |
| E2 Định danh & Bảo mật | 26 | S0, S1 |
| E3 Sự kiện & Kiểm duyệt | 31 | S1, S2, S4, S6 |
| E4 Ticketing & Chống oversell | 62 | S0, S1, S3, S6 |
| E5 Phòng chờ ảo | 60 | S2, S5, S6 |
| E6 Thanh toán | 34 | S3, S5, S6 |
| E7 AI (Gemini) | 89 | S3, S4, S5, S6 |
| E8 Anti-bot | 29 | S4, S6 |
| E9 Frontend | 49 | S0, S2, S3, S5, S6 |
| E10 Vận hành, chịu tải & phát hành | 47 | S5 |
| **Tổng** | **461** | |

Theo độ ưu tiên: P0 = 328 SP · P1 = 112 SP · P2 = 21 SP.

## Đường tới hạn (critical path)

```
EVF-1 -> EVF-2/3/4/5 -> EVF-30 -> EVF-31 -> EVF-32 -> EVF-34 -> EVF-39        (S0 -> S1)
                                     |
EVF-11/13 -> EVF-50 -> EVF-51 -> EVF-52 -> EVF-54 -> EVF-55 -> EVF-56          (S1 -> S2)
                                                                  |
EVF-7 -> EVF-80 -> EVF-81 ------------------------------------> EVF-124 -> EVF-126   (S3 -> S5)
```

Ba hạng mục quyết định thành bại của cả dự án — cần làm sớm và làm kỹ nhất:

1. **EVF-39** — test đồng thời chống oversell. Đây là bất biến không được phép vi phạm.
2. **EVF-55** — admit controller. Sai ở đây thì hoặc DB sập, hoặc khách chờ vô ích.
3. **EVF-81** — limiter Gemini toàn cục. Sai ở đây thì trợ lý AI chết ngay khi cần nhất.

## Hướng dẫn import vào Jira

1. Tạo project key `EVF` (Scrum). Bật trường *Story Points* và *Sprint* trên màn hình issue.
2. Tạo sẵn các sprint `Sprint 0` … `Sprint 6` trên board (nếu chưa có, Jira sẽ tạo theo tên).
3. Jira → **Filters → Import issues from CSV** (hoặc *Settings → System → External system import → CSV*), chọn `docs/jira-import.csv`.
4. Ở bước cấu hình: **File encoding = UTF-8**, dấu phân cách `,`.
5. Ánh xạ cột:

| Cột CSV | Trường Jira |
|---|---|
| Issue Type | Issue Type |
| Issue ID | Issue ID |
| Parent ID | Parent ID (liên kết Story/Task vào Epic) |
| Summary | Summary |
| Epic Name | Epic Name (chỉ project company-managed đời cũ; không có thì bỏ qua) |
| Story Points | Story Points (hoặc Story point estimate) |
| Priority | Priority |
| Labels | Labels (các nhãn cách nhau bằng dấu cách) |
| Component | Component/s |
| Sprint | Sprint |
| Description | Description (định dạng wiki markup của Jira) |
| Blocked By (nhiều cột) | Linked Issues → loại liên kết *is blocked by* (tuỳ chọn) |

6. Sau import: kiểm tra 10 epic, mỗi epic có đủ issue con, tổng SP = 461.
