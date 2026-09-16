# EventFlow — Backlog Jira

Project key: **EVF**. File import sẵn sàng: [`jira-import.csv`](jira-import.csv)
(Jira → Filters → Import issues from CSV).

**Quy ước:** SP = story point (Fibonacci). P = độ ưu tiên (P0 chặn đường tới hạn, P1 cần cho MVP, P2 hoàn thiện).
Mỗi Story đều có tiêu chí nghiệm thu (AC) đo được — không dùng "hoạt động tốt".

---

## EVF-E1 — Nền móng & Hạ tầng (Sprint 0) · 34 SP

| Key | Loại | Tiêu đề | SP | P |
|---|---|---|---|---|
| EVF-1 | Story | Khởi tạo monorepo: go.work, pnpm workspace, uv, Makefile | 3 | P0 |
| EVF-2 | Story | `libs/go/httpx`: router, middleware (recover, log, trace, auth, ratelimit), lỗi RFC7807 | 5 | P0 |
| EVF-3 | Story | `libs/go/otelx`: OpenTelemetry trace + Prometheus metric, propagate qua AMQP header | 5 | P0 |
| EVF-4 | Story | `docker-compose` local: Postgres, PgBouncer, Redis, RabbitMQ, MinIO, Jaeger, Prometheus, Grafana | 5 | P0 |
| EVF-5 | Story | Khung migration goose + schema khởi tạo | 3 | P0 |
| EVF-6 | Story | CI pipeline: lint → unit → integration (testcontainers) → build → Trivy scan | 5 | P0 |
| EVF-7 | Story | `libs/go/mq`: publisher + consumer có retry/DLQ, khai báo topology | 5 | P0 |
| EVF-8 | Task | Template service Go (hexagonal) + generator để tạo service mới | 3 | P1 |

> **AC EVF-4:** `make up` khởi động toàn bộ; `make smoke` chạy qua 1 luồng mua vé end-to-end; một trace duy nhất nhìn thấy được trong Jaeger xuyên ≥ 3 service.

---

## EVF-E2 — Định danh & Bảo mật · 26 SP

| Key | Loại | Tiêu đề | SP | P |
|---|---|---|---|---|
| EVF-10 | Story | Đăng ký/đăng nhập, hash mật khẩu argon2id | 5 | P0 |
| EVF-11 | Story | Xác thực OTP qua SMS/email (bắt buộc trước khi xếp hàng — BR-Q3) | 5 | P0 |
| EVF-12 | Story | JWT ngắn hạn + refresh token xoay vòng, thu hồi được | 5 | P0 |
| EVF-13 | Story | Đăng ký device fingerprint, liên kết identity ↔ device | 3 | P1 |
| EVF-14 | Story | RBAC: Buyer / Organizer / Moderator / Ops | 5 | P1 |
| EVF-15 | Task | Mã hoá PII ở tầng cột + lọc PII khỏi log | 3 | P1 |

---

## EVF-E3 — Sự kiện & Kiểm duyệt · 31 SP

| Key | Loại | Tiêu đề | SP | P |
|---|---|---|---|---|
| EVF-20 | Story | CRUD sự kiện + hạng vé, state machine DRAFT→…→COMPLETED (BR-E1) | 8 | P0 |
| EVF-21 | Story | Cấu hình lịch mở bán, ràng buộc T0 ≥ now + 30' (BR-E2) | 3 | P0 |
| EVF-22 | Story | Khoá không cho giảm quota sau khi ON_SALE (BR-E3) | 3 | P0 |
| EVF-23 | Story | Đẩy job kiểm duyệt AI khi submit, nhận kết quả bất đồng bộ | 5 | P1 |
| EVF-24 | Story | Hàng đợi việc cho Moderator: duyệt / từ chối / lật ngược quyết định AI (BR-E5) | 5 | P1 |
| EVF-25 | Story | Trang sự kiện công khai render ISR, phục vụ từ CDN | 5 | P0 |
| EVF-26 | Task | Upload ảnh lên object storage + sinh biến thể | 2 | P2 |

---

## EVF-E4 — Ticketing & Chống oversell (**epic rủi ro nhất**) · 55 SP

| Key | Loại | Tiêu đề | SP | P |
|---|---|---|---|---|
| EVF-30 | Story | Schema tồn kho có phân mảnh bucket (32 bucket/hạng vé) + `CHECK (available >= 0)` | 5 | P0 |
| EVF-31 | Story | Redis Lua inventory gate: hold atomic, giảm tồn, ghi ZSET hết hạn | 8 | P0 |
| EVF-32 | Story | Ghi hold vào Postgres trong transaction, chọn bucket ngẫu nhiên + fallback vòng tròn | 8 | P0 |
| EVF-33 | Story | Middleware Idempotency-Key (`libs/go/idem`) cho mọi API ghi (BR-O5) | 5 | P0 |
| EVF-34 | Story | Hold sweeper: quét ZSET mỗi 1s, trả kho **idempotent** (BR-O7) | 8 | P0 |
| EVF-35 | Story | Giới hạn mua: `max_per_order`, `max_per_identity` gộp mọi đơn (BR-O4) | 5 | P0 |
| EVF-36 | Story | Transactional outbox + relay ra RabbitMQ | 5 | P0 |
| EVF-37 | Story | Job đối soát tồn kho Redis ↔ Postgres mỗi 10s, export metric `inventory_drift` | 5 | P0 |
| EVF-38 | Story | Phát hành vé + QR ký HMAC có `jti` (BR-O8) | 5 | P1 |
| EVF-39 | **Test** | **Test đồng thời: 10.000 goroutine mua 100 vé → đúng 100 đơn, 0 oversell** | 5 | **P0** |
| EVF-40 | Task | API check-in, đổi trạng thái ISSUED→CHECKED_IN một lần duy nhất | 3 | P2 |

> **AC EVF-39 (cổng chất lượng không thể bỏ qua):** chạy 200 lần trong CI, 0 lần oversell,
> 0 lần under-sell (bán thiếu do bug trả kho). Test này fail → không được merge, không được release.

---

## EVF-E5 — Phòng chờ ảo · 55 SP

| Key | Loại | Tiêu đề | SP | P |
|---|---|---|---|---|
| EVF-50 | Story | Queue token ký, gắn identity+device+event, join idempotent (BR-Q3) | 5 | P0 |
| EVF-51 | Story | `join.lua`: O(1), một Redis round-trip, shard theo event | 5 | P0 |
| EVF-52 | Story | LOBBY trước T0 + lottery xáo trộn công bằng tại T0 (BR-Q1) | 8 | P0 |
| EVF-53 | Story | Nối đuôi FIFO sau T0 (BR-Q2) | 3 | P0 |
| EVF-54 | Story | `GET /queue/status`: rank, ETA, `poll_after_ms` thích nghi + ETag (BR-Q4) | 8 | P0 |
| EVF-55 | Story | Admit controller AIMD, bầu leader qua Redis lock có fencing (BR-Q5) | 8 | P0 |
| EVF-56 | Story | `admit.lua`: ZPOPMIN theo lô → tập admitted, TTL 15' (BR-Q7) | 5 | P0 |
| EVF-57 | Story | SSE push "tới lượt bạn" + fallback polling | 5 | P0 |
| EVF-58 | Story | Heartbeat + giữ chỗ 5' khi rớt kết nối | 3 | P1 |
| EVF-59 | Story | Xử lý SOLD_OUT: thông báo hàng chờ, chuyển sang waitlist (BR-Q6) | 3 | P1 |
| EVF-60 | Story | Ops API + UI: xem/ghi đè `admit_rate`, tạm dừng khẩn cấp | 5 | P1 |
| EVF-61 | Task | Jitter client 0..5s tại T0 | 2 | P0 |

> **AC EVF-55:** khi tiêm độ trễ giả vào checkout, `admit_rate` phải tự giảm trong ≤ 10s và phục hồi trong ≤ 60s sau khi độ trễ hết. Không dao động (oscillation) quá 2 chu kỳ.

---

## EVF-E6 — Thanh toán · 34 SP

| Key | Loại | Tiêu đề | SP | P |
|---|---|---|---|---|
| EVF-70 | Story | Adapter cổng thanh toán (VNPay/Momo/Stripe) + sandbox mock | 8 | P0 |
| EVF-71 | Story | Webhook exactly-once: UNIQUE `provider_txn_id`, verify chữ ký, chống replay (BR-O6) | 8 | P0 |
| EVF-72 | Story | `payment_inbox` cho webhook đến sớm + replay | 5 | P1 |
| EVF-73 | Story | Job đối soát: đơn quá 15' không webhook → hỏi cổng thanh toán, tự chữa | 5 | P0 |
| EVF-74 | Story | Luồng hoàn tiền khi sự kiện bị huỷ | 5 | P2 |
| EVF-75 | Task | Email/SMS xác nhận + đính vé qua notification-svc | 3 | P1 |

---

## EVF-E7 — AI (Gemini) · 63 SP

| Key | Loại | Tiêu đề | SP | P |
|---|---|---|---|---|
| EVF-80 | Story | Khung ai-worker: FastAPI + 4 consumer RabbitMQ tách biệt (BR-A3) | 8 | P0 |
| EVF-81 | Story | **Global token-bucket limiter RPM+TPM trên Redis Lua, đặt chỗ token trước khi gọi** | 8 | P0 |
| EVF-82 | Story | Retry backoff qua delay-queue 5s/30s/2m + DLQ | 5 | P0 |
| EVF-83 | Story | Tầng FAQ/intent trả lời 0 token (~50 mẫu) | 5 | P0 |
| EVF-84 | Story | Cache ngữ nghĩa bằng embedding trên Redis, ngưỡng 0.93, scope theo event | 8 | P0 |
| EVF-85 | Story | Singleflight gộp câu hỏi trùng đang in-flight | 3 | P1 |
| EVF-86 | Story | Tool-call lấy số liệu thật (vị trí, ETA, tình trạng vé) — cấm bịa số (BR-A5) | 5 | P0 |
| EVF-87 | Story | 4 nấc suy biến NORMAL/SAVING/FAQ_ONLY/OFF + công tắc cho Ops (BR-A7) | 5 | P0 |
| EVF-88 | Story | Ngân sách token theo sự kiện, cảnh báo 80%, tự khoá 100% (BR-A6) | 3 | P1 |
| EVF-89 | Story | Kiểm duyệt sự kiện: structured JSON output, chấm 4 trục, ngưỡng định tuyến (BR-E4) | 8 | P1 |
| EVF-90 | Story | Phát hiện bất thường: gom cụm thống kê trước, LLM chỉ diễn giải & đề xuất rule (BR-B2) | 8 | P1 |
| EVF-91 | Story | Báo cáo sau bán 6 phần: SQL tính số, LLM viết | 8 | P1 |
| EVF-92 | Story | Chống prompt injection + lọc đầu ra (BR-A7) | 5 | P0 |
| EVF-93 | Task | Bộ eval prompt: gold set 200 câu, chạy khi đổi prompt/model | 5 | P2 |
| EVF-94 | Story | API trợ lý phía user: 202 + job_id, SSE trả kết quả, quota 10 câu/15' (BR-A1, BR-A2) | 5 | P0 |

> **AC EVF-81:** bắn 5.000 câu hỏi/phút liên tục 10 phút với hạn mức giả lập 60 RPM →
> `gemini_429_total == 0`, không job nào mất, p95 phản hồi < 4s, cache hit ≥ 80%.

---

## EVF-E8 — Anti-bot · 29 SP

| Key | Loại | Tiêu đề | SP | P |
|---|---|---|---|---|
| EVF-100 | Story | Thu thập tín hiệu client (nhịp, tương tác, fingerprint) gửi kèm request | 5 | P0 |
| EVF-101 | Story | Rule engine R1–R7 chạy in-path < 5ms | 8 | P0 |
| EVF-102 | Story | Chấm điểm + 4 tầng hành động (qua / thử thách / làn chậm / chặn) | 5 | P0 |
| EVF-103 | Story | Tích hợp Turnstile ở bước join và bước thử thách nâng cao | 3 | P1 |
| EVF-104 | Story | `bot_decisions` giải thích được + UI khiếu nại/gỡ chặn (BR-B1) | 5 | P1 |
| EVF-105 | Task | Quy trình duyệt rule do AI đề xuất | 3 | P2 |

---

## EVF-E9 — Frontend · 42 SP

| Key | Loại | Tiêu đề | SP | P |
|---|---|---|---|---|
| EVF-110 | Story | Khung Next.js App Router + design system + dark mode | 5 | P0 |
| EVF-111 | Story | Trang sự kiện ISR + đồng hồ đếm ngược theo giờ server | 5 | P0 |
| EVF-112 | Story | UI phòng chờ: vị trí, ETA, tiến trình, polling thích nghi + SSE | 8 | P0 |
| EVF-113 | Story | Chọn vé + đồng hồ giữ ghế 10:00 theo `expires_at` server (BR-O2) | 8 | P0 |
| EVF-114 | Story | Luồng thanh toán + trang kết quả (kể cả trường hợp hold hết hạn giữa chừng) | 5 | P0 |
| EVF-115 | Story | Panel chat trợ lý: streaming, FAQ chips, trạng thái suy biến | 5 | P1 |
| EVF-116 | Story | Console Organizer: tạo sự kiện, trạng thái duyệt, doanh số realtime | 5 | P1 |
| EVF-117 | Story | War room cho Ops: phễu realtime, admit rate, tồn kho, AI lag | 5 | P1 |
| EVF-118 | Task | Accessibility (WCAG AA) + i18n vi/en | 3 | P2 |

---

## EVF-E10 — Vận hành, chịu tải, phát hành · 42 SP

| Key | Loại | Tiêu đề | SP | P |
|---|---|---|---|---|
| EVF-120 | Story | K8s manifest mọi service: probe, resource, PDB, anti-affinity | 8 | P0 |
| EVF-121 | Story | HPA theo custom metric (rps, queue_depth) qua Prometheus adapter | 5 | P0 |
| EVF-122 | Story | CronJob pre-warm: scale lên mức đỉnh tại T0 − 30' | 5 | P0 |
| EVF-123 | Story | Load shedding có phân lớp ưu tiên + circuit breaker giữa service | 5 | P0 |
| EVF-124 | Story | Bộ k6: 100k VU join trong 5s, 30' chờ, đợt checkout | 8 | P0 |
| EVF-125 | Story | Dashboard Grafana + alert rule cho 9 metric nghiệp vụ | 5 | P0 |
| EVF-126 | Story | Chaos drill: giết Redis primary, Gemini 429 100%, Postgres failover | 5 | P1 |
| EVF-127 | Task | Runbook trực sự kiện + checklist đóng băng deploy | 3 | P1 |
| EVF-128 | Task | E2E Playwright luồng đầy đủ | 3 | P1 |

---

## Tổng hợp

| Epic | SP | Sprint chính |
|---|---:|---|
| E1 Nền móng | 34 | S0 |
| E2 Định danh | 26 | S1 |
| E3 Sự kiện | 31 | S1 |
| E4 Ticketing | 55 | S1–S2 |
| E5 Phòng chờ | 55 | S2 |
| E6 Thanh toán | 34 | S3 |
| E7 AI | 63 | S3–S4 |
| E8 Anti-bot | 29 | S4 |
| E9 Frontend | 42 | S2–S5 |
| E10 Vận hành | 42 | S5 |
| **Tổng** | **411 SP** | 6 sprint |

Với vận tốc ~70 SP/sprint (đội 4–5 người) → **12 tuần**.

## Đường tới hạn (critical path)

```
EVF-1 -> EVF-2/3/4 -> EVF-30 -> EVF-31 -> EVF-32 -> EVF-34 -> EVF-39
                                    |
                                    +-> EVF-51 -> EVF-52 -> EVF-55 -> EVF-56 -> EVF-124
```

Ba hạng mục quyết định thành bại của cả dự án — cần làm sớm và làm kỹ nhất:

1. **EVF-39** — test đồng thời chống oversell. Đây là bất biến không được phép vi phạm.
2. **EVF-55** — admit controller. Sai ở đây thì hoặc DB sập, hoặc khách chờ vô ích.
3. **EVF-81** — limiter Gemini toàn cục. Sai ở đây thì trợ lý AI chết ngay khi cần nhất.
