# EventFlow — Cấu trúc source chuẩn

Monorepo, quản lý bằng `pnpm` workspace (TS) + Go workspace (`go.work`) + `uv` (Python).

## 1. Toàn cảnh

```
EventFlow/
├── apps/
│   └── web/                       # Next.js 15 (App Router) - buyer + organizer + ops console
├── services/
│   ├── gateway/                   # Go  - edge API
│   ├── identity/                  # Go
│   ├── event/                     # Go
│   ├── waitingroom/               # Go  - service nong nhat
│   ├── ticketing/                 # Go  - chong oversell
│   ├── payment/                   # Go
│   ├── antibot/                   # Go
│   ├── notification/              # Go
│   └── ai-worker/                 # Python FastAPI - consumer Gemini
├── libs/go/                       # module Go dung chung
├── packages/                      # package TS dung chung
├── proto/                         # hop dong gRPC / event schema
├── migrations/                    # SQL migration tap trung (goose)
├── deploy/
│   ├── docker/                    # Dockerfile tung service
│   ├── compose/                   # docker-compose cho local
│   ├── k8s/                       # manifest / kustomize / helm
│   └── scripts/                   # seed, pre-warm, vận hành
├── tests/
│   ├── load/                      # k6
│   └── e2e/                       # Playwright
├── docs/
├── go.work
├── pnpm-workspace.yaml
├── Makefile
└── README.md
```

## 2. Service Go — layout chuẩn (hexagonal-lite)

Mọi service Go dùng **cùng một** layout. Nhất quán quan trọng hơn tối ưu từng service.

```
services/ticketing/
├── cmd/
│   ├── server/main.go             # HTTP/gRPC server
│   └── worker/main.go             # sweeper + outbox relay
├── internal/
│   ├── config/                    # nap env, validate luc khoi dong
│   ├── domain/                    # THUAN TUY: entity, value object, loi nghiep vu
│   │   ├── order.go               #   state machine don hang
│   │   ├── inventory.go           #   bat bien chong oversell
│   │   ├── hold.go
│   │   └── errors.go
│   ├── app/                       # use case - dieu phoi domain, KHONG biet HTTP/SQL
│   │   ├── create_hold.go
│   │   ├── confirm_payment.go
│   │   ├── release_expired.go
│   │   └── issue_tickets.go
│   ├── port/                      # interface ma app can (DIP)
│   │   ├── repository.go
│   │   ├── inventory_gate.go
│   │   └── publisher.go
│   ├── adapter/                   # hien thuc port - cho duy nhat biet cong nghe
│   │   ├── http/                  #   handler, dto, middleware, router
│   │   ├── grpc/
│   │   ├── postgres/              #   repo (sqlc generated) + queries/*.sql
│   │   ├── redis/                 #   gate + script/*.lua
│   │   └── rabbitmq/              #   publisher, consumer
│   ├── worker/                    # hold_sweeper.go, outbox_relay.go, reconciler.go
│   └── observability/             # metric, trace, log rieng cua service
├── test/
│   ├── integration/               # testcontainers
│   └── concurrency/               # test 10k goroutine chong oversell
├── Dockerfile -> ../../deploy/docker/ticketing.Dockerfile
└── go.mod
```

**Quy tắc phụ thuộc (bắt buộc, enforce bằng lint):**

```
adapter ──> app ──> domain
   │                  ^
   └──> port ─────────┘        domain KHONG import bat cu thu gi ben ngoai
```

- `domain/` không import `database/sql`, `net/http`, redis, rabbitmq — nhờ vậy business rule test được không cần hạ tầng.
- `app/` chỉ nói chuyện qua `port/` interface.
- Chỉ `adapter/` biết công nghệ cụ thể. Đổi Redis → Valkey chỉ sửa một thư mục.

## 3. `libs/go` — dùng chung

```
libs/go/
├── httpx/          # router, middleware (auth, idempotency, ratelimit, recover), loi chuan RFC7807
├── redisx/         # client, helper nap va chay Lua script co SHA cache
├── mq/             # bao boc AMQP: publisher co outbox, consumer co retry/DLQ
├── authx/          # ky & xac minh JWT, queue token, QR HMAC
├── otelx/          # khoi tao trace + metric + log, propagate qua AMQP header
├── idem/           # idempotency store tren Postgres
├── outbox/         # transactional outbox pattern
├── leader/         # bau leader qua Redis lock co fencing token
└── testx/          # helper testcontainers
```

## 4. `services/waitingroom` — điểm nóng

```
services/waitingroom/
├── cmd/
│   ├── server/main.go
│   └── controller/main.go         # admit controller (leader-elected, 1 instance/su kien)
└── internal/
    ├── domain/
    │   ├── ticket.go              # queue token: ky, xac minh, han dung
    │   ├── lottery.go             # xao tron cong bang tai T0 (BR-Q1)
    │   └── position.go            # tinh rank, ETA, poll_after_ms thich nghi (BR-Q4)
    ├── app/
    │   ├── join_queue.go
    │   ├── get_status.go
    │   ├── admit_batch.go
    │   └── heartbeat.go
    └── adapter/
        ├── redis/script/
        │   ├── join.lua           # idempotent join, O(1)
        │   ├── admit.lua          # ZPOPMIN N -> admitted, atomic
        │   ├── shuffle.lua        # lottery tai T0
        │   └── status.lua         # ZRANK + doc trang thai mot vong
        ├── http/                  # REST + SSE stream
        └── metrics/
```

## 5. `services/ai-worker` (Python/FastAPI)

```
services/ai-worker/
├── app/
│   ├── main.py                    # FastAPI: health, metric, admin (dieu khien degradation)
│   ├── config.py
│   ├── consumers/                 # 1 consumer / 1 queue (BR-A3)
│   │   ├── chat.py
│   │   ├── moderation.py
│   │   ├── anomaly.py
│   │   └── report.py
│   ├── gemini/
│   │   ├── client.py              # wrapper co retry, timeout, dem token
│   │   ├── limiter.py             # global token-bucket RPM+TPM tren Redis (Lua)
│   │   ├── budget.py              # ngan sach token theo su kien (BR-A6)
│   │   └── schemas.py             # structured output pydantic
│   ├── pipeline/
│   │   ├── faq.py                 # L2 - intent + template, 0 token
│   │   ├── semantic_cache.py      # L3 - embedding cache tren Redis
│   │   ├── singleflight.py        # L4 - gop request trung
│   │   ├── degradation.py         # L7 - NORMAL/SAVING/FAQ_ONLY/OFF
│   │   └── tools.py               # tool-call lay so lieu that (BR-A5)
│   ├── prompts/                   # prompt versioned, tach khoi code
│   │   ├── assistant.v3.md
│   │   ├── moderation.v2.md
│   │   ├── anomaly.v1.md
│   │   └── report.v2.md
│   └── domain/
├── tests/
│   └── eval/                      # bo eval: do chat luong khi doi prompt/model
├── pyproject.toml
└── Dockerfile -> ../../deploy/docker/ai-worker.Dockerfile
```

> `prompts/` tách khỏi code và **đánh version**: đổi prompt là đổi hành vi hệ thống, phải review và eval như đổi code.

## 6. `apps/web` (Next.js)

```
apps/web/src/
├── app/
│   ├── (marketing)/               # trang tinh, ISR - phuc vu tu CDN
│   │   ├── page.tsx
│   │   └── events/[slug]/page.tsx
│   ├── (queue)/
│   │   └── waiting/[eventId]/     # phong cho: vi tri, ETA, chat AI
│   ├── (checkout)/
│   │   ├── select/[eventId]/      # chon ve (chi vao duoc khi ADMITTED)
│   │   ├── hold/[orderId]/        # dong ho dem nguoc 10:00
│   │   └── result/[orderId]/
│   ├── (organizer)/               # tao su kien, trang thai kiem duyet, doanh so
│   ├── (ops)/                     # war room: admit rate, phieu, AI lag
│   └── api/                       # BFF mong: proxy, che secret, SSE relay
├── components/
│   ├── queue/                     # QueuePosition, ProgressRing, AdmitBanner
│   ├── checkout/                  # SeatPicker, HoldCountdown, PaymentForm
│   ├── assistant/                 # ChatPanel, StreamingMessage, FaqChips
│   └── ui/
├── lib/
│   ├── api/                       # client sinh tu OpenAPI
│   ├── sse.ts                     # SSE co reconnect + backoff
│   ├── queue-client.ts            # polling thich nghi theo poll_after_ms cua server
│   ├── jitter.ts                  # hoan ngau nhien 0..5s tai T0 (Tang 2)
│   └── fingerprint.ts             # tin hieu thiet bi cho anti-bot
├── hooks/
│   ├── use-queue-status.ts
│   ├── use-hold-timer.ts          # dem nguoc theo gio SERVER (BR-O2)
│   └── use-assistant.ts
└── styles/
```

## 7. Quy ước chung

| Hạng mục | Quy ước |
|---|---|
| Nhánh Git | `main` (bảo vệ) ← `develop` ← `feat/EVF-123-mo-ta` |
| Commit | Conventional Commits, kèm mã Jira: `feat(ticketing): EVF-123 them redis hold gate` |
| API | REST + OpenAPI 3.1 ra ngoài; gRPC giữa các service; lỗi theo RFC 7807 |
| Event MQ | `<domain>.<entity>.<action>` — vd `ticketing.hold.released`; payload có `event_id`, `occurred_at`, `trace_id`, `schema_version` |
| Migration | `goose`, chỉ tiến (forward-only) trên production; đặt tên `NNNN_mo_ta.sql` |
| Cấu hình | 12-factor, toàn bộ qua env; validate lúc khởi động, sai thì **fail fast** |
| Redis key | `<domain>:{<shard-tag>}:<entity>:<id>` — hash tag để cùng sự kiện nằm cùng shard |
| Test | Go `_test.go` cạnh file nguồn; integration đặt ở `test/`; coverage tối thiểu 70% cho `domain/` và `app/` |
