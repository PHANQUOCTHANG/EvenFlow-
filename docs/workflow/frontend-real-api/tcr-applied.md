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

## WP3

Khong co test bi loi/phai doi assertion. Chi them test moi (khong xoa/skip/lam yeu):
- `components/event/events-browser-list.test.tsx` (moi): loading / empty / error + thu lai (AC-17).
- `components/event/event-list-view.test.tsx` (moi): co du lieu, rong, loi (AC-17).
- `app/(marketing)/events/[slug]/page.test.tsx`: them gia theo price_cents, availability AVAILABLE + ghi chu tham khao, availability loi -> UNKNOWN trung tinh, SOLD_OUT, SCHEDULED chua toi gio (AC-16).
- `components/event/event-action-panel.test.tsx`: them `it.each` cac trang thai khong ban duoc khong co CTA vao phong cho.
