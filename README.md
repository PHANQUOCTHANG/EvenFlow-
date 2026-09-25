# EventFlow

Nền tảng bán vé sự kiện chịu tải đột biến, có **Phòng chờ ảo** và **AI hỗ trợ vận hành**.

> 20:00:00 mở bán concert. ~300.000 người bấm cùng lúc để giành ~20.000 vé.
> Hệ thống phải công bằng, không bán quá một vé nào, không sập, chặn bot,
> và trả lời hàng nghìn câu hỏi trong lúc khách chờ.

## Ý tưởng cốt lõi

Phòng chờ ảo không phải là tính năng UX — nó là **bộ điều tiết tải** biến một
đỉnh tải vô hạn thành dòng chảy hằng số:

| Tầng | Chịu tải | Vì sao chịu được |
|---|---|---|
| CDN / Edge | 300k rps | Tĩnh 100%, không chạm backend |
| Waiting Room (Redis) | 300k rps | O(1), chỉ `INCR` + `ZADD`, shard theo event |
| Ticketing / Postgres | **~200 rps cố định** | Bị ghim bởi `admit_rate` — **không phụ thuộc** số người xếp hàng |
| AI (Gemini) | ~60 rpm | Limiter toàn cục + cache ngữ nghĩa hấp thụ 85% câu hỏi |

Dù 300k hay 3 triệu người vào, Postgres vẫn chỉ thấy ngần ấy request.

## Bắt đầu

```bash
cp .env.example .env
make up          # toan bo ha tang + service
make migrate
make seed        # 1 su kien ON_SALE, 20.000 ve
make smoke       # chay tron mot luong mua ve
```

| Dịch vụ | URL |
|---|---|
| Web | http://localhost:3000 |
| RabbitMQ | http://localhost:15672 (`eventflow` / `eventflow`) |
| Jaeger | http://localhost:16686 |
| Grafana | http://localhost:3001 |

Dev không cần API key Gemini: `GEMINI_MOCK=1` (mặc định) dùng client giả lập.

## Tài liệu

| Tài liệu | Nội dung |
|---|---|
| [docs/00-tong-quan.md](docs/00-tong-quan.md) | Kế hoạch, SLO, lộ trình 7 sprint |
| [docs/01-nghiep-vu.md](docs/01-nghiep-vu.md) | Actor, state machine, 30+ business rule |
| [docs/02-kien-truc.md](docs/02-kien-truc.md) | Chiến lược scale 10×, xử lý rate-limit AI, anti-bot |
| [docs/03-cau-truc-src.md](docs/03-cau-truc-src.md) | Cấu trúc thư mục, quy ước |
| [docs/04-jira-backlog.md](docs/04-jira-backlog.md) | 10 epic, 89 story/task, 461 SP — kèm `docs/jira-import.csv` |

## Cấu trúc

```
apps/web/            Next.js 15 (buyer + organizer + ops)
services/
  waitingroom/       Go -- lottery, rank, admit controller   [nong nhat]
  ticketing/         Go -- ton kho, hold, chong oversell     [rui ro nhat]
  ai-worker/         Python -- consumer Gemini qua RabbitMQ
  gateway/ identity/ event/ payment/ antibot/ notification/
libs/go/             thu vien Go dung chung
migrations/          schema PostgreSQL (goose)
deploy/              docker, compose, k8s, observability
tests/load/          k6
```

## Ba đoạn code đáng đọc trước

1. [`waitingroom/.../script/shuffle.lua`](services/waitingroom/internal/adapter/redis/script/shuffle.lua)
   — lottery tại T0. Đây là lý do bot bấm sớm không có lợi thế, và cũng là lý do
   client có thể jitter 0–5s mà không thiệt gì.
2. [`waitingroom/internal/app/admit_controller.go`](services/waitingroom/internal/app/admit_controller.go)
   — AIMD controller. Đây là chỗ spike biến thành dòng chảy hằng số.
3. [`ai-worker/app/scripts/limiter.lua`](services/ai-worker/app/scripts/limiter.lua)
   — token bucket toàn cục RPM+TPM. Hệ thống **không bao giờ chủ động** chạm trần
   Gemini, thay vì đợi bị 429 rồi mới lùi.

## Bất biến không được phép vi phạm

```bash
make test-oversell   # 10.000 goroutine mua 100 ve, chay 200 lan
```

Test này fail → **không merge, không release**. Bán quá vé là lỗi không sửa được
bằng hotfix: vé đã bán, khách đã đến sân, và chỗ ngồi thì không có.

Bốn lớp bảo vệ, độc lập nhau:

| Lớp | Cơ chế |
|---|---|
| Redis Lua | Giảm tồn atomic, chặn 95% request thừa trước khi chạm DB |
| Postgres | `UPDATE ... WHERE available >= qty` trong transaction |
| Schema | `CHECK (available >= 0)` — chốt chặn cuối cùng |
| Đối soát | Job so Redis ↔ Postgres mỗi 10s, sửa lệch tự động |

Redis lệch số **không thể** gây oversell — chỉ có thể gây "báo hết vé sớm".

## Trạng thái hiện tại

Đây là **scaffold theo kế hoạch trong `docs/`**, không phải hệ thống đã chạy production.

| Đã có | Còn phải làm |
|---|---|
| Schema đầy đủ + mọi bất biến chống oversell | Các service `gateway`, `identity`, `event`, `payment`, `antibot`, `notification` mới có thư mục |
| 4 Lua script phòng chờ + 2 script ticketing | `apps/web` mới có khung |
| Admit controller AIMD | Helper testcontainers cho EVF-39 |
| Postgres repo (hold + release idempotent) | K8s manifest |
| Hold sweeper | Gemini client thật, FAQ matcher, semantic cache |
| Limiter Gemini + suy biến 4 nấc | |
| Compose, Dockerfile, RabbitMQ topology, k6, alert | |

Backlog đầy đủ với thứ tự ưu tiên: [docs/04-jira-backlog.md](docs/04-jira-backlog.md).
