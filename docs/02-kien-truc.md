# EventFlow — Kiến trúc

## 1. Sơ đồ tổng thể

```
                         Nguoi dung (web / mobile)
                                   |
                      [ CDN + WAF + Turnstile + Bot rules ]        <- hap thu 300k rps tinh
                                   |
                        [ Ingress / API Gateway (Go) ]             <- authN, rate-limit, load-shed
                                   |
   +---------------+---------------+---------------+---------------+----------------+
   |               |               |               |               |                |
identity-svc   event-svc    waitingroom-svc   ticketing-svc   payment-svc     antibot-svc
   (Go)          (Go)            (Go)             (Go)           (Go)            (Go)
   |               |               |               |               |                |
   +-------+-------+-------+-------+-------+-------+-------+-------+-------+--------+
           |               |               |               |               |
      [ PostgreSQL ]  [ Redis Cluster ] [ RabbitMQ ]  [ Object store ]  [ ClickHouse/Mongo ]
       nguon su that   hang cho + kho     job bat dong bo   anh/QR          log & analytics
                                             |
                                    +--------+--------+
                                    |                 |
                            ai-worker (FastAPI)   notification-svc (Go)
                                    |
                             [ Gemini API ]  <- qua global token-bucket limiter
```

### Phân chia trách nhiệm service

| Service | Ngôn ngữ | Trách nhiệm | Trạng thái |
|---|---|---|---|
| `gateway` | Go | TLS, authN, rate-limit theo IP/identity, load shedding, định tuyến | stateless |
| `identity` | Go | Đăng ký/đăng nhập, OTP, JWT, device fingerprint | Postgres |
| `event` | Go | CRUD sự kiện, hạng vé, lịch mở bán, kích hoạt kiểm duyệt | Postgres |
| `waitingroom` | Go | Token hàng chờ, lottery, rank, admit controller, SSE | **Redis** (nóng nhất) |
| `ticketing` | Go | Tồn kho, hold, order, phát vé, sweeper, outbox | Redis + Postgres |
| `payment` | Go | Adapter cổng thanh toán, webhook, đối soát, hoàn tiền | Postgres |
| `antibot` | Go | Rule engine, scoring, quyết định chặn/làn chậm | Redis |
| `notification` | Go | Email/SMS/push, fan-out SSE | RabbitMQ |
| `ai-worker` | Python/FastAPI | Consumer Gemini: chat, moderation, anomaly, report | RabbitMQ + Redis |

> **Vì sao Go cho lõi, Python cho AI:** đường đi nóng (join queue, hold vé) cần
> p99 ổn định ở hàng trăm nghìn kết nối đồng thời — goroutine + GC thấp của Go
> hợp hơn hẳn. Còn AI worker bị chặn bởi rate-limit của Gemini chứ không phải CPU,
> nên Python đổi lấy tốc độ phát triển và hệ sinh thái (SDK, embedding, eval).

---

## 2. Câu hỏi đánh giá #1 — Scale khi request tăng 10× đúng giây mở bán

Chiến lược gồm **6 tầng phòng thủ**, đi từ ngoài vào trong:

### Tầng 1 — Không cho tải chạm backend

- Trang sự kiện render tĩnh (Next.js ISR), phục vụ 100% từ CDN. Số vé còn lại hiển thị dạng **khoảng** ("còn nhiều / sắp hết"), lấy từ snapshot cache 5s — không query DB.
- Đồng hồ đếm ngược đồng bộ qua `GET /time` (edge, cache 1s), không phải bằng giờ máy khách.
- Tài nguyên tĩnh có `Cache-Control: immutable`, hash trong tên file.

### Tầng 2 — Làm phẳng đỉnh nhọn (jitter + lottery)

Đỉnh thật sự nằm ở **đúng giây 20:00:00**. Hai kỹ thuật:

- Client tự hoãn gọi `join` một khoảng ngẫu nhiên `0..5s` sau T0.
- Vì **BR-Q1** dùng lottery, hoãn không hề gây thiệt — nên không ai có động cơ "lách".

→ Đỉnh 300k rps/giây bị trải thành ~60k rps trong 5 giây.

### Tầng 3 — Đường đi nóng O(1) trên Redis

`POST /queue/join` chỉ thực hiện một Lua script atomic:

```
GET  wr:{ev}:token:{identity}   -- da co? tra ve token cu (idempotent, BR-Q3)
INCR wr:{ev}:seq
ZADD wr:{ev}:lobby  <score>  <token>
SET  wr:{ev}:token:{identity} <token> EX 86400
```

- Không chạm Postgres. Không ghi đĩa trên đường nóng.
- Shard Redis **theo `event_id`** (hash tag `{ev}`) → mỗi sự kiện lớn có shard riêng, không hàng xóm ồn ào.
- Một node Redis xử lý được ~100k ops/s; 6 shard = 600k ops/s, thừa cho 10×.

### Tầng 4 — Đọc trạng thái rẻ hơn ghi

`GET /queue/status` là request chiếm **90% lưu lượng** trong 30 phút chờ:

- Rank đọc từ `ZRANK` O(log N), kèm cache theo *bucket* (làm tròn rank tới bội số 100) → cache hit cao, tiết kiệm CPU.
- Server trả `poll_after_ms` thích nghi (**BR-Q4**): rank > 10.000 → 30s; < 500 → 3s. Giảm lưu lượng polling tới **10×**.
- Hỗ trợ SSE cho nhóm gần lượt; nhóm ở xa vẫn polling (SSE tốn kết nối, chỉ dùng nơi đáng).
- `ETag` + `304 Not Modified` cho trạng thái không đổi.

### Tầng 5 — Admit controller: biến spike thành dòng chảy hằng số

Đây là **trái tim** của giải pháp scale.

```
                     do luong thuc te moi 1s
    checkout_throughput, p99_latency, db_pool_usage, error_rate
                             |
                             v
              +------------------------------+
              |  AIMD controller (leader)     |
              |  on  : rate += step           |   tang tuyen tinh khi khoe
              |  tai : rate *= 0.7            |   giam nhan khi co suc ep
              +------------------------------+
                             |
                       admit_rate (v/d)
                             |
        moi tick: ZPOPMIN N tu wr:{ev}:queue  ->  wr:{ev}:admitted (TTL 15')
                             |
                       publish SSE "YOUR_TURN"
```

Nhờ vậy **tải xuống Postgres bị ghim ở mức hằng số** (~200 rps) bất kể có 30k hay 3 triệu người xếp hàng. Đây là lý do hệ thống không cần scale DB theo lượng khách.

Chọn leader qua Redis lock (`SET NX PX` + fencing token) để chỉ một pod chạy controller mỗi sự kiện.

### Tầng 6 — Chuẩn bị hạ tầng & xả tải có kiểm soát

| Biện pháp | Chi tiết |
|---|---|
| **Pre-warm theo lịch** | CronJob scale deployment lên mức đỉnh **T0 − 30 phút**; HPA chỉ làm nhiệm vụ thu nhỏ sau đó. Tuyệt đối không dựa vào autoscale tại T0 — pod khởi động mất 30–60s, quá muộn. |
| **HPA custom metric** | Scale theo `queue_depth` và `rps`, không theo CPU (CPU tăng trễ). |
| **PgBouncer** | Transaction pooling; app 2000 kết nối → Postgres 100 kết nối thật. |
| **Read replica** | Mọi truy vấn đọc (danh sách sự kiện, lịch sử đơn) đi replica. |
| **Load shedding** | Khi quá tải: giữ `queue/status` và `payment/webhook` (quan trọng nhất), trả `503 + Retry-After` cho endpoint phụ. Ưu tiên theo lớp request, không cắt bừa. |
| **Circuit breaker** | Giữa các service; hỏng phần nào suy biến phần đó, không sập dây chuyền. |
| **Quorum queue** | RabbitMQ quorum + 3 node, chịu mất 1 node. |
| **Multi-AZ** | Pod anti-affinity theo zone; Redis có replica mỗi shard. |

### Chống oversell dưới tải cao — chi tiết

Vấn đề kinh điển: hàng nghìn transaction cùng `UPDATE` một dòng tồn kho → row lock contention, p99 nổ.

**Giải pháp 2 lớp + phân mảnh kho:**

```
1. REDIS GATE (Lua, atomic, ~0.2ms)
   if available >= qty then
        available -= qty
        HSET hold:{id} ...          EXPIRE 600
        ZADD holds:{ev} <expire_at> {id}
        return OK
   else return SOLD_OUT            -- chan 95% request thua ngay tai day

2. POSTGRES AUTHORITATIVE (trong transaction cua don hang)
   UPDATE ticket_inventory
      SET available = available - $qty
    WHERE ticket_type_id = $1 AND bucket = $2 AND available >= $qty
   -- CHECK (available >= 0) la chot chan cuoi cung o tang schema

3. PHAN MANH KHO (bucket)
   Quota 20.000 ve chia thanh 32 bucket x 625 ve.
   Client bam ngau nhien 1 bucket -> contention giam 32 lan.
   Bucket het -> thu bucket ke tiep (vong tron, toi da 3 lan) -> moi bao SOLD_OUT.
```

**Bất biến:** Redis có thể lệch (mất dữ liệu, failover) nhưng **không bao giờ** gây oversell, vì Postgres mới là nơi chốt. Redis lệch chỉ dẫn tới "báo hết sớm", và job `inventory-reconciler` chạy mỗi 10s sẽ đồng bộ lại Redis từ Postgres.

**Nhả giữ ghế (BR-O2, BR-O7):**

```
sweeper (moi 1s):
  ZRANGEBYSCORE holds:{ev} -inf <now>   -> danh sach hold het han
  voi moi hold:
     BEGIN
       UPDATE seat_holds SET released_at = now()
        WHERE id = $1 AND released_at IS NULL     -- idempotent, tra 0 row neu da release
       (neu 1 row) UPDATE ticket_inventory SET available = available + qty ...
       UPDATE orders SET status = 'EXPIRED' WHERE ...
     COMMIT
     -> phat outbox `hold.released` -> Redis tra kho -> thong bao hang cho
```

Không dùng Redis keyspace notification (không đảm bảo giao nhận). Sweeper quét ZSET là nguồn duy nhất, có thể chạy lại an toàn.

---

## 3. Câu hỏi đánh giá #2 — Rate-limit của AI API và vai trò của Queue

Bối cảnh: 30.000 người trong phòng chờ, ~5.000 câu hỏi/phút. Gemini cho phép ví dụ **60 RPM / 1M TPM**. Chênh lệch ~80×. Queue một mình **không** giải quyết được — nó chỉ hoãn vấn đề. Cần một chuỗi 7 lớp:

```
5.000 cau/phut
   |
   v  [L1] Chan dau vao: quota 10 cau/15 phut moi khach (BR-A2)
4.000
   |
   v  [L2] Rule/FAQ khop chinh xac (khong ton token)          -40%
2.400
   |
   v  [L3] Cache ngu nghia (embedding + nguong 0.93)          -45%
1.320
   |
   v  [L4] Singleflight: gop cac cau y het dang in-flight     -30%
  920
   |
   v  [L5] RabbitMQ: dem va dieu tiet (san bang tai)
   |
   v  [L6] Global token-bucket limiter (Redis Lua): RPM + TPM
   |         -> chi ~60 call/phut thuc su cham Gemini
   v  [L7] Retry backoff qua delay-queue; kiet suc -> tra loi du phong
[ Gemini API ]  ~60 RPM  (an toan)
```

### Chi tiết từng lớp

**L2 — Rule/FAQ (0 token).** Phần lớn câu hỏi trong phòng chờ lặp lại: "còn bao lâu tới lượt tôi", "còn vé hạng A không", "mất chỗ nếu tắt trình duyệt không". Bộ intent classifier nhẹ (từ khoá + embedding cục bộ) khớp vào ~50 mẫu, trả lời bằng template và **số liệu thật lấy từ hệ thống** (BR-A5).

**L3 — Cache ngữ nghĩa.** Chuẩn hoá câu hỏi → embedding → tìm láng giềng gần nhất trong Redis vector. `cosine ≥ 0.93` → dùng lại câu trả lời cũ. Key cache gồm `event_id` để tránh lẫn sự kiện. TTL 15 phút với câu hỏi động, 24h với câu hỏi tĩnh.

**L4 — Singleflight.** `SET NX` trên hash của câu hỏi; ai vào sau thì subscribe kết quả của call đang chạy thay vì tạo call mới.

**L5 — RabbitMQ.** 4 hàng đợi tách biệt (BR-A3) để job nặng không chèn ép chat:

| Queue | Ưu tiên | TTL msg | Consumer | Model |
|---|---|---|---|---|
| `ai.chat.q` | cao | 60s | 20 | Gemini Flash |
| `ai.moderation.q` | trung | 10 phút | 5 | Gemini Flash |
| `ai.anomaly.q` | thấp | 30 phút | 2 | Gemini Flash |
| `ai.report.q` | thấp nhất | 2 giờ | 1 | Gemini Pro |

Topology: `ai.direct` exchange → 4 queue; mỗi queue có `x-dead-letter-exchange` trỏ `ai.retry` với các delay-queue 5s / 30s / 2m; hết 3 lần → `ai.dlq`.

Message `ai.chat.q` quá 60s bị TTL loại bỏ — **cố ý**: câu trả lời tới sau khi khách đã được admit là vô giá trị, giữ lại chỉ tốn quota.

**L6 — Global token-bucket limiter (mấu chốt).** Mọi worker replica dùng chung bucket trên Redis, tính **cả hai chiều** RPM và TPM:

```lua
-- ai_limiter.lua : refill theo thoi gian troi, kiem ca request va token
local now, rpm_cap, tpm_cap, est_tokens = ...
-- refill
-- neu du ca 2 bucket -> tru va tra {1, 0}
-- neu thieu -> tra {0, wait_ms}   (worker nack + requeue sau wait_ms)
```

Worker **ước lượng token trước khi gọi** (đếm token prompt + `max_output_tokens`) và đặt chỗ trước; sau khi có usage thật thì hoàn lại phần chênh. Nhờ vậy hệ thống **không bao giờ chủ động gây 429** thay vì đợi bị 429 rồi mới lùi.

**L7 — Suy biến có tầng (degradation).** Ops và hệ thống có 4 nấc:

| Nấc | Kích hoạt | Hành vi |
|---|---|---|
| NORMAL | mặc định | Đầy đủ LLM |
| SAVING | queue lag > 30s **hoặc** ngân sách > 80% | Tắt câu hỏi mở, chỉ FAQ + cache, rút `max_output_tokens` |
| FAQ_ONLY | queue lag > 2 phút **hoặc** ngân sách cạn | 100% template, không gọi Gemini |
| OFF | Gemini sự cố / circuit open | Ẩn trợ lý, hiện kênh CSKH |

**Khách không bao giờ thấy lỗi 429.** Tệ nhất họ nhận câu trả lời FAQ — vẫn hữu ích.

### Bốn nhiệm vụ AI

| Nhiệm vụ | Kích hoạt | Đặc thù thiết kế |
|---|---|---|
| **Trợ lý phòng chờ** | Khách hỏi | Độ trễ quan trọng → Flash + cache + tool-call lấy số liệu thật |
| **Kiểm duyệt sự kiện** | Organizer submit | Gộp lô nhiều trường vào 1 prompt; đầu ra **structured JSON** có schema; điểm ≥0.8 chặn, 0.3–0.8 đẩy người (BR-E4) |
| **Phát hiện bất thường** | Cron 30s trong cửa sổ bán | Không đưa log thô cho LLM. Pipeline gom cụm bằng thống kê trước, chỉ đưa **đặc trưng cụm đã tóm tắt** → LLM diễn giải và đề xuất rule (BR-B2) |
| **Báo cáo sau bán** | Sự kiện đóng | Số liệu do SQL/analytics tính chính xác; LLM chỉ **diễn giải và viết**, không tự tính toán số |

> Nguyên tắc xuyên suốt: **LLM diễn giải, hệ thống tính toán.** Mọi con số trong đầu ra AI đều đến từ truy vấn thật, không từ trí nhớ mô hình.

---

## 4. Mô hình dữ liệu (PostgreSQL)

```
identities ──< devices
     │
     └──< queue_sessions (mirror nguoi dung tu Redis, de audit)

organizers ──< events ──< ticket_types ──< ticket_inventory (32 bucket/hang ve)
                  │             │
                  │             └──< seat_holds ──> orders
                  │
                  ├──< moderation_reviews   (ket qua AI + nguoi duyet)
                  └──< sale_reports          (bao cao sau ban)

orders ──< order_items
   │
   ├──< payments  (UNIQUE provider_txn_id)
   └──< tickets   (QR, jti, trang thai check-in)

outbox_events        -- transactional outbox -> RabbitMQ
idempotency_keys     -- (key, endpoint, response, expires_at)
payment_inbox        -- webhook den som, cho replay
audit_logs           -- moi hanh dong nhay cam, gom ca quyet dinh AI
bot_decisions        -- diem rui ro + rule kich hoat (giai thich duoc, BR-B1)
```

**Phân chia lưu trữ:**

| Loại dữ liệu | Nơi lưu | Lý do |
|---|---|---|
| Tồn kho, đơn, thanh toán | PostgreSQL | Cần ACID tuyệt đối |
| Hàng chờ, rank, hold, rate-limit, cache | Redis | O(1), tần suất cực cao, chịu mất được (có nguồn sự thật ở PG) |
| Chat history, log hành vi thô | MongoDB / ClickHouse | Ghi nhiều, schema lỏng, truy vấn phân tích |
| Ảnh sự kiện, file QR, báo cáo PDF | Object storage (S3/MinIO) | |

---

## 5. Bảo mật

- JWT ngắn hạn (15') + refresh token xoay vòng; queue token ký riêng, gắn `identity_id` + `device_id` + `event_id`.
- QR vé ký HMAC-SHA256 kèm `jti` + hạn dùng; check-in xác minh online, chống chụp màn hình chia sẻ.
- Webhook thanh toán xác minh chữ ký + chống replay bằng timestamp window.
- Secret quản lý bằng External Secrets / Vault, không nằm trong image.
- Rate-limit nhiều chiều: IP, identity, device, ASN.
- PII mã hoá ở tầng cột (số điện thoại, email); log **không** ghi PII thô.
- Chống prompt injection ở AI worker (BR-A7) + lọc đầu ra trước khi trả khách.

## 6. Quan trắc (Observability)

**Metric nghiệp vụ (quan trọng hơn metric hệ thống):**

| Metric | Cảnh báo khi |
|---|---|
| `waitingroom_queue_depth{event}` | — (dashboard) |
| `waitingroom_admit_rate{event}` | = 0 trong > 60s dù queue còn người |
| `ticketing_oversell_guard_rejections` | **> 0 → cảnh báo nghiêm trọng** (bất biến bị đe doạ) |
| `ticketing_hold_conversion_ratio` | < 40% |
| `ticketing_inventory_drift{ticket_type}` | ≠ 0 kéo dài > 30s |
| `ai_queue_lag_seconds{queue}` | > 30s |
| `ai_gemini_429_total` | > 0 (chứng tỏ limiter sai) |
| `ai_semantic_cache_hit_ratio` | < 70% |
| `payment_webhook_lag_seconds` | p99 > 30s |

- Trace: OpenTelemetry xuyên suốt, `trace_id` chảy qua cả RabbitMQ message header.
- Log: JSON có cấu trúc, đính `event_id` / `identity_id` (đã hash) / `trace_id`.
- Dashboard "War room" một màn hình: phễu chuyển đổi realtime + admit rate + tồn kho + AI lag.

## 7. Triển khai

| Môi trường | Hình thức |
|---|---|
| Local | `docker compose up` — đủ cả Postgres, Redis, RabbitMQ, mọi service, Gemini có thể chạy chế độ mock |
| Staging | K8s 1 node pool, dữ liệu giả, chạy load test |
| Production | K8s multi-AZ, Redis Cluster, Postgres HA + PgBouncer, RabbitMQ quorum 3 node |

- Image phân tầng, distroless, non-root, có health/readiness probe riêng biệt.
- Rollout: rolling update + PDB; sự kiện lớn thì **đóng băng deploy** từ T0 − 2h tới T0 + 2h.
- CI: lint → unit → integration (testcontainers) → build → scan (Trivy) → deploy staging → load test → cổng duyệt tay → production.

## 8. Chiến lược kiểm thử

| Loại | Trọng tâm |
|---|---|
| Unit | Business rule, state machine, tính tiền |
| Integration | testcontainers: Postgres + Redis + RabbitMQ thật |
| **Concurrency** | 10.000 goroutine cùng mua 100 vé → **đúng 100 đơn thành công, 0 oversell**. Đây là test không được phép fail. |
| Contract | Pact giữa web ↔ gateway ↔ service |
| Load (k6) | 100k VU join trong 5s; 30 phút chờ; đợt checkout |
| Chaos | Giết Redis primary giữa đợt bán; Gemini trả 429 100%; Postgres failover; RabbitMQ mất 1 node |
| E2E | Playwright: xếp hàng → được gọi → giữ ghế → thanh toán → nhận vé |
