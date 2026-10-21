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
