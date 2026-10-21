# Test change request (frontend-real-api)

Cac test hien co khang dinh hanh vi MOCK / hanh vi cu, se vo mot cach hop le khi implement. Test designer KHONG sua; implementer
sua/thay chung (khong xoa, khong skip, khong lam yeu) kem ly do duoi day. File nay duoc suy ra tu doc ma, can xac nhan lai bang
`npx vitest run` sau khi implement.

## Chac chan vo (da doc noi dung)

| File | Test | Ly do |
|---|---|---|
| apps/web/src/lib/event-service.test.ts | tat ca (`tra ve danh sach slug dung cho generateStaticParams`, `tim dung su kien theo slug hop le`, `tra ve null khi khong tim thay slug`, `tra ve toan bo danh sach su kien cong khai`, `tuan thu BR-A4...`) | Doc MOCK_EVENTS / getAllEventSlugs; service doi sang goi API, tra Result `{ok,...}`, MOCK_EVENTS di chuyen sang lib/sample-data (SAMPLE_EVENTS). Thay bang test fixture + mock fetch (da co `event-service.contract.test.ts`); giu test BR-A4 (availability dang khoang) tren du lieu API. |
| apps/web/src/lib/organizer-service.test.ts | `lay danh sach toan bo su kien cua organizer`, `lay su kien theo ID hoac Slug`, `tao moi su kien thanh cong va luu vao store`, `tu choi cap nhat giam quota cua su kien dang ON_SALE (BR-E3)` (async), `resubmitForModeration cap nhat trang thai ... SCHEDULED`, `getRealtimeSales ...` | Dung kho mock trong bo nho + `resetOrganizerEventsStore`; ham doi sang goi API (createOrganizerEvent tra `{event, failedStep?}`). Cac test validator thuan (validateEventSchedule / validateTiers / validateTierQuotaUpdate / evaluateAiScanRisk) KHONG doi. getRealtimeSales giu sample nhung phai mang `isSample`. |
| apps/web/src/lib/payment-client.test.ts | `che do sandbox mock tra ve ket qua redirect thanh cong khi khong co backend` | Khang dinh dung hanh vi bi cam (AC-12: gia payment_id/thanh cong). Thay bang test mong doi reject PaymentClientError. |
| apps/web/src/lib/payment-client.test.ts | `lay chi tiet don hang thanh cong tu API va luu vao cache`, `lay tu sessionStorage khi fetch bi loi mang` (+ test 404 dung cache/ORDER_NOT_FOUND neu co) | AC-14: khong phuc vu cache nhu du lieu song, 404 -> NOT_FOUND (doi ten tu ORDER_NOT_FOUND), khong dung mac dinh quantity/tier_name. Cac test dung `apiBaseUrl` tham so: chuyen sang `vi.stubEnv("NEXT_PUBLIC_API_BASE")` neu tham so bi bo. |
| apps/web/src/lib/hold-client.test.ts | test header co `X-Identity-Id` = `user-456` (`expect(init.headers["X-Identity-Id"])`) | AC-10: khong bao gio gui X-Identity-Id; `identityId` bi bo khoi tham so. |
| apps/web/src/lib/hold-client.test.ts | `chuyen loi 422 thanh LIMIT_EXCEEDED` | AC-10: 422 tach thanh PER_IDENTITY_LIMIT / QUANTITY_NOT_ALLOWED theo `type`; 403 co type queue-token-* -> QUEUE_TOKEN_INVALID. Test `chuyen loi 403 thanh NOT_ADMITTED` (type about:blank) van dung. Mock fetch dang tra object tho `{ok, status, json}` va doc `init.headers[...]` nhu plain object: neu apiFetch dung `Headers` thi doi cach assert. |

## Co the vo (can xac nhan khi chay)

| File | Ly do du kien |
|---|---|
| apps/web/src/lib/queue-client.test.ts, queue-status.test.ts, sse.test.ts, hooks/use-queue-status.test.tsx | Cac test mock `globalThis.fetch` bang `Response`; neu code di qua `apiFetch` (them Bearer, doc `localStorage`) hay doi `credentials` thi assert `init.credentials === "include"` (neu co) phai bo; QueueError them `code`. |
| apps/web/src/hooks/use-auth.test.ts | `parseJwtPayload` co the van duoc giu; chi vo neu export bi bo. |
| apps/web/src/app/(marketing)/events/[slug]/page.test.tsx, app/(checkout)/checkout/[eventId]/page.test.tsx, app/(queue)/waiting/[eventId]/page.test.tsx | Import `MOCK_EVENTS`/`getEventBySlug` dong bo; pages doi sang Result + fetch (notFound() cho 404, error state cho unavailable); `generateStaticParams` khong doc mock. |
| apps/web/src/components/checkout/checkout-view.test.tsx, components/event/ticket-tier-list.test.tsx | Dung MOCK_EVENTS lam fixture va gia dinh `availability` khong co "UNKNOWN", `price` field. Chuyen fixture sang `lib/sample-data/events.ts` hoac literal trong test. |
| apps/web/src/components/organizer/{event-list,event-wizard,moderation-status-card,sales-dashboard}.test.tsx, hooks/use-organizer-sales.test.tsx | Dung mock organizer store/ham dong bo; doi sang async + API mock; wizard doi sang `{event, failedStep}`; sales/moderation phai hien nhan sample. |
| apps/web/src/lib/ops-service.test.ts | Import MOCK_EVENTS; doi import sang `lib/sample-data/events` (SAMPLE_EVENTS). |
| apps/web/src/components/auth/*.test.tsx (LoginForm, RegisterForm, ...) neu co | Mock `lib/api/auth`; sua neu chu ky tra ve doi (setSession, persist, register->login). |
| apps/web/src/components/event/event-action-panel.test.tsx | Baseline da co 1 "unhandled error sau khi moi truong test tat" (khong gay fail) -- khong lien quan, nhung de y neu so luong error doi. |
| tests/e2e/fixtures/api-mock.ts va cac spec trong tests/e2e/specs | AC-28: CORS giong gateway, hold 200, mock them auth/events/availability/methods/issue-tickets. Chay Playwright (khong thuoc vitest). |

## Go (additive, khong vo ve logic)

| File | Thay doi can thiet |
|---|---|
| services/event/internal/adapter/http/handler_test.go (`fakeRepo`) | Phai thoa `port.EventRepository` moi (them `ListPublic`). Phuong thuc da duoc dinh nghia trong `public_events_contract_test.go` (cung package) -> KHONG them lai vao handler_test.go (se trung ten). |
| services/event/internal/app/fakes_test.go (`fakeEventRepo`) | Tuong tu: `ListPublic` da o `app/list_public_contract_test.go`. |
| Fake khac thoa `port.EventRepository` (neu implementer tim thay them, vd trong moderation/handle_ai_verdict test) | Them `ListPublic` additive. |
| Neu implementer doi ten/chu ky `ListPublic(ctx, limit int) ([]*domain.Event, error)` | Sua dung cac ham fake trong 2 file test moi tren va test tich hop postgres (ten nay do test designer GIA DINH). |

## Bo sung theo coordinator

| File | Ly do |
|---|---|
| tests/e2e/specs/i18n.spec.ts, a11y.spec.ts, buy-flow.spec.ts | Hang so cung (`EVENT_SLUG`, id su kien mock, ten hang ve, gia) tro toi du lieu mock; sau doi sang UUID/slug that tu `api-mock.ts` thi phai doi hang so theo fixture moi. Test i18n "ISR" (`/events`, `/events/[slug]`) giu nguyen y nghia (revalidate=60) nhung can fixture goi API that/mocked o gateway. |
| apps/web/src/app/(checkout)/checkout/[eventId]/page.test.tsx, payment/**/page.test.tsx, result/**/page.test.tsx | Phai doi sang event theo UUID qua `getEventById` (null -> notFound()), khong con MOCK_EVENTS. |
| apps/web/src/app/(queue)/waiting/[eventId]/page.test.tsx | Tuong tu; dong thoi thong bao OTP/thiet bi (AC-9). |

Luu y quy trinh: test designer da dung `git stash -u` mot lan de do baseline (da pop ngay, khong con stash); khong lap lai.
Hinh dang event-service da dong bang: `getEventBySlug/getEventById` -> `EventDetailSnapshot | null` (null chi khi 404; 400 id khong hop le cung null), loi khac reject; `getAllPublicEvents` -> mang, reject khi loi. KHONG co `getAvailability` rieng trong hop dong test.
