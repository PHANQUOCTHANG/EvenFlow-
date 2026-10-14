# EV-198 — E2E Playwright luồng mua vé đầy đủ (Spec)

| Mục | Giá trị |
|---|---|
| Jira | EV-198 (mã backlog **EVF-128**, issue 1909) — 3 SP — High — Sprint 5 |
| Branch | `feature/EV-198-e2e-playwright` (từ `develop` @ `51c4729`) |
| Tier | **Standard** (10 bước). Có sửa CI: thêm workflow mới, không sửa job/gate hiện có. |
| Quyết định của người dùng (2026-10-13) | **E2E giả lập API**: chạy web thật, giả lập backend bằng `page.route`. |

## 1. Bối cảnh

Jira yêu cầu: "xếp hàng → được gọi → giữ ghế → thanh toán (mock) → nhận vé", **chạy trên compose trong CI**,
không flaky qua 20 lần chạy liên tiếp.

Khảo sát `develop` @ `51c4729` cho thấy luồng này **không chạy được trên compose**:

| Bước | Backend trên compose |
|---|---|
| Trang sự kiện | `lib/event-service.ts` là dữ liệu giả cứng; event-svc chỉ có `/healthz` |
| Join / status / admit | waitingroom thật, nhưng web gọi `NEXT_PUBLIC_API_BASE=http://localhost:8080` mà không có gì nghe ở :8080 (gateway là stub), không có CORS, join đòi `X-Identity-Id` |
| Hold | ticketing thật, nhưng id hạng vé trong frontend không phải UUID của seed |
| Thanh toán | payment-svc chỉ có `/healthz` |
| Đơn hàng / vé | không có route GET order, không code nào ghi bảng `tickets` |

Vì vậy slice này kiểm **toàn bộ frontend thật** (Next build production, router, sessionStorage, hook, fetch)
với backend được giả lập **đúng theo hợp đồng HTTP** mà frontend đang gọi. AC "chạy trên compose" của Jira
**chưa đạt** và được ghi lại là nợ, chờ gateway + payment + order/ticket backend.

## 2. Lỗi frontend đã phát hiện khi khảo sát (sửa trong slice này)

| # | Lỗi | Bằng chứng | Hậu quả |
|---|---|---|---|
| B1 | Lệch khoá token: phòng chờ ghi `ef:queue:token:{eventId}`, trang giữ vé đọc `evenflow_queue_token_{eventId}` | `components/queue/waiting-room.tsx:22`, `hooks/use-hold-timer.ts:36` | Khách đã được gọi vẫn bị báo "Chưa được cấp quyền mua vé", request hold không bao giờ được gửi |
| B2 | Hold và thanh toán gọi URL tương đối: `apiBaseUrl` mặc định `""`, không ai truyền vào | `lib/hold-client.ts:108`, `lib/payment-client.ts:140,245`, `components/checkout/checkout-view.tsx:51-55` | Request đi vào chính server web → 404, thay vì `NEXT_PUBLIC_API_BASE` như join/status |
| B3 | Chuyển sang trang thanh toán không kèm `tierId` / `quantity` | `checkout-view.tsx:89`, `payment/page.tsx:44-45` | Trang thanh toán luôn hiện hạng vé đầu tiên × 1, số tiền sai |

## 3. Tiêu chí nghiệm thu

Sự kiện dùng cho test: `evt-tech-summit-2026` (slug `vietnam-tech-summit-2026`, ON_SALE), hạng
`tier-all-access` "All-Access Executive Pass" 2.500.000 ₫. "API_BASE" = giá trị `NEXT_PUBLIC_API_BASE`
dùng khi build web cho E2E.

**AC-1 — Luồng chính chạy xanh.**
Given web build production, backend giả lập trả về đúng hợp đồng,
When khách mở `/events/vietnam-tech-summit-2026`, bấm "Vào phòng chờ mua vé",
Then được đưa tới `/waiting/evt-tech-summit-2026`, thấy vị trí trong hàng chờ (QUEUED, có rank),
và khi server trả ADMITTED thì tự chuyển tới `/checkout/evt-tech-summit-2026`;
When chọn "All-Access Executive Pass", số lượng 2, bấm giữ vé,
Then thấy trạng thái đang giữ vé có đồng hồ; When bấm sang thanh toán, điền thông tin và thanh toán,
Then tới trang kết quả và thấy đơn **đã thanh toán** cùng mã vé do server trả về.

**AC-2 — Hold dùng đúng token và đúng địa chỉ (B1, B2).**
Request `POST {API_BASE}/v1/events/evt-tech-summit-2026/holds` có `X-Queue-Token` **bằng đúng**
`queue_token` mà join trả về, có `Idempotency-Key` không rỗng, body
`{ ticket_type_id: "tier-all-access", quantity: 2 }`. Không có request nào tới `/v1/...` trên origin của web.

**AC-3 — Thanh toán gửi đúng đơn (B2, B3).**
Request `POST {API_BASE}/v1/events/evt-tech-summit-2026/payments` có `Idempotency-Key`, body có
`order_id` / `hold_id` bằng giá trị hold trả về và `amount` = 5.000.000.

**AC-4 — Trang thanh toán hiện đúng lựa chọn (B3).**
Trang thanh toán hiện "All-Access Executive Pass", số lượng 2 và tổng 5.000.000 ₫ (định dạng hiện có của
app), không phải hạng vé đầu tiên × 1.

**AC-5 — Join bị từ chối.** Given join trả 401, Then phòng chờ hiện thông báo cần đăng nhập/xác thực OTP
(role `alert`) và không chuyển trang.

**AC-6 — Hết vé khi giữ.** Given hold trả 409, Then trang chọn vé hiện thông báo hạng vé đã hết,
không chuyển sang trang thanh toán.

**AC-7 — Giả lập không che lỗi.** Bộ test chỉ giả lập request có origin = API_BASE. Mọi request
`/v1/**` tới origin của web, và mọi request tới API_BASE không có handler, làm test **fail** với thông báo
nêu rõ URL (không lặng lẽ trả 404).

**AC-8 — Ổn định.** `npx playwright test --repeat-each=20 --retries=0` trên máy và trong CI: 0 fail.
Test không dùng `waitForTimeout` cố định để chờ nghiệp vụ; jitter 0..5s của trang sự kiện được xử lý
một cách tất định (không dựa vào may rủi).

**AC-9 — Chạy trong CI cho mỗi PR.** Workflow mới chạy bộ E2E giả lập trên PR / push vào `develop`,
`main` khi đổi `apps/web/**`, `tests/e2e/**` hoặc chính workflow đó: build web production với API_BASE giả,
`next start`, chạy Playwright với `retries: 0`, upload report khi fail. Không cần Docker.
Không sửa job/gate có sẵn trong `ci.yml`.

**AC-10 — Không còn kịch bản "để sau".** `test.skip` của luồng đầy đủ trong `tests/e2e/specs/smoke.spec.ts`
được thay bằng test thật; `tests/e2e/**` không còn `.skip` / `.only` / `.todo`. Test "trang chủ tải được" giữ
nguyên và vẫn xanh.

**AC-11 — Unit test cho lỗi đã sửa, không hồi quy.** Mỗi lỗi B1–B3 có ít nhất một unit test Vitest đỏ
trên code cũ, xanh trên code mới. `apps/web`: lint, typecheck, test (≥ 1528 + test mới, 0 fail), build đều pass.

**AC-12 — Ghi nhận trung thực.** `traceability.md` ghi AC Jira "chạy trên compose" là **CHƯA ĐẠT** kèm lý do
và điều kiện để đạt; `docs/kiem-thu/README.md` cập nhật dòng E2E và bảng truy vết.

## 4. Ngoài phạm vi

- Backend thật: gateway/proxy :8080, CORS, identity, payment, order, phát hành vé, event-svc.
- `identityId = "guest-user"` mặc định của hold (backend thật cần UUID) — ghi nợ.
- Trang thanh toán truyền `expiresAt={undefined}` (không có đồng hồ ở trang thanh toán) — ghi nợ, không sửa.
- E2E nightly trên compose (`e2e-nightly.yml`) — giữ nguyên; bộ test mới vẫn phải chạy được ở đó
  (API_BASE mặc định `http://localhost:8080`).
- Trình duyệt khác ngoài Chromium.
