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
