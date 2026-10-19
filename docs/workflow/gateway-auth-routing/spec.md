# EV-NEW-1 — Gateway thật: định tuyến, xác thực JWT, gắn danh tính, rate limit, chặn `/internal/*` (Spec)

| Mục | Giá trị |
|---|---|
| Jira | **Chưa có key** — EV-NEW-1 trong kế hoạch, 8 SP, Sprint A việc #2. Khi tạo issue, đổi tên nhánh/PR theo key thật |
| Kế hoạch tổng | [10-ke-hoach-hoan-thien-du-an.md](../../10-ke-hoach-hoan-thien-du-an.md) — làn bảo mật #1 → **#2** → #3 → #9 |
| Branch | `feature/gateway-auth-routing`, chồng trên `feature/EV-112-refresh-token-rotation` (cần `authx.VerifyAccessToken`) |
| Kiến trúc | [02-kien-truc.md](../../02-kien-truc.md) — gateway: authN, rate-limit, định tuyến, stateless |

## 1. Bài toán

`services/gateway` chỉ có `/healthz`. Web và k6 không có cửa vào chung; waitingroom/ticketing tin header
`X-Identity-Id` do **client** gửi, nên ai cũng giả danh được người khác. API nội bộ
(`PATCH /internal/v1/events/{id}/ai-verdict`, `POST /internal/reconcile`) và `/metrics` không có lớp nào chặn.

## 2. Thiết kế

Một bảng route duy nhất (`internal/proxy/routes.go`), mỗi route: pattern `http.ServeMux`, upstream, mức bảo vệ
(`Public` / `Authenticated` / `Blocked`), role bắt buộc (nếu có), lớp rate limit. Mỗi request đi qua:

1. **Xoá danh tính client tự khai** (`X-Identity-Id`, `X-Identity-Role`, `X-User-Id`, `X-Role`, `X-Organizer-Id`) —
   trên **mọi** route, kể cả công khai. `X-Forwarded-*` client gửi bị `httputil.ReverseProxy.Rewrite` xoá rồi gán lại.
2. **Xác thực** access token bằng `authx.VerifyAccessToken` (EV-112). Route `Authenticated` thiếu/sai token → 401
   `/problems/unauthorized`; hết hạn → 401 `/problems/token-expired` (client biết gọi `/auth/refresh`). Route công
   khai: token hợp lệ vẫn được gắn danh tính, token hỏng coi như khách.
3. **Role thô** cho route chỉ một role dùng: `/api/v1/moderation/*` = moderator; `/v1/payments/inbox/*`,
   `/v1/payments/reconcile` = ops → 403. Quyền chi tiết (chủ sự kiện, trạng thái) vẫn do service kiểm (EV-114).
4. **Rate limit** token bucket trong bộ nhớ, khoá theo identity (đã đăng nhập) hoặc IP. Ba lớp, chỉnh bằng env
   `RATE_<LỚP>=rps:burst`:

   | Lớp | Mặc định | Khoá | Mục đích |
   |---|---|---|---|
   | `auth` | 1 rps, burst 10 | IP (kể cả có token) | Chống dò mật khẩu/OTP |
   | `queue_poll` | 0,5 rps, burst 3 | identity + event | `GET …/queue/status` không dày hơn `poll_after_ms` (BR-Q4) |
   | `default` | 10 rps, burst 30 | identity hoặc IP | Còn lại |

   Vượt → 429 `/problems/rate-limited` + `Retry-After`.
5. **Gắn danh tính đã xác minh** (`authx.SetIdentityHeaders`), `X-Request-Id`, giới hạn body 1 MiB (413), chuyển
   tiếp. SSE (`queue/stream`, `vneid/sse`) flush ngay (`FlushInterval: -1`); không đặt `WriteTimeout`.

`/internal/*`, `/metrics`, `/admin/*` và mọi đường dẫn không có trong bảng → 404 (không lộ route tồn tại). CORS
xử lý tại gateway theo `CORS_ALLOWED_ORIGINS`; header `Access-Control-*` từ service (identity đặt `*`) bị xoá để
không thành hai giá trị. Upstream sập → 502 `/problems/upstream-unavailable`.

Hợp đồng header nằm ở `libs/go/authx/principal.go` (`PrincipalFromHeaders`) để service phía sau đọc cùng một cách.

## 3. Bảng route

| Pattern | Service | Bảo vệ |
|---|---|---|
| `/api/v1/auth/` | identity | Công khai (lớp `auth`); `/me`, `/logout-all` do identity tự xác minh |
| `GET /api/v1/events/{id}`, `GET /api/v1/events/slug/{slug}` | event | Công khai |
| `/api/v1/events`, `/api/v1/events/` | event | Đăng nhập |
| `/api/v1/moderation/` | event | moderator |
| `GET /v1/events/{eventID}/queue/status` | waitingroom | Đăng nhập, lớp `queue_poll` |
| `/v1/events/{eventID}/queue/` | waitingroom | Đăng nhập |
| `/v1/events/{eventID}/holds`, `…/purchase-limit`, `/v1/orders/`, `/v1/tickets/` | ticketing | Đăng nhập |
| `/v1/events/{eventID}/payments`, `/v1/payments/{paymentID}` | payment | Đăng nhập |
| `/v1/payments/webhook/`, `GET /v1/payments/methods` | payment | Công khai (payment tự xác minh chữ ký webhook) |
| `/v1/payments/inbox/`, `/v1/payments/reconcile` | payment | ops |
| `/v1/events/{eventID}/assistant/`, `/v1/assistant/` | ai-worker | Công khai |
| `/internal/`, `/metrics`, `/admin/` | — | Chặn (404) |

## 4. Tiêu chí nghiệm thu

| AC | Nội dung |
|---|---|
| AC-1 | Header danh tính client gửi không bao giờ tới service; route cần đăng nhập nhận đúng một `X-Identity-Id` lấy từ token |
| AC-2 | Route cần đăng nhập: thiếu/sai secret/rác/hết hạn/role lạ/`sub` không phải UUID → 401, service không bị chạm |
| AC-3 | `/internal/*`, `/metrics`, `/admin/*`, đường dẫn lạ → 404 kể cả với token ops |
| AC-4 | Route chỉ dành cho một role trả 403 với role khác |
| AC-5 | Mọi endpoint web đang gọi tới đúng service, giữ nguyên path |
| AC-6 | Rate limit 429 + `Retry-After`; poll phòng chờ đếm theo người + sự kiện; lớp auth theo IP không lách bằng token |
| AC-7 | CORS: preflight 204 tại gateway, đúng một `Allow-Origin`, origin lạ không nhận header cho phép |
| AC-8 | SSE được đẩy ngay; upstream sập → 502; body > 1 MiB → 413 |

## 5. Giới hạn và việc để lại

- **Service phía sau không được mở cổng ra ngoài.** Compose hiện publish `8081`, `8082` ra host: ai gọi thẳng vẫn giả
  danh được. EV-104 phải bỏ `ports:` của service (chỉ gateway publish 8080) và k8s chỉ expose gateway.
- **Rate limit theo replica** (bộ nhớ). N replica → giới hạn thực ×N. Giới hạn chính xác theo người/sự kiện thuộc
  antibot (EV-174). IP lấy từ kết nối: đặt sau ingress/LB thì phải thêm cấu hình proxy tin cậy.
- **Access token đã cấp vẫn sống tới 15 phút sau logout** (đánh đổi của EV-112).
- **Web:** `NEXT_PUBLIC_API_URL` (auth) đang trỏ thẳng identity `:8083/api/v1`; đổi sang `http://localhost:8080/api/v1`
  khi chạy qua gateway. `hold-client.ts` vẫn gửi `X-Identity-Id` — vô hại (bị xoá), nên chuyển sang gửi
  `Authorization`. Dockerfile + service compose cho gateway thuộc EV-104.
- **Quan sát:** chưa gắn `otelx` (EV-103).
