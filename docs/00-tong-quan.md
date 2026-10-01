# EventFlow — Kế hoạch tổng quan

> Nền tảng bán vé sự kiện chịu tải đột biến, có Phòng chờ ảo (Virtual Waiting Room)
> và AI hỗ trợ vận hành.

## 1. Bài toán một câu

20:00:00 mở bán concert. ~300.000 người bấm cùng lúc để giành ~20.000 vé.
Hệ thống phải: **công bằng**, **không bán quá 1 vé nào**, **không sập**,
**chặn bot**, và **trả lời hàng nghìn câu hỏi** trong lúc khách chờ.

## 2. Nguyên lý kiến trúc cốt lõi

> **Phòng chờ ảo không phải là tính năng UX. Nó là bộ điều tiết tải (admission
> controller) biến một đỉnh tải vô hạn thành một dòng tải hằng số có thể dự đoán.**

Hệ quả thiết kế quan trọng nhất:

| Tầng | Chịu tải | Đặc tính |
|---|---|---|
| CDN / Edge | 300k rps | Tĩnh 100%, không chạm backend |
| Waiting Room (Redis) | 300k rps | O(1), chỉ `INCR` + `ZADD`, shard theo event |
| Ticketing / Postgres | **~200 rps (cố định)** | Bị chặn bởi `admit_rate`, tải **không phụ thuộc** số người xếp hàng |
| AI (Gemini) | ~60 rpm | Bị chặn bởi rate-limiter toàn cục + cache ngữ nghĩa |

Tức là dù 300k hay 3 triệu người vào, **Postgres vẫn chỉ thấy đúng ngần ấy request**.
Toàn bộ áp lực được hấp thụ ở tầng Redis/CDN — nơi rẻ và scale ngang tuyến tính.

## 3. 5 đảm bảo (Guarantees) của hệ thống

| # | Đảm bảo | Cơ chế |
|---|---|---|
| G1 | **Công bằng** — không thưởng cho người mạng nhanh / bot bấm sớm | Lottery ngẫu nhiên cho nhóm vào trước T0, FIFO sau T0 |
| G2 | **Không oversell tuyệt đối** | 2 tầng: Redis Lua atomic gate + Postgres row-level `CHECK (available >= 0)`, đối soát định kỳ |
| G3 | **Giữ ghế 10 phút, tự nhả chính xác** | Redis ZSET theo `expire_at` + sweeper worker 1s, không phụ thuộc keyspace notification |
| G4 | **Không sập ở T0** | Pre-warm pod trước 30', HPA custom metric, load shedding có ưu tiên, circuit breaker |
| G5 | **AI luôn trả lời, không bao giờ 429 ra tới user** | 100% qua RabbitMQ, limiter toàn cục, cache ngữ nghĩa, suy biến có tầng (degradation) |

## 4. Lộ trình 7 sprint (14 tuần)

| Sprint | Mục tiêu | Định nghĩa hoàn thành |
|---|---|---|
| **S0** — Nền móng | Monorepo, CI/CD, compose, migration, observability | `docker compose up` chạy toàn hệ, trace xuyên service |
| **S1** — Lõi nghiệp vụ | Identity, Event, Ticketing, chống oversell | Test đồng thời 10k goroutine mua 100 vé → bán đúng 100 |
| **S2** — Phòng chờ ảo | Token, lottery, admit controller, SSE vị trí | k6: 100k VU join < 2s p99, admit rate ổn định |
| **S3** — Thanh toán & AI | Payment state machine, AI worker, chat trợ lý | Webhook idempotent; 5.000 câu hỏi/phút, 0 lỗi 429 tới user |
| **S4** — Anti-bot & Kiểm duyệt | Rule engine, scoring, AI moderation, anomaly | Bot script bị chặn ≥ 95%, false positive < 1% |
| **S5** — Chịu tải & Vận hành | K8s, HPA, load test, chaos drill, runbook | Load test 10x đạt SLO, chaos drill pass, runbook đã diễn tập |
| **S6** — Ổn định & Hoàn thiện | Báo cáo sau bán, phát hiện bất thường, hạng mục P2, sửa lỗi sau load test | Báo cáo AI tự sinh, không còn bug P0/P1 mở |

> Backlog 461 SP ở vận tốc ~70 SP/sprint cần 7 sprint. Chi tiết phân bổ: [04-jira-backlog.md](04-jira-backlog.md#kế-hoạch-sprint).

## 5. SLO mục tiêu

| Chỉ số | Mục tiêu |
|---|---|
| `POST /queue/join` p99 | < 300 ms ở 300k rps |
| `GET /queue/status` p99 | < 150 ms |
| Checkout (admitted) p99 | < 800 ms |
| Tỉ lệ oversell | **0** (hard requirement) |
| Uptime cửa sổ mở bán | 99.95% |
| AI phản hồi p95 | < 4 s (cache hit < 200 ms) |
| Cache hit ngữ nghĩa | > 80% |

## 6. Tài liệu liên quan

- [01-nghiep-vu.md](01-nghiep-vu.md) — Nghiệp vụ, actor, state machine, business rules
- [02-kien-truc.md](02-kien-truc.md) — Kiến trúc, scale, AI queue, anti-bot, dữ liệu
- [03-cau-truc-src.md](03-cau-truc-src.md) — Cấu trúc thư mục chuẩn
- [04-jira-backlog.md](04-jira-backlog.md) — Epic / Story / Task để import Jira
