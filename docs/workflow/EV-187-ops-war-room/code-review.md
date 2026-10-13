# EV-187 — Review code (Code review)

| Mục | Giá trị |
|---|---|
| Người review | Agent review độc lập (context sạch) |
| Đầu vào | `spec.md`, `plan.md` (§6 thay §1), `plan-review.md`, diff chưa commit so với `origin/develop` @ `51c4729` |
| Worktree | `D:/Documents/Nam4-1/Project1/EvenFlow-187` (branch `feature/EV-187-ops-war-room`) |
| Ngày | 2026-10-13 |
| Kiểm chạy | `npx vitest run` (ops-service, format, hook, components/ops, app/(ops), components/layout): **16 file, 430 test pass**; `npx tsc --noEmit`: exit 0; `npx eslint` các file nguồn đã đổi: sạch |

Ghi chú: test quanh dòng ~229 của `components/ops/ops-war-room.test.tsx` đang được test designer viết lại song song — không review phần đó.

## Kết luận: **PASS** (không có blocker; 1 major nên sửa trước merge, các minor sửa hoặc ghi nợ)

Implementation bám sát hợp đồng §6: hook an toàn hydration (`ageMs = null` tới khi mount), `formatTimeVn`/`formatCount`
tính tay, page `try/catch` với `notFound()` gọi **ngoài** `try`, `force-dynamic` ở cả hai trang, `__setOpsSampleOverride`
no-op khi production, BR-O1 bỏ qua khi `ticketing ≠ ok`, ánh xạ nguồn → ô đúng M-3, câu AI khớp nguyên văn m-5,
khu "Điều khiển" tách riêng với nút vô hiệu. Phạm vi diff nằm trong danh sách file được phép (§3 + §6).

## 1. Đối chiếu AC và quyết định §6

| Mục | Kết quả | Bằng chứng |
|---|---|---|
| AC-1 | Đạt | `ops-panels.tsx:77-109` (6 bước phễu + đơn vị), `:111-146` (admit rate "khách/giây", mode, độ sâu), `:148-202` (bảng tồn kho), `:211-247` (AI) |
| AC-2 | Đạt | `use-ops-war-room.ts:137` tick 1s, `:153-155` `ageMs`/`isStale`; `ops-war-room.tsx:181-187` banner `Alert warning` (role `alert`), số liệu giữ nguyên |
| AC-3 | Đạt | `ops-war-room.tsx:175-179` "Dữ liệu mẫu" (`Alert info` → role `status`); `ops-panels.tsx:51-68` "Không có dữ liệu" / "(cũ)" |
| AC-4 | Đạt | `ops-service.ts:82-88`; `ops-war-room.tsx:140-143,189-193`; `ops-panels.tsx:185-191` (chữ "Vượt quota"/"Còn lại âm") |
| AC-5 | Đạt | `ops-panels.tsx:204-209` khớp nguyên văn m-5; `:227` "Ops đặt tay" |
| AC-6 | Đạt | `ops-panels.tsx:251-265` nút `disabled`, chữ "Chưa có API ghi đè (EVF-60)", không có handler |
| AC-7 | Đạt | `ops-war-room.tsx:88-137`; `page.tsx` `[eventId]` dòng 76-92; `not-found.tsx` |
| AC-8 | Đạt | `app/(ops)/ops/events/page.tsx`; `nav-config.ts:35` `ready: true`, comment cập nhật (m-2) |
| AC-9 | Đạt | hook `:78-122` một `setTimeout` tự lên lịch sau khi xong, `inflightRef` dedupe, dọn ở `:139-146` |
| AC-10 | Đạt (unit/type/lint); E2E chưa chạy trong review này | `tests/e2e/specs/ops-war-room.spec.ts` bắt `console` error + `pageerror`, bấm "Làm mới" |
| AC-11 | Đạt phần chính, có minor | `<table>` + `<caption>` + `th scope`; `h1` = tên sự kiện; không `aria-live` trên vùng số; xem m-2 (focus khi bấm "Làm mới") |
| M-1 hydration | Đạt | Lần render đầu không gọi `Date.now`/`toLocale*`: `ageMs` null, `isStale` chỉ phụ thuộc `error` (null), giờ hiển thị qua `formatTimeVn` |
| M-2 | Đạt | `notFound()` ngoài `try` (`page.tsx:92`); `initialError` là object thường; test page phân biệt được (nếu `notFound()` nằm trong `try`, lỗi bị nuốt và test 404 fail) |
| M-3 | Đạt | `FUNNEL_STEPS` (`ops-panels.tsx:83-88`) map đúng nguồn; BR-O1 bỏ qua khi `ticketing ≠ ok` (`ops-war-room.tsx:140`, `ops-panels.tsx:151`) |
| M-4 | Đạt | `ops-service.ts:111-163` đúng bảng số, map trạng thái, không dựa `Date.now` |
| M-6 | Đạt | initialSnapshot → fetch sau `intervalMs`; `refresh()` trả cùng promise; 403 dừng poll (`:107`); StrictMode: lần mount thứ hai nhận lại cùng promise đang bay → chỉ một fetch, một timer |
| m-3 | Đạt | `dynamic = "force-dynamic"` ở cả hai trang + test |
| m-4 | Đạt | `ops-service.ts:103`. Next thay `process.env.NODE_ENV` lúc build nên bản client thành `return` ngay; `overrides` là `Map` cục bộ module, không phải biến toàn cục → không ghi được từ trình duyệt |
| m-7 | **Lệch nhẹ** | xem m-5 dưới đây |

## 2. Phát hiện

### Major

**M-1 — `formatCount` cắt phần thập phân của chỉ số vốn là số thực (admit rate, độ trễ chat).**
Bằng chứng: `lib/format.ts:58-62` dùng `Math.trunc`; `ops-panels.tsx:124` (`admission.ratePerSec`) và `:235`
(`ai.chatQueueLagSeconds`) đi qua hàm này. Nguồn thật là số thực: `services/waitingroom/internal/app/admit_controller.go:31`
("nguoi/giay * 1000, de luu float"), `Backoff 0.5` (`config/admit_test.go:41`) cho ra 12,5 → 6,25 khách/giây;
`ai_queue_lag_seconds` là `Gauge` float (`services/ai-worker/app/main.py:29`). Kiểm tay: `formatCount(12.7) = "12"`,
`formatCount(-0.5) = "-0"`, `formatCount(1e21) = "1e+21"`. Với độ trễ 4,9 giây màn hình hiện "4 giây" — đánh giá thấp
đúng chỉ số mà màn này dùng để quyết định. Hiện chưa lộ vì dữ liệu mẫu là số nguyên, nhưng hợp đồng được thiết kế để
thay thân `getOpsWarRoom` mà không sửa UI.
Sửa: giữ `formatCount` cho số đếm (và chuẩn hoá `-0` → `"0"`), thêm hàm định dạng số thực tính tay một chữ số thập phân
dấu phẩy (vd. `formatDecimal(12.5) = "12,5"`) cho `ratePerSec` và `chatQueueLagSeconds`; thêm test cho số thập phân,
`-0.5`, số âm có phần lẻ.

### Minor

**m-1 — `formatTimeVn` phụ thuộc múi giờ máy khi ISO không có `Z`/offset.**
Bằng chứng: `lib/format.ts:67` dùng `Date.parse(iso)`; chuỗi dạng `"2026-10-13T10:00:00"` được JS hiểu là giờ **địa phương**:
chạy với `TZ=UTC` ra `"17:00:00"`, với `TZ=Asia/Ho_Chi_Minh` ra `"10:00:00"`. Comment (`:64-65`) khẳng định "không phụ thuộc
múi giờ của máy" là sai cho trường hợp này, và khi Ops API thật trả ISO thiếu offset thì server (UTC) và trình duyệt (UTC+7)
render khác nhau → lỗi hydration đúng loại M-1 muốn chặn. `toISOString()` hiện tại luôn có `Z` nên chưa lộ.
Sửa: chỉ chấp nhận chuỗi có `Z` hoặc `±HH:MM` (regex), còn lại trả `"--:--:--"`; thêm test cho chuỗi thiếu offset.

**m-2 — Bấm "Làm mới" bằng bàn phím làm mất focus.**
Bằng chứng: `ops-war-room.tsx:168` truyền `loading={isRefreshing}`; `components/ui/button.tsx:46,62` biến `loading` thành
`disabled`. Theo quy tắc focus fixup của HTML, phần tử đang focus bị `disabled` sẽ mất focus (về `body`) — người dùng bàn phím
phải tab lại từ đầu sau mỗi lần làm mới. AC-11 chỉ nói "cập nhật tự động", nhưng đây là cùng tinh thần. Test focus
(`ops-war-room.test.tsx:649-677`) chỉ kiểm poll tự động.
Sửa: cho nút "Làm mới" giữ trạng thái bận bằng `aria-busy`/`aria-disabled` + chặn click khi đang chạy (không đặt `disabled`),
hoặc hiện spinner cạnh nút thay vì `loading`; thêm test bấm "Làm mới" khi nút đang focus thì focus vẫn ở nút.

**m-3 — `refresh()` trong lúc poll tự động đang bay không bật `isRefreshing`.**
Bằng chứng: `use-ops-war-room.ts:92` trả promise đang bay trước khi tới `:97` (`if (manual) setIsRefreshing(true)`). Người dùng
bấm "Làm mới" đúng lúc request tự động chậm thì nút không có phản hồi nào. Test `use-ops-war-room.test.tsx:314-334` chỉ kiểm
không chồng request.
Sửa: ở nhánh dedupe, nếu `manual` thì `setIsRefreshing(true)` (nhánh `finally` đã tắt); thêm test.

**m-4 — Dedupe in-flight không phân biệt `eventId`.**
Bằng chứng: `use-ops-war-room.ts:92` trả `inflightRef.current` bất kể promise đó là của sự kiện nào; effect `:126-149` chạy lại
khi đổi `eventId` nhưng `run(false)` có thể nhận promise cũ, và promise cũ vẫn `setSnapshot` số của sự kiện cũ (`:100-101` chỉ
kiểm `mountedRef`). Hiện được che bởi `key={eventId}` ở page (`[eventId]/page.tsx:96`), nhưng hook là API công khai.
Sửa: ghi `eventId` cùng promise in-flight (hoặc một `generation` tăng khi effect chạy lại) và bỏ kết quả lệch thế hệ; hoặc ghi rõ
trong JSDoc rằng caller phải dùng `key`.

**m-5 — "Cập nhật lúc …" không đặt cạnh nhãn "Dữ liệu mẫu" (plan §6 m-7).**
Bằng chứng: giờ cập nhật nằm ở cột phải của header (`ops-war-room.tsx:161-171`), nhãn "Dữ liệu mẫu" là `Alert` riêng bên dưới
header (`:175-179`). Với dữ liệu mẫu, giờ này dễ đọc như số sống. Sửa: đưa dòng "Cập nhật lúc …" vào trong (hoặc ngay cạnh)
`Alert` "Dữ liệu mẫu" khi `isSample`.

**m-6 — Lớp Tailwind không tồn tại.**
Bằng chứng: `styles/tokens.css:203-246` chỉ định nghĩa `text-headline-*`, `text-body-{lg,md,sm}`, `text-label-*`,
`text-numeric-metric`. Code mới dùng `text-heading-xl` (`ops-war-room.tsx:151`, `events/page.tsx:28`), `text-heading-md`
(`ops-war-room.tsx:51,196,208`, `ops-panels.tsx:62`, `not-found.tsx:9`) và `text-body-xs` (`ops-panels.tsx:38`) — không sinh CSS,
nên `h1`/`h2` và số liệu hiện cỡ chữ thường (preflight đặt heading `font-size: inherit`). `text-heading-*` là lỗi có sẵn ở
trang hàng xóm (vd. `(marketing)/events/page.tsx:20`), nhưng `text-body-xs` là mới.
Sửa: dùng `text-headline-xl`/`text-headline-md`/`text-body-sm`; số liệu có thể dùng `text-numeric-metric`.

**m-7 — Thông điệp lỗi thô được chuyển xuống client.**
Bằng chứng: `[eventId]/page.tsx:86` lấy `err.message` của **mọi** `Error` (không chỉ `OpsServiceError`) đưa vào `initialError`,
rồi `ops-war-room.tsx:118,129` hiển thị nguyên văn. Khi có API thật, lỗi mạng kiểu `fetch failed ... ECONNREFUSED 10.x.x.x`
sẽ lộ ra màn hình. Sửa: chỉ dùng `message` khi `err instanceof OpsServiceError`, còn lại dùng câu mặc định.

**m-8 — Timer 1s vẫn chạy sau 403.**
Bằng chứng: khi 403, `LiveWarRoom` vẫn mount (chỉ trả `<NoPermission />`, `ops-war-room.tsx:114`), nên `setInterval`
(`use-ops-war-room.ts:137`) tiếp tục `setTick` mỗi giây, re-render vô ích mãi. Sửa: dừng `ageTimer` khi `stoppedRef` bật
(lưu ref của interval), hoặc bỏ qua `setTick` khi đã dừng.

**m-9 — Nút điều khiển bị vô hiệu không gắn với câu giải thích.**
Bằng chứng: `ops-panels.tsx:255-262` — câu "Chưa có API ghi đè (EVF-60)" là `<p>` rời, không `aria-describedby`; trình đọc màn hình
đọc "Ghi đè admit rate, mờ" mà không có lý do. Sửa: đặt `id` cho `<p>` và `aria-describedby` trên hai nút.

**m-10 — Lặp code/copy.**
Bằng chứng: `SALE_STATE_LABEL` lặp ở `ops-war-room.tsx:41-46` và `events/page.tsx:15-20`; `NotFoundScreen`
(`ops-war-room.tsx:74-81`) trùng nội dung `not-found.tsx`; `statusOf` lặp ở hook (`:46-49`), component (`:83-86`) và page (`:79`).
Sửa: đưa nhãn trạng thái và `statusOf` vào `lib/ops-service.ts` (hoặc `components/ops`), dùng chung một màn "Không tìm thấy".

### Chất lượng test

**t-1 — Test StrictMode không có `initialSnapshot` chấp nhận fetch hai lần.**
`use-ops-war-room.test.tsx:240-242` cho phép `1 ≤ calls ≤ 2`; một implementation fetch trùng khi mount vẫn pass. Implementation
hiện tại cho đúng 1 — nên chốt `toHaveBeenCalledTimes(1)` (đúng tinh thần "không chồng request").

**t-2 — Chưa có test hydration ở mức component.**
Hook có test `seen[0] === null` (`use-ops-war-room.test.tsx:116-129`), nhưng nếu component tự gọi `Date.now()`/`toLocale*` trong
render thì không test unit nào bắt (chỉ E2E bắt, và E2E local thường cùng múi giờ). Đề xuất: `renderToString(<OpsWarRoom …/>)` với
`generatedAt` cũ 60s → markup **không** có "Tuổi dữ liệu" và **không** có banner "Dữ liệu cũ"; và render hai lần với
`vi.setSystemTime` khác nhau cho cùng chuỗi HTML.

**t-3 — Chưa test `isRefreshing`/nút bận khi làm mới thủ công**, và chưa test focus khi bấm "Làm mới" (liên quan m-2, m-3).

**t-4 — E2E: bước "Làm mới" không cô lập được tác dụng của nút.** `ops-war-room.spec.ts:46-51` chờ "Cập nhật lúc" đổi trong 10s;
poll tự động 2s cũng làm đổi, nên nút hỏng vẫn pass. Vẫn đủ để chứng minh client đã hydrate (mục tiêu M-5), nên chỉ ghi nhận.

**t-5 — `format.test.ts` thiếu ca thập phân / âm / rất lớn cho `formatCount`, và ca ISO thiếu offset cho `formatTimeVn`**
(xem M-1, m-1).

Các test còn lại có tính phân biệt tốt: số liệu cố ý khác nhau từng ô, so ô theo cột thay vì `textContent` cả hàng, lỗi lấy từ service
thật, test 404 của page fail nếu `notFound()` nằm trong `try`, test production no-op fail nếu bỏ dòng `NODE_ENV`.

## 3. Phạm vi diff

Đã đổi/thêm: `lib/ops-service.ts`(+test), `lib/format.ts`(+test), `hooks/use-ops-war-room.ts`(+test), `components/ops/**`,
`app/(ops)/ops/**` (gồm `not-found.tsx` + test), `components/layout/nav-config.ts` (một cờ + comment),
`tests/e2e/specs/ops-war-room.spec.ts`, `docs/workflow/EV-187-ops-war-room/**`. Tất cả nằm trong §3 + §6 của plan.
Không đụng `tests/e2e/playwright.config.ts`, `tests/e2e/fixtures/**`, test layout/nav (đúng I-1).

## 4. Việc cần làm trước merge

1. Sửa M-1 (định dạng số thực) + test.
2. Nên sửa m-1, m-2, m-3, m-6, m-7 (rẻ, chạm đúng AC-11/hydration/bảo mật); còn lại sửa hoặc ghi nợ trong `traceability.md`.
3. Siết t-1; cân nhắc thêm t-2.
4. Chạy E2E trên bản build standalone và ghi vào `evidence.md` (AC-10 chưa được xác minh trong review này).
