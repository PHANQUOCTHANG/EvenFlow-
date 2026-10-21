# Test change request applied (WP1)

Nguon: `test-change-request.md`. Khong xoa, khong skip, khong lam yeu; chi doi assertion sang hanh vi that.

| File | Old test -> new test | Ly do |
|---|---|---|
| apps/web/src/lib/event-service.test.ts | `tra ve danh sach slug dung cho generateStaticParams` -> bo (getAllEventSlugs khong con; thay bang `generateStaticParams tra ve []` o page test) | event-service khong con danh sach slug mau; khong doc backend luc build |
| ditto | `tim dung su kien theo slug hop le` -> cung ten, chay tren fetch mock | du lieu tu API |
| ditto | `tra ve null khi khong tim thay slug` -> `... (404)` fetch mock 404 | null chi khi 404 |
| ditto | `tra ve toan bo danh sach su kien cong khai` -> `... tu API` | MOCK_EVENTS khong con |
| ditto | `tuan thu BR-A4 ...` -> giu, kiem availability thuoc AVAILABLE/FEW_LEFT/SOLD_OUT/UNKNOWN, khong quota/remaining | BR-A4 giu nguyen tren du lieu API (mapping khong tra quota) |
| ditto | (moi) `khong goi backend luc next build` | them coverage cho NEXT_PHASE |
| apps/web/src/lib/ops-service.test.ts | mock `./event-service` + MOCK_EVENTS -> mock `./sample-data/events` (SAMPLE_EVENTS + 3 su kien phu), `getAllPublicEvents` -> `SAMPLE_EVENTS`; fixture them `currency`, `maxPerOrder` | ops-service doc SAMPLE_EVENTS (AC-19); assertion khong doi |
| apps/web/src/components/event/ticket-tier-list.test.tsx | fixture: bo `description`, them `currency`; import type tu `lib/types/event`; (moi) `availability UNKNOWN hien nhan trung tinh` | kieu moi (UNKNOWN, khong description) |
| apps/web/src/components/checkout/checkout-view.test.tsx | fixture them `currency`, `maxPerOrder`; import type tu `lib/types/event` | kieu moi |
| apps/web/src/app/(marketing)/events/[slug]/page.test.tsx | `generateStaticParams tra ve danh sach slug` -> `... tra ve [] va khong goi backend`; moi test dung fetch mock thay MOCK_EVENTS; `khong co hang ve` dung fetch mock tiers rong thay spyOn; (moi) `backend loi (503) hien trang loi co lien ket thu lai`; `notFound` dung 404 mock | ISR on-demand, async lookup, error state |
| apps/web/src/app/(marketing)/events/page.test.tsx | `render tieu de chinh va danh sach` -> dung fetch mock; (moi) revalidate=60, empty state, error state khong du lieu mau, build-phase khong fetch + tai o trinh duyet | AC-17, plan 8 m-R3-3 |
| apps/web/src/app/(checkout)/checkout/[eventId]/payment/page.test.tsx, result/page.test.tsx | eventId `evt-tech-summit-2026` (mock) -> UUID + fetch mock tra su kien | event lay tu API; WP2 se chuyen trang sang client-side va thay tiep |

Khong can sua: `hooks/use-auth.test.ts` (parseJwtPayload van export), `app/unauthorized/page.test.tsx` (mock useAuth), RoleGuard (khong co test, khong doi), checkout/waiting `page.test.tsx` (mock event-service).
Chua dong den (thuoc WP khac): organizer-service/payment-client/hold-client/queue-client tests va organizer component tests.

## WP2

Nguon: `test-change-request.md`. Khong xoa, khong skip, khong lam yeu; chi doi assertion sang hanh vi that.

| File | Old test -> new test | Ly do |
|---|---|---|
| apps/web/src/lib/hold-client.test.ts | `gui dung endpoint, headers ...` -> doc header bang `Headers`; `X-Identity-Id = user-456` -> `expect(headers.has("X-Identity-Id")).toBe(false)`; bo `identityId` | AC-10: khong bao gio gui header danh tinh; apiFetch dung `Headers` |
| ditto | `tu sinh Idempotency-Key ...` -> doc key qua `Headers` | apiFetch dung `Headers` |
| ditto | `409 do cung Idempotency-Key dang xu ly la AMBIGUOUS` -> mock tra `Response` that (co body problem+json) thay object tho | `readProblem` doc body bang `res.clone()` |
| ditto | `chuyen loi 422 thanh LIMIT_EXCEEDED` -> giu, doi ten: 422 khong co problem type nhan dien duoc van la LIMIT_EXCEEDED; PER_IDENTITY_LIMIT / QUANTITY_NOT_ALLOWED do `hold-client.contract.test.ts` phu | AC-10: 422 tach theo `type` |
| ditto | `apiBaseUrl truyen tuong minh van thang bien moi truong` -> `tham so apiBaseUrl da bo ... goc API luon tu NEXT_PUBLIC_API_BASE` (caller cu truyen van bi bo qua) | apiFetch tu them `apiBase()`; khong con tham so ghi de |
| apps/web/src/lib/payment-client.test.ts | `gui header Idempotency-Key va body chuan ...` -> `stubEnv(NEXT_PUBLIC_API_BASE)` thay cho `apiBaseUrl`, header doc qua `Headers` | apiFetch + bo tham so base |
| ditto | `che do sandbox mock tra ve ket qua redirect thanh cong khi khong co backend` -> `... KHONG gia thanh cong, reject NETWORK_ERROR` | AC-12 |
| ditto | `lay chi tiet don hang thanh cong tu API va luu vao cache` -> giu, bo `qrPayload` khoi `issuedTickets` (khong con trong kieu `IssuedTicket`), bo tham so base | AC-14/15: QR chi den tu issueTickets |
| ditto | `lay tu sessionStorage khi fetch bi loi mang` -> `loi mang KHONG phuc vu cache ..., reject NETWORK_ERROR` | AC-14 |
| ditto | B2: `initiatePayment/getOrderDetails: apiBaseUrl truyen tuong minh van thang` -> bo qua tham so, URL luon tu env; `getOrderDetails gui Authorization khi da dang nhap` -> gieo `ef_token`+`ef_user` o localStorage (session lib) va doc `Headers` | AC-3 + bo tham so base |
| apps/web/src/lib/queue-status.test.ts | `200 -> updated ...` -> `stubEnv(NEXT_PUBLIC_API_BASE)` thay cho `baseUrl`, header qua `Headers`; `khong co ETag truoc do` doc qua `Headers` | fetchQueueStatus qua apiFetch; bo `baseUrl` |
| apps/web/src/lib/sse.test.ts | `gui header (X-Queue-Token) va Accept ...` -> doc header qua `Headers` | connectSse qua apiFetch |
| apps/web/src/hooks/use-hold-timer.test.tsx | 2 test doc `init.headers["X-Queue-Token"]` -> `new Headers(init.headers).get(...)` | apiFetch dung `Headers` |
| apps/web/src/components/checkout/payment-handoff-view.test.tsx | `render day du ... cac cong thanh toan`, `khoa form ... het han`, `hien thi loi validation`, `goi onPaymentSuccess` -> spy `listPaymentMethods`, doi dong bo sang `findBy*`; them 4 test (loc cong da tat + stripe, loi+thu lai, rong, mac dinh = cong dau tien dang bat) | AC-13: cong thanh toan tu API, khong con danh sach cung |
| apps/web/src/components/checkout/order-result-view.test.tsx | `render trang thai PAID voi ve dien tu va ma QR demo` -> `PAID -> goi issueTickets, moi ve mot khung QR tu qr_payload that, khong co QR demo` | AC-15, BR-O8 |
| ditto | 2 test a11y QR (`moi ve co mot khoi QR role img`, `khong con aria-label ...`) -> ve den tu `issueTickets` (spy), ma ve = 8 ky tu dau cua ticketId | `issuedTickets` cua don khong co QR; AC-15 |
| ditto | (moi) thu thanh toan lai mang du tham so; pending phat hanh ve + thu lai co gioi han; het luot -> loi + lay lai; 404 NOT_FOUND; loi mang + thu lai | AC-14, AC-15 |
| apps/web/src/app/(checkout)/checkout/[eventId]/{page,payment/page,result/page}.test.tsx | render page + `generateMetadata` theo ten su kien -> render vo mong voi client component (mock) + `metadata` tinh noindex; payment/result khong con bia `orderId`/`holdId` | AC-18: su kien tai phia client, server khong goi backend |
| apps/web/src/app/(queue)/waiting/[eventId]/page.test.tsx | tuong tu (`WaitingRoomPageClient`) | AC-18 |

Test moi (WP2, khong thay test cu): `components/event/event-loader.test.tsx`, `components/checkout/{checkout-view.wp2,payment-page-client,result-page-client}.test.tsx`, `components/queue/waiting-room.wp2.test.tsx`.
Khong can sua: `components/event/ticket-tier-list.test.tsx` (UNKNOWN da duoc WP1 xu ly), `components/queue/queue-position.test.tsx`, `hooks/use-queue-status.test.tsx`, `lib/queue-client.test.ts`.
## WP3

Khong co test bi loi/phai doi assertion. Chi them test moi (khong xoa/skip/lam yeu):
- `components/event/events-browser-list.test.tsx` (moi): loading / empty / error + thu lai (AC-17).
- `components/event/event-list-view.test.tsx` (moi): co du lieu, rong, loi (AC-17).
- `app/(marketing)/events/[slug]/page.test.tsx`: them gia theo price_cents, availability AVAILABLE + ghi chu tham khao, availability loi -> UNKNOWN trung tinh, SOLD_OUT, SCHEDULED chua toi gio (AC-16).
- `components/event/event-action-panel.test.tsx`: them `it.each` cac trang thai khong ban duoc khong co CTA vao phong cho.
## WP4

| File | Old test -> new test | Ly do |
|---|---|---|
| apps/web/src/lib/organizer-service.test.ts | `lay danh sach toan bo su kien cua organizer` -> cung y, fetch mock `GET /api/v1/events` | kho mock da bo |
| ditto | `lay su kien theo ID hoac Slug` -> `lay su kien theo ID; 404 -> null` | chi tra cuu theo id that (slug khong con duoc ho tro) |
| ditto | `tao moi su kien thanh cong va luu vao store` -> `tao moi su kien voi submitForReview -> trang thai PENDING_REVIEW tu backend` | tra `{event, failedStep?}`; trang thai do backend quyet dinh, khong con AI scan gia lap |
| ditto | `tu choi cap nhat giam quota ... ON_SALE (BR-E3)` (updateOrganizerEvent tiers) -> `updateTierQuota` chan o client, khong goi PUT | updateOrganizerEvent khong con nhan tiers; quota qua updateTierQuota |
| ditto | `resubmitForModeration cap nhat ... SCHEDULED` -> `submitOrganizerEvent chuyen sang PENDING_REVIEW` | ham resubmit gia lap bi bo, thay bang POST /submit |
| ditto | `getRealtimeSales ...` -> cung assertion tren fetch mock + `isSample === true` | sales giu mau nhung phai co isSample |
| ditto | bo `resetOrganizerEventsStore`/`beforeEach` store | store mock bi xoa; setup env + session thay the |
| apps/web/src/components/organizer/event-list.test.tsx | `render cac link hanh dong nhanh ...`: link edit chi cho evt-2, evt-3 (khong cho ON_SALE evt-1) | AC-22: chi DRAFT/SCHEDULED/REJECTED moi duoc sua |
| apps/web/src/components/organizer/sales-dashboard.test.tsx | fixture them `isSample: true`; `cho phep bat/tat tu dong cap nhat`: mac dinh tat, bam bat truoc | du lieu mau khong polling mac dinh; (moi) test nhan "Du lieu mau" truoc cac khoi so lieu |
| apps/web/src/hooks/use-organizer-sales.test.tsx | fixture them `isSample: true` | kieu moi |
| apps/web/src/app/(organizer)/organizer/events/page.test.tsx | `await OrganizerEventsPage()` (server, mock store) -> client fetch voi loading/empty/error + retry | AC-20: lay danh sach client-side |
| .../events/new/page.test.tsx | (giu) render wizard; (moi) thanh cong, ket qua mot phan failedStep + link ban nhap, loi buoc 1 | AC-21 |
| .../events/[eventId]/edit/page.test.tsx | `render form chinh sua su kien da co` (id mock) -> fetch mock UUID + them: 404, loi, trang thai khong sua duoc, PUT day du / tiers moi / quota | AC-22 |
| .../events/[eventId]/review-status/page.test.tsx | `render trang thai kiem duyet` (mock, "Da duyet") -> trang thai that + nhan mau; `non-existent` 404; (moi) submit/cancel + 422 | AC-23, AC-24 |
| .../events/[eventId]/sales/page.test.tsx | dashboard tu mock store -> fetch client-side + nhan "Du lieu mau"; 404 | AC-24 |
| apps/web/src/lib/ops-service.test.ts | khong doi (da thay o WP1) | |

## WP6

Nguon: AC-28 (spec.md). Khong xoa, khong skip, khong lam yeu assertion cu; chi doi hang so/id va setup cho luong API that.

| File | Old -> new | Ly do |
|---|---|---|
| tests/e2e/fixtures/buy-flow-helpers.ts | `EVENT_ID`, `TIER_ID`, `ORDER_ID`, `HOLD_ID` la chuoi `evt-...`/`tier-...`/`ord-...` -> UUID (events.json); `TICKET_CODES` `E2E-TKT-000x` -> `E2E00001`, `E2E00002` | backend tu choi id khong phai UUID (400 -> null); man hinh ket qua hien ma ve = 8 ky tu dau cua `ticketId` viet hoa nen ma ve phai suy ra tu ticketId. Assertion `tickets.getByText(code)` giu nguyen |
| tests/e2e/fixtures/buy-flow-helpers.ts | `mockHoldCreated` tra 201 -> 200 | gateway/ticketing that tra 200 (contracts.md muc 4) |
| tests/e2e/fixtures/buy-flow-helpers.ts | `mockPaidOrder`: `issued_tickets[].qrPayload` bo; them `total_cents`, `currency`, `items`, `hold_id`, `expires_at`, `status` tren ve | dung ten truong that cua GET /v1/orders/{id}; backend khong tra QR payload |
| tests/e2e/fixtures/api-mock.ts | CORS: bo `Access-Control-Allow-Credentials`, Allow/Expose-Headers y het `gateway.go`; header tac gia ngoai danh sach -> vi pham (them kiem tra) | AC-28; fixture truoc day cho phep nhieu hon gateway |
| tests/e2e/specs/buy-flow.spec.ts | `beforeEach` them `seedLoggedInSession(page)` | web that gui Bearer; luong mua ve bat dau o trang thai da dang nhap (ef_token/ef_refresh/ef_user trong localStorage) |
| tests/e2e/specs/buy-flow.spec.ts | THEM (khong thay the): sau khi kiem ma ve, kiem `QR-<ma>` hien trong khung QR va `POST /v1/orders/{id}/issue-tickets` duoc goi | AC-15: QR hien payload ky that tu issue-tickets (san pham moi hien payload dang chu) |
| tests/e2e/specs/a11y.spec.ts | test luong mua ve va test join 401 goi `seedLoggedInSession(page)` truoc `goto` | nhu tren; test `/auth/sign-in` va trang cong khai khong seed |
| tests/e2e/specs/i18n.spec.ts | chi doi comment ve nguon su kien SCHEDULED | du lieu nay nay den tu events.json/stub-api |
| tests/e2e/specs/ops-war-room.spec.ts, smoke.spec.ts | khong doi | ops van dung SAMPLE_EVENTS (AC-19) |
| tests/e2e/tsconfig.json | them `resolveJsonModule` | import `events.json` dung chung giua api-mock.ts va stub-api.mjs |
| tests/e2e/playwright.config.ts | them `webServer` chay `fixtures/stub-api.mjs` (:8080, `reuseExistingServer: !CI`) | fetch phia server cua Next (SSR/ISR) khong di qua page.route |

## Integrator (after merging wp/fe-qr)

| File | Change | Why |
|---|---|---|
| tests/e2e/specs/buy-flow.spec.ts | `getByText("QR-<code>")` → `[data-qr-payload="QR-<code>"][data-qr-state="ready"] svg` visible | The result page now draws a scannable SVG QR from the payload (human-approved `qrcode` dependency) instead of printing the payload text; the new assertion is stronger (payload bound to the element AND an SVG rendered) |

## Fix round 1 (organizer: F-1, F-2, F-3, F-9, F-10, F-19)

| File | Old -> new | Why |
|---|---|---|
| apps/web/src/app/(organizer)/organizer/events/[eventId]/edit/page.test.tsx | `it.each` of non-editable states: `ON_SALE` removed (now opens the quota-only form; new tests assert PUT tier quota only, decrease blocked) | F-1 / AC-22: ON_SALE tier quota increases must be reachable |
| apps/web/src/app/(organizer)/organizer/events/[eventId]/edit/page.test.tsx | partial tier failure test: no longer asserts the "Mo ban nhap" link (`/edit` on the same page); asserts the event is reloaded instead | F-3: a link to the current route never reloaded; retry now diffs against freshly loaded server tiers |
| apps/web/src/components/organizer/event-wizard.test.tsx | BR-E2 test: step 3 with <30 min shows the BR-E2 warning and still advances; "Gui kiem duyet" is blocked with BR-E2 | F-19: gate belongs to submit-for-review only (matches createOrganizerEvent) |
| apps/web/src/components/organizer/event-wizard.test.tsx | ON_SALE decrease test goes through the quota-only form ("Luu so luong ve") instead of wizard step 2 | F-1: ON_SALE renders quota-only mode; BR-E3 message unchanged |
## Fix round 1 (flow)

| File | Old -> new | Why |
|---|---|---|
| apps/web/src/components/checkout/order-result-view.test.tsx | WP2 PAID test: synchronous `getByRole("heading", /Vé điện tử của bạn đã sẵn sàng/)` moved to AFTER `await findByRole("img", "Mã QR vé ABCDEF12")` (assertion text unchanged) | F-13 (C-11): the "tickets are ready" headline is now shown only when QR tickets exist; before that the headline is "Thanh toán thành công — đang phát hành vé". The test still asserts the same headline once the QR is rendered. |

## Fix round 2

| File | Old -> new | Why |
|---|---|---|
| apps/web/src/app/(organizer)/organizer/events/[eventId]/edit/page.test.tsx | `it.each` of non-editable states: `SOLD_OUT` removed (now opens the quota-only form); added `G-2` tests: SOLD_OUT raises quota via PUT tier only, SOLD_OUT decrease blocked client-side | G-2: backend (event.go) accepts a quota increase on SOLD_OUT and reopens the sale; the old test locked the exclusion |
| apps/web/src/components/organizer/event-list.status.test.tsx | "F-1" test expected exactly 1 'Tăng số lượng vé' link for [ON_SALE, SOLD_OUT]; now expects 2 links (ON_SALE, SOLD_OUT) and none for CLOSED | G-2 |
| apps/web/src/lib/fix1/f14-lock-timeout.test.ts | lock never granted: tab refreshed by itself outside the lock -> tab requests the lock with `{steal: true}` and refreshes inside it (fake lock grants a steal request at once) | G-1: refresh must never run outside the Web Lock (refresh-token reuse revokes the family) |
| apps/web/src/lib/fix1/f6-register-autologin.test.tsx | sign-in link href `/auth/sign-in?identifier=a%40b.co` -> `/auth/sign-in` | G-7: identifier goes through one-shot sessionStorage, not the URL |
| apps/web/src/lib/fix1/f17-logout-clears-buyflow.test.ts | unchanged (explicit logout still clears buy-flow keys); G-4 adds fix2/g4 test that involuntary expiry keeps them | G-4 |

G-9 (403 `/problems/forbidden` -> FORBIDDEN) skipped: conflicts with a protected contract test, see test-change-request-fix2.md.
| apps/web/src/lib/fix1/f16-google-remember.test.tsx | "F-6: sign-in prefills identifier from ?identifier=" -> prefill from one-shot sessionStorage key `ef:signin:identifier` (removed after read) | G-7 |
