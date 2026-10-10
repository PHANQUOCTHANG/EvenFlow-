# EventFlow — Kế hoạch hoàn thiện backend

Cập nhật: 2026-10-10. Phạm vi: `services/`, `libs/go/`, `migrations/`, `deploy/`, `tests/`.
Frontend (`apps/web`, epic E9) không nằm trong kế hoạch này; chỉ ghi các điểm backend phải sẵn sàng cho frontend.

Mã `EVF-n` là issue trong [04-jira-backlog.md](04-jira-backlog.md). Kế hoạch này **không thay** backlog: backlog nói *làm gì*, tài liệu này nói *làm theo thứ tự nào* dựa trên hiện trạng code.

## 1. Vì sao cần thứ tự khác với kế hoạch sprint

Kế hoạch sprint trong backlog giả định bắt đầu từ repo trống. Thực tế repo đã có sẵn phần lõi khó nhất (Lua script, chống oversell, admit controller AIMD, limiter Gemini) nhưng thiếu phần nối chúng lại:

- 6 service (`gateway`, `identity`, `event`, `payment`, `antibot`, `notification`) mới chỉ có `/healthz`.
- `libs/go` chỉ có `redisx`; `httpx`, `otelx`, `mq`, `outbox`, `idem`, `authx`, `leader`, `testx` đều rỗng.
- Trước giai đoạn 0, code đã viết không chạy được thành một luồng: Redis không có tồn kho, không ai chạy admit controller, PgBouncer không đăng nhập được Postgres.

Nên thứ tự ở đây là: nối cho chạy → dựng nền → hoàn thiện từng năng lực, mỗi giai đoạn kết thúc bằng một thứ **chạy được và kiểm được**.

## 2. Toàn cảnh

| GĐ | Tên | Việc chính | Loại | Phụ thuộc | Trạng thái |
|---|---|---|---|---|---|
| 0 | Nối dây | Seed, nạp tồn kho Redis, chạy worker + admit controller, smoke | Nối dây | — | **Xong** (nhánh `feat/phase-0-wiring`) |
| 1 | Thư viện dùng chung | HTTP, quan trắc, test tích hợp, idempotency, message queue, outbox | Nền | 0 | **Đang làm** — PR A (`go.sum` + `httpx`) và PR B (`otelx`) xong, chờ review |
| 2 | Ticketing hoàn chỉnh | Đối soát tồn kho, giới hạn mua, API đơn hàng / huỷ hold / tình trạng vé | Nghiệp vụ + hệ thống | 1 | Chưa làm |
| 3 | Identity + Gateway | Đăng ký/đăng nhập, OTP, JWT, RBAC, queue token có ký, gateway | Nghiệp vụ | 1 | Chưa làm |
| 4 | Event | CRUD sự kiện, state machine, lịch mở bán, khoá quota | Nghiệp vụ | 3 | Chưa làm |
| 5 | Phòng chờ hoàn chỉnh | Bầu leader, đo sức khoẻ thật, lottery, heartbeat, hết vé, API Ops | Hệ thống | 3, 4 | Chưa làm |
| 6 | Thanh toán + phát vé | Webhook exactly-once, đối soát đơn treo, vé QR, email | Nghiệp vụ | 2, 4 | Chưa làm |
| 7 | AI | Consumer, Gemini thật, cache, suy biến, kiểm duyệt | Nghiệp vụ + hệ thống | 1 (phần `mq`), 4 | Chưa làm |
| 8 | Anti-bot | Rule engine, chấm điểm rủi ro, Turnstile | Nghiệp vụ | 3 | Chưa làm |
| 9 | Vận hành + chịu tải | K8s, autoscale, load shedding, k6 100k VU, chaos drill | Hệ thống | tất cả | Chưa làm |

```
0 ──> 1 ──┬──> 2 ─────────────┬──> 6 ──┐
          ├──> 3 ──┬──> 4 ────┤        ├──> 9
          │        ├──> 8     └──> 5 ──┤
          └──> 7 <─┘ (cần 4 cho kiểm duyệt)
```

- Giai đoạn 0 → 1 → 2 đi tuần tự, một người làm được.
- Sau giai đoạn 1 có thể chia song song: nhánh 3 → 4, nhánh 2, nhánh 7.
- Sau giai đoạn 6, một khách đi được trọn luồng: đăng ký → xếp hàng → mua → trả tiền → nhận vé.

### Ba bài toán hệ thống quyết định thành bại

| Bài toán | Giải ở | Hiện trạng |
|---|---|---|
| Không bán quá vé dưới tải đồng thời (BR-O1) | GĐ 1 (test) + GĐ 2 (đối soát) | Lõi đã có code; test EVF-39 còn `t.Skip`, chưa có đối soát |
| Biến đỉnh tải thành dòng chảy đều (BR-Q5) | GĐ 5 | Controller đã chạy nhưng "mù": probe sức khoẻ là bản tĩnh, chưa bầu leader |
| AI không bao giờ chạm trần rate limit (BR-A1) | GĐ 7 | Limiter đã có; chưa có consumer nào chạy |

Giai đoạn 9 kiểm chứng cả ba dưới tải thật.

## 3. Chi tiết từng giai đoạn

### Giai đoạn 0 — Nối những gì đã viết cho chạy được · XONG

| Việc | Kết quả |
|---|---|
| Script seed | `deploy/scripts/seed.sh`: sự kiện `ON_SALE`, 32 bucket tồn kho, ghi `sale_start_ms` |
| Nạp tồn kho Redis | `InventorySeeder` trong worker, dùng `HSETNX` nên không ghi đè số đang bị hold trừ |
| Chạy worker | Image ticketing có thêm `/worker`; service `ticketing-worker` |
| Chạy admit controller | `services/waitingroom/cmd/controller`, config validate lúc khởi động; service `waitingroom-controller` |
| Script smoke | `deploy/scripts/smoke.sh`: 15 bước, kiểm cả Postgres lẫn Redis |
| Sửa lỗi tích hợp | PgBouncer dùng SCRAM để đăng nhập được Postgres 16 |

**Tiêu chí đã đạt:** từ volume trống, `up → migrate → smoke` xanh 15/15, lặp lại 3 lần.
Nợ để lại: xem [workflow/phase-0/report.md](workflow/phase-0/report.md).

### Giai đoạn 1 — Thư viện dùng chung

| # | Việc | Issue | Nội dung |
|---|---|---|---|
| W0 | Track `go.sum` | — | Sinh và commit `go.sum`, `go.work.sum`; sửa Dockerfile, cache CI |
| W1 | `libs/go/httpx` | EVF-2 | Lỗi RFC 7807, middleware recover / request-id / access-log, tắt êm, health dùng chung |
| W2 | `libs/go/otelx` | EVF-3 | Trace OTLP, metric Prometheus, log gắn `trace_id`; metric cho controller và worker |
| W3 | `libs/go/testx` + test oversell thật | EVF-39 | Testcontainers Postgres / Redis / RabbitMQ; bỏ `t.Skip` trong `helpers.go` |
| W4 | `libs/go/idem` | EVF-33 | Middleware `Idempotency-Key`: trả lại response cũ, 409 khi đang xử lý, 422 khi body khác |
| W5 | `libs/go/mq` | EVF-7 | Publisher có confirm, consumer có retry / DLQ, lan truyền trace qua header |
| W6 | `libs/go/outbox` + relay | EVF-36 | Relay `FOR UPDATE SKIP LOCKED`; `hold.released` đi qua outbox thay vì publish thẳng |

Chia 3 PR: **A** = W0 + W1 · **B** = W2 · **C** = W3 → W6.

**Xong khi:**

- `TestNoOversell` chạy thật 200 lần: 0 oversell, 0 under-sell.
- Gửi lại cùng `Idempotency-Key` nhận đúng response cũ.
- Tắt rồi bật RabbitMQ không mất message `ticketing.hold.released`.
- Prometheus thấy đủ target; một request giữ ghế hiện thành trace trong Jaeger.

### Giai đoạn 2 — Ticketing hoàn chỉnh phần giữ ghế

| Việc | Issue | Nội dung |
|---|---|---|
| Đối soát tồn kho | EVF-37 | So Redis ↔ Postgres mỗi 10 giây, sửa lệch, metric `inventory_drift`; thay seeder tối thiểu của GĐ 0 |
| Giới hạn mua | EVF-35 | Chốt `CountPurchased` theo BR-O4 (tính đơn đang giữ hay chỉ đơn đã trả tiền) và viết test |
| API đơn hàng | — | `GET /v1/orders/{id}` cho trang giữ ghế và trang kết quả |
| Huỷ hold | — | Khách chủ động huỷ → trả kho ngay (dùng lại `ReleaseHold`) |
| Tình trạng vé | — | `GET /v1/events/{id}/availability` trả `MANY` / `FEW` / `SOLD_OUT` (BR-A4: không lộ số chính xác) |
| Dọn dẹp | — | Xoá `idempotency_keys` hết hạn và `outbox_events` đã phát |

**Xong khi:** cố ý làm lệch Redis thì `inventory_drift` về 0 trong ≤ 20 giây; chạy 2 sweeper song song không trả kho hai lần.

### Giai đoạn 3 — Identity và Gateway

| Việc | Issue | Nội dung |
|---|---|---|
| `libs/go/authx` | — | Ký / xác minh JWT, queue token, QR HMAC |
| Đăng ký, đăng nhập | EVF-10 | Băm mật khẩu argon2id |
| JWT + refresh | EVF-12 | Token ngắn hạn, refresh xoay vòng, thu hồi được |
| OTP | EVF-11 | SMS / email, có mock cho local; chưa verify thì không xếp hàng được (BR-Q3) |
| Device fingerprint | EVF-13 | Liên kết identity ↔ thiết bị |
| RBAC | EVF-14 | Buyer / Organizer / Moderator / Ops |
| Mã hoá PII | EVF-15 | Mã hoá cột + lọc PII khỏi log |
| Gateway | — | Xác minh JWT, định tuyến, CORS, chặn poll dày hơn `poll_after_ms` |
| Queue token có ký | EVF-50 | Gắn identity + device + event |
| Bỏ tin header | — | Waitingroom và ticketing không còn nhận `X-Identity-Id` từ client |
| Template service | EVF-8 | Rút khuôn mẫu từ `identity` — service mới đầu tiên |

**Xong khi:** mọi request đi qua gateway ở cổng 8080; k6 và web gọi được backend; ma trận quyền ở [01-nghiep-vu.md](01-nghiep-vu.md) được phủ bằng test.

### Giai đoạn 4 — Event service

| Việc | Issue | Nội dung |
|---|---|---|
| CRUD + state machine | EVF-20 | `DRAFT → PENDING_REVIEW → SCHEDULED → ON_SALE → … → COMPLETED`; chuyển sai bị từ chối |
| Lịch mở bán | EVF-21 | T0 ≥ thời điểm duyệt + 30 phút |
| Khoá quota | EVF-22 | Không cho giảm quota sau `ON_SALE` (BR-E3) |
| Chuẩn bị mở bán | — | Khi sang `SCHEDULED`: tạo bucket tồn kho, ghi `sale_start_ms` — thay phần `seed.sh` làm tạm |
| Danh sách sự kiện đang bán | — | Thay biến `ACTIVE_EVENT_IDS` của sweeper, seeder, controller |
| API công khai | EVF-25 | `GET /v1/events/{id}` cho trang ISR và cho tool-call của AI |
| Upload ảnh | EVF-26 | Object storage + biến thể (P2, có thể dời) |

**Xong khi:** tạo sự kiện qua API rồi mua vé được, không cần seed tay.

### Giai đoạn 5 — Phòng chờ hoàn chỉnh

| Việc | Issue | Nội dung |
|---|---|---|
| Bầu leader | EVF-55 | `libs/go/leader`: Redis lock có fencing; controller chỉ chạy khi giữ lock |
| Đo sức khoẻ thật | EVF-55 | `HealthProbe` đọc p99, tỷ lệ lỗi, mức dùng pool của ticketing — thay probe tĩnh |
| Lottery tại T0 | EVF-52, 53 | Kiểm với T0 thật; kiểm định thống kê: thời điểm vào không tương quan với rank |
| Heartbeat | EVF-58 | Giữ chỗ 5 phút khi rớt kết nối |
| Hết vé | EVF-59 | Ticketing phát event → dừng admit → báo hàng chờ, chuyển waitlist |
| API Ops | EVF-60 | Xem / ghi đè / bỏ ghi đè admit rate, tạm dừng khẩn cấp |
| Audit | — | Ghi `queue_sessions` |

**Xong khi:** `admit_rate` tự giảm ≤ 10 giây khi checkout chậm và phục hồi ≤ 60 giây; chạy 2 controller không admit gấp đôi; k6 `opening-spike.js` chạy trọn luồng.

### Giai đoạn 6 — Thanh toán và phát vé

| Việc | Issue | Nội dung |
|---|---|---|
| Adapter cổng thanh toán | EVF-70 | VNPay / Momo / Stripe + sandbox mock chạy local |
| Webhook exactly-once | EVF-71 | UNIQUE `provider_txn_id`, xác minh chữ ký, chống replay (BR-O6) |
| Inbox | EVF-72 | Webhook đến trước khi đơn tồn tại → lưu, replay sau |
| Đối soát đơn treo | EVF-73 | Quá 15 phút không có webhook → hỏi cổng thanh toán, tự chữa |
| Xác nhận thanh toán | — | Ticketing: `Consume` hold (không trả kho), đơn → `PAID` |
| Phát vé | EVF-38 | QR ký HMAC có `jti` (BR-O8) |
| Thông báo | EVF-75 | Notification gửi email xác nhận kèm vé |
| Hoàn tiền | EVF-74 | Khi sự kiện bị huỷ (P2) |
| Check-in | EVF-40 | `ISSUED → CHECKED_IN` đúng một lần (P2) |

**Xong khi:** bắn cùng một webhook 100 lần → đúng 1 đơn `PAID`, 1 bộ vé; không còn đơn lửng lơ quá 15 phút; bỏ `test.skip` ở E2E.

### Giai đoạn 7 — AI worker

| Việc | Issue | Nội dung |
|---|---|---|
| Khởi động consumer | EVF-80 | 4 consumer RabbitMQ tách biệt; nối `ChatConsumer` đã viết |
| Retry | EVF-82 | Nack qua delay-queue 5s / 30s / 2m, hết lượt → DLQ |
| Gemini thật | EVF-80 | SDK, timeout cứng, đếm token để hoàn permit; embedder cho cache |
| API trợ lý | EVF-94 | 202 + `job_id`, SSE trả kết quả, quota 10 câu / 15 phút |
| Tool-call số liệu thật | EVF-86 | Vị trí, ETA, tình trạng vé lấy từ API thật |
| Suy biến | EVF-87, 88 | Tín hiệu thật (lag hàng đợi, ngân sách token), công tắc Ops |
| Chống injection | EVF-92 | Lọc đầu ra + bộ 50 payload |
| Cache ngữ nghĩa | EVF-84, 85 | Vector search thay quét tuyến tính |
| Kiểm duyệt | EVF-89, 23, 24 | Consumer chấm 4 trục; hàng việc Moderator bên event |
| Test | — | Hiện 0 test Python |
| Có thể dời | EVF-90, 91, 93 | Phát hiện bất thường, báo cáo sau bán, bộ eval |

**Xong khi:** 5.000 câu / phút với hạn mức 60 RPM → `gemini_429_total == 0`; Gemini sập hoàn toàn khách vẫn nhận câu trả lời FAQ; 0/50 payload injection vượt rào.

### Giai đoạn 8 — Anti-bot

| Việc | Issue | Nội dung |
|---|---|---|
| Nhận tín hiệu | EVF-100 | Nhịp, tương tác, fingerprint từ client |
| Rule engine | EVF-101 | R1–R7 chạy in-path < 5 ms |
| Chấm điểm | EVF-102 | 4 tầng: cho qua / thử thách / làn chậm / chặn |
| Turnstile | EVF-103 | Ở bước join và bước thử thách |
| Giải thích được | EVF-104 | Ghi `bot_decisions`, API khiếu nại / gỡ chặn |
| Nối vào join | — | Thay `BotGuard = nil` trong handler waitingroom |
| Có thể dời | EVF-105 | Duyệt rule do AI đề xuất (cần EVF-90) |

**Xong khi:** bot script mẫu bị chặn ≥ 95%, false positive < 1%; rule engine p99 < 5 ms.

### Giai đoạn 9 — Vận hành, chịu tải, phát hành

| Việc | Issue | Nội dung |
|---|---|---|
| K8s manifest | EVF-120 | Mọi service: probe, resource, PDB, anti-affinity |
| Autoscale | EVF-121 | HPA theo `rps`, `queue_depth` |
| Pre-warm | EVF-122 | CronJob scale lên mức đỉnh tại T0 − 30 phút |
| Load shedding | EVF-123 | Phân lớp ưu tiên + circuit breaker giữa service |
| Dashboard + alert | EVF-125 | 9 metric nghiệp vụ, màn hình war room |
| Load test | EVF-124 | k6 100k VU join trong 5 giây, 30 phút chờ, đợt checkout |
| Chaos drill | EVF-126 | Giết Redis primary, Gemini 429 100%, Postgres failover |
| Runbook | EVF-127 | Trực sự kiện + checklist đóng băng deploy |
| E2E đầy đủ | EVF-128 | Playwright trọn luồng trong CI |

**Xong khi:** load test 10× đạt toàn bộ SLO ở [00-tong-quan.md](00-tong-quan.md); chaos drill không oversell, không mất đơn.

## 4. Điểm backend phải sẵn sàng cho frontend

| Màn hình frontend | Cần backend | Có từ |
|---|---|---|
| Phòng chờ (EVF-112) | join / status / stream | Đã có; qua gateway từ GĐ 3 |
| Chọn vé + đồng hồ giữ ghế (EVF-113) | `POST /holds`, `GET /orders/{id}` | GĐ 2 |
| Thanh toán + kết quả (EVF-114) | Tạo phiên thanh toán, trạng thái đơn | GĐ 6 |
| Trang sự kiện ISR (EVF-111) | `GET /v1/events/{id}` | GĐ 4 |
| Chat trợ lý (EVF-115) | API trợ lý + SSE | GĐ 7 |
| Console Organizer (EVF-116) | CRUD sự kiện, trạng thái kiểm duyệt | GĐ 4, 7 |
| War room Ops (EVF-117) | API Ops, metric | GĐ 5, 9 |

Cho tới giai đoạn 3, trình duyệt chưa gọi được backend: web trỏ về cổng 8080 nhưng chưa có gateway.

## 5. Nếu thiếu thời gian

Theo đúng nguyên tắc cắt của backlog, các hạng mục sau dời được mà không ảnh hưởng luồng mua vé:

- P2: EVF-26 (upload ảnh), EVF-40 (check-in), EVF-74 (hoàn tiền), EVF-93 (eval), EVF-105 (duyệt rule AI).
- P1 dời sau phát hành: EVF-90 (phát hiện bất thường), EVF-91 (báo cáo sau bán).

Không được cắt: giai đoạn 1 (W3, W4, W6), giai đoạn 2, giai đoạn 5 và giai đoạn 9 — đây là nơi ba bài toán hệ thống được giải và được chứng minh.

## 6. Điểm còn chờ chốt

| # | Câu hỏi | Đề xuất |
|---|---|---|
| 1 | Metric dùng thẳng thư viện Prometheus hay qua OpenTelemetry metrics? | Prometheus — khớp tên trong `alerts.yml`, ít dependency |
| 2 | Topology RabbitMQ: `definitions.json` là nguồn duy nhất hay code Go tự khai báo? | `definitions.json` — ai-worker Python dùng chung |
| 3 | EVF-39 làm ở giai đoạn 1 hay 2? | Giai đoạn 1 — vừa là bất biến quan trọng nhất, vừa chứng minh `testx` dùng được |
| 4 | EVF-8 (generator service) làm khi nào? | Giai đoạn 3, rút từ `identity` |
| 5 | BR-O4: giới hạn mua tính cả đơn đang giữ hay chỉ đơn đã trả tiền? | Chốt ở đầu giai đoạn 2 |

## 7. Cách làm việc áp dụng cho mọi giai đoạn

- Mỗi giai đoạn một nhánh từ `develop`, chia PR nhỏ kiểm được riêng.
- Trước khi coi là xong: `gofmt`, `go vet`, `go test -race`, golangci-lint 0 lỗi, Trivy không CRITICAL, coverage `domain/` + `app/` ≥ 70%, smoke xanh từ volume trống.
- Test viết từ nghiệp vụ. Test đúng nghiệp vụ mà fail thì báo bug, không sửa test cho xanh.
- Mỗi giai đoạn để lại `docs/workflow/phase-N/report.md`: đã làm gì, bằng chứng, nợ cố ý.
