# EV-187 — Review kế hoạch (Plan review)

| Mục | Giá trị |
|---|---|
| Người review | Agent review độc lập (context sạch) |
| Đầu vào | `spec.md`, `plan.md` (cùng thư mục) |
| Base đối chiếu | `develop` @ `51c4729` (worktree `EvenFlow-187`) |
| Ngày | 2026-10-13 |

## Kết luận: **PASS WITH CHANGES**

Hướng đi đúng (bám mẫu EV-186, có nhãn dữ liệu mẫu, tách khu "Điều khiển", không đụng file của EV-198).
Không có blocker. Có **6 major** cần sửa trong `plan.md` **trước khi** viết test, vì chúng là chỗ hợp đồng
còn mơ hồ: test designer và implementer sẽ hiểu khác nhau, hoặc sẽ chép lại lỗi hydration đang có ở mẫu EV-186.

## 1. Độ phủ AC

| AC | Plan phủ ở đâu | Đánh giá |
|---|---|---|
| AC-1 | §1.3 `ops-war-room.tsx` | Đủ. Thiếu "cửa sổ thời gian" theo handoff §10.18 (xem m-7) |
| AC-2 | §1.2 hook (`ageMs`, `isStale`, 2s) | Phủ logic; **thiếu cách render thời gian an toàn với SSR** (M-1), lệch đồng hồ (M-6) |
| AC-3 | §1.3 banner | **Thiếu bảng ánh xạ nguồn → ô số liệu** (M-3) |
| AC-4 | §1.1 `tierInventoryViolation` | Đủ; nên chốt thứ tự ưu tiên khi vi phạm cả hai (m-6) |
| AC-5 | §1.3 | Thiếu câu mô tả cố định cho 4 nấc (m-5) |
| AC-6 | §1.3 khu "Điều khiển" | Đủ |
| AC-7 | §1.3 + §1.4 (`notFound()`) | **Server component chỉ xử lý 404; 403 / lỗi khác ở lần tải đầu chưa có đường đi** (M-2) |
| AC-8 | §1.4 nav-config | Đủ; test hiện có không cần đổi (xem I-1) |
| AC-9 | §1.2 | Phủ ý chính; **`refresh()` khi đang in-flight và thời điểm fetch đầu chưa chốt** (M-6) |
| AC-10 | §1.5 + §2 bước 3 | **E2E hiện chỉ kiểm HTML SSR, không chứng minh client đã hydrate** (M-5) |
| AC-11 | §1.3 gián tiếp | Thiếu quy tắc live region (m-8) |

## 2. Phát hiện

### Major

**M-1 — Render phụ thuộc thời gian sẽ gây hydration mismatch (AC-2).**
Bằng chứng: mẫu EV-186 được plan chỉ định làm theo có đúng lỗi này —
`apps/web/src/hooks/use-organizer-sales.ts:49-51` khởi tạo `lastUpdatedAt` bằng `new Date()` trong `useState`,
`apps/web/src/components/organizer/sales-dashboard.tsx:110` render `lastUpdatedAt.toLocaleTimeString("vi-VN")`.
`OpsWarRoom` là client component được SSR với `initialSnapshot`; nếu `ageMs = now() - generatedAt` tính lúc render
và "cập nhật lúc …" format bằng `toLocaleTimeString`, thì: (a) server (container standalone, thường TZ=UTC) và trình
duyệt (UTC+7) cho chuỗi giờ khác nhau **chắc chắn**; (b) `ageMs`/`isStale` khác nhau nếu hydrate chậm qua mốc giây/5s.
Lỗi không lộ khi dev local cùng múi giờ, nên unit test và E2E local đều xanh. `lib/format.ts:1` cũng đã đặt nguyên tắc
"không phụ thuộc ICU/locale".
Sửa plan: thêm vào §1.2/§1.3:
- Hook: `ageMs` khởi tạo `null` (hoặc 0 với `isStale=false`) và chỉ tính sau mount trong `useEffect`; kiểu trả về
  `ageMs: number | null`. Không gọi `now()` trong thân render.
- "cập nhật lúc HH:mm:ss" format bằng hàm tất định với múi giờ cố định `Asia/Ho_Chi_Minh` (cộng +7h vào UTC rồi pad,
  không dùng ICU), hoặc chỉ render sau mount. Không dùng `toLocaleTimeString`.
- Test: render component với `now` khác nhau giữa 2 lần render không đổi markup ban đầu; E2E bắt `console` error
  "Hydration"/"did not match" (xem M-5).

**M-2 — Server component chưa có đường cho 403 / lỗi khác ở lần tải đầu; lỗi không serialize được (AC-7).**
Bằng chứng: plan §1.4 chỉ nói "404 → `notFound()`". `OpsServiceError` là class instance — không truyền được từ server
component sang client component qua props. Ngoài ra repo **không có** `not-found.tsx` nào (`find src/app -name "not-found*"`
rỗng), nên `notFound()` hiện trang 404 mặc định tiếng Anh của Next, **không** có chữ "Không tìm thấy sự kiện" mà AC-7 yêu cầu.
Sửa plan:
- §1.4: `try { getOpsWarRoom } catch (e)`: `status === 404` → `notFound()`; `403` → render trạng thái "Không có quyền"
  (trực tiếp hoặc qua `OpsWarRoom` với `initialError`); lỗi khác → render `OpsWarRoom` với `initialSnapshot = null` và
  `initialError: { status: number; message: string }` (object thuần) để client hiện thông báo + nút "Thử lại".
- Thêm `app/(ops)/ops/events/[eventId]/not-found.tsx` có heading "Không tìm thấy sự kiện" (đã nằm trong `app/(ops)/ops/**`).
- Test trang: mock `next/navigation` `notFound` ném lỗi như mẫu sẵn có
  `apps/web/src/app/(checkout)/checkout/[eventId]/page.test.tsx:30` và `(marketing)/events/[slug]/page.test.tsx:12`;
  thêm test 403 và lỗi 500 bằng `__setOpsSampleOverride(id, { error: 403 })`; test riêng `not-found.tsx`.

**M-3 — Chưa chốt ô nào thuộc nguồn nào (AC-3 không kiểm được).**
Bằng chứng: spec §2 có `sources: { waitingroom, ticketing, ai }`, AC-3 nói "các ô thuộc nguồn đó", nhưng spec/plan không
liệt kê ánh xạ. Test designer phải tự đoán (ví dụ `holding` thuộc waitingroom hay ticketing?).
Sửa plan §1.1: thêm bảng chốt, ví dụ:
`waitingroom` → `funnel.lobby/queued/admitted`, `admission.*`; `ticketing` → `funnel.holding/paid/issued`, `inventory[]`
và cảnh báo lệch tồn kho; `ai` → `ai.*`. Ghi rõ: nguồn `unavailable` thì **không** đánh giá BR-O1 trên số của nguồn đó
(tránh cảnh báo lệch tồn kho từ số không tin được), và không có chỉ báo "bình thường" nào khi có nguồn ≠ `ok`.

**M-4 — `getAllPublicEvents()` không có quota; trạng thái sự kiện không khớp kiểu `saleState`.**
Bằng chứng: `apps/web/src/lib/event-service.ts:1-5,11-17` — tier công khai **cố ý** không có `quota` (BR-A4), chỉ có
`availability`. `EventPublicState` (`components/event/event-card.tsx:13-19`) có thêm `COMPLETED`, `CANCELLED`, trong khi
`saleState` chỉ có 4 giá trị → typecheck hỏng nếu gán thẳng. Plan §1.1 nói "lấy id/tên/hạng vé, không chép tay" nhưng
số quota/held/paid/issued thì vẫn phải bịa ở đâu đó mà chưa nói cách.
Sửa plan §1.1:
- Chốt công thức/bảng tất định cho từng `tierId` (quota, held, paid, issued), nhất quán với `availability`
  (`SOLD_OUT` → `available = 0`, `FEW_LEFT` → `available` nhỏ), và **mặc định thoả BR-O1** (để E2E không thấy "lệch tồn kho").
  Cân nhắc lấy quota từ `INITIAL_ORGANIZER_EVENTS` (`lib/organizer-service.ts:274,281,316,323,330`) để Ops và Organizer
  không hiện hai bộ số khác nhau cho cùng sự kiện; `evt-indie-acoustic-night` không có ở organizer → cần bảng riêng.
- Chốt ánh xạ trạng thái: `COMPLETED`/`CANCELLED` → `CLOSED` (hoặc loại khỏi `listOpsEvents`).
- `saleState` lấy từ `state` tĩnh, **không** suy ra từ `Date.now()` so với `saleStartAt`, để E2E tất định
  (`evt-tech-summit-2026` luôn `ON_SALE`).

**M-5 — E2E chưa chứng minh trang thực sự chạy phía client; cách chạy standalone chưa ghi.**
Bằng chứng: các assert trong plan §1.5 ("Dữ liệu mẫu", 6 bước phễu, bảng, nút disabled) đều có sẵn trong HTML SSR — E2E vẫn xanh
nếu JS client 404 (ví dụ quên copy `.next/static` khi chạy `.next/standalone/server.js`, `next.config.mjs:10`
`output: "standalone"`) hoặc client crash do hydration. Trên `develop`, `tests/e2e/playwright.config.ts` không có `webServer`,
chỉ đọc `BASE_URL` (mặc định `:3000`); `e2e-nightly.yml` chạy **toàn bộ** specs với web trong docker-compose.
Sửa plan §1.5 và §2 bước 3:
- E2E thêm một bước cần hydrate: bấm "Làm mới" rồi chờ `isRefreshing` kết thúc / "cập nhật lúc" xuất hiện hoặc đổi
  (dùng `expect.poll`/`toHaveText`, không `waitForTimeout`); đăng ký `page.on("console")` và fail nếu có lỗi hydration.
- Ghi lệnh chạy cụ thể vào `evidence.md`: `npm run build`, copy `.next/static` (và `public`) vào thư mục standalone,
  `PORT=3000 node <đường dẫn>/server.js` (xác nhận đường dẫn thực tế — repo có `package-lock.json` ở gốc nên Next có thể
  lồng output thành `.next/standalone/apps/web/server.js`).
- Rủi ro: khi thay dữ liệu mẫu bằng Ops API thật, assert "Dữ liệu mẫu" sẽ gãy (và nightly sẽ đỏ) — ghi vào §4.

**M-6 — Hợp đồng hook còn mơ hồ ở các điểm test bằng fake timer sẽ phụ thuộc (AC-2, AC-9).**
Bằng chứng: plan §1.2 không nói (a) có `initialSnapshot` thì fetch đầu chạy ngay hay sau `intervalMs`
(mẫu EV-186 fetch ngay khi mount dù có `initialData` — `use-organizer-sales.ts:88-95`); (b) `refresh()` khi đang
in-flight làm gì; (c) `refresh()` có đặt lại lịch `setTimeout` không; (d) lỗi 403 giữa chừng (đã có snapshot) hiển thị gì;
(e) `ageMs` âm do lệch đồng hồ server/client (`generatedAt` từ server, `now` từ trình duyệt) — lệch > 5s sẽ làm banner
"dữ liệu cũ" bật vĩnh viễn hoặc không bao giờ bật.
Sửa plan §1.2, chốt từng câu:
- Có `initialSnapshot` → fetch đầu sau `intervalMs`; không có → fetch ngay khi mount.
- `refresh()` khi đang in-flight → **không** gọi `fetcher` lần nữa, trả về cùng promise; sau khi xong thì lên lịch lại
  từ đầu (một timer duy nhất).
- Lỗi khi đã có snapshot → giữ snapshot, `error` được set, `isStale = true`; 403 giữa chừng vẫn giữ số liệu và hiện
  banner quyền (hoặc chốt ngược lại — miễn là ghi ra).
- `ageMs = max(0, now - Date.parse(generatedAt))`; ghi nợ lệch đồng hồ cho khi có API thật.
- Đổi `eventId` → reset state (hoặc trang render `<OpsWarRoom key={eventId}>`).
- Ghi chú test: timer tự lên lịch sau `await` nên dùng `vi.advanceTimersByTimeAsync`, và hook phải an toàn với
  `reactStrictMode: true` (`next.config.mjs:12`) — effect chạy hai lần trong dev không được tạo hai vòng poll.

### Minor

**m-1 — Định dạng số chưa có hàm; `lib/format.ts` không nằm trong danh sách file được đổi.**
Bằng chứng: `lib/format.ts` chỉ có `formatClock`, `formatVnd` (gắn hậu tố "đ"), `spellDuration` (dòng 10, 26, 39) và nguyên tắc
"không phụ thuộc ICU" (dòng 1). Không có hàm nhóm nghìn cho số đếm, % hay "khách/giây".
Sửa: thêm `formatCount` (và `formatPercent`, `formatRate` nếu cần) vào `lib/format.ts` (+ test) — bổ sung vào §3 file được phép;
hoặc ghi rõ hàm cục bộ trong `components/ops/`. Không dùng `toLocaleString("vi-VN")` như `sales-dashboard.tsx:169-231`.

**m-2 — Comment trong `nav-config.ts` sẽ sai sau khi bật `ready: true`.**
Bằng chứng: `components/layout/nav-config.ts:8` ("ngoài trang chủ, chưa route nào tồn tại") và `:30-32`
("Giữ `ready: false` cho đến khi Product chốt"). Sửa: cập nhật comment cùng commit, ghi rằng EV-187 tạo `/ops/events` làm
trang chọn sự kiện (handoff §10.18 yêu cầu "event selector").

**m-3 — `/ops/events` sẽ bị prerender tĩnh lúc build.**
Plan chỉ đặt `force-dynamic` cho trang `[eventId]`. Trang danh sách không có params → Next prerender lúc build; với dữ liệu mẫu
thì vô hại, nhưng khi có API thật thì danh sách đông cứng. Sửa: đặt `dynamic = "force-dynamic"` cho cả hai, có test khẳng định
giá trị export.

**m-4 — `__setOpsSampleOverride` là trạng thái module dùng chung, xuất ra trong code production.**
Trên server, state module dùng chung giữa các request; ở client là bản sao riêng (E2E không dùng được). Sửa: ghi rõ chỉ dùng
trong unit test, test phải `afterEach(() => __setOpsSampleOverride(id, null))`, và cân nhắc no-op khi
`process.env.NODE_ENV === "production"`.

**m-5 — Chưa chốt câu mô tả cho 4 nấc AI (AC-5).** Sửa: thêm bảng `NORMAL/SAVING/FAQ_ONLY/OFF` → câu tiếng Việt cố định
vào §1.3 để test so khớp chính xác.

**m-6 — `tierInventoryViolation` chưa chốt thứ tự khi vi phạm cả hai.** Sửa: `OVER_QUOTA` ưu tiên trước
`NEGATIVE_AVAILABLE`; chốt các tầng là rời nhau theo BR-O1 (`docs/01-nghiep-vu.md:84`).

**m-7 — Handoff §10.18 yêu cầu "unit, time window, and freshness" cho mỗi chỉ số.**
Bằng chứng: `docs/08-stitch-ui-ux-handoff.md:437`. AC-1 chỉ nói đơn vị. Sửa: ghi rõ phễu là số hiện tại hay luỹ kế từ khi mở
bán, admit rate là trung bình bao nhiêu giây; hiển thị nhãn đó. Ngoài ra khi `isSample` thì "cập nhật lúc …" chỉ là giờ chụp
mẫu — nên đặt cạnh nhãn "Dữ liệu mẫu" để không trông như số sống.

**m-8 — Quy tắc live region chưa có (AC-11).** Sửa: không đặt `aria-live` trên vùng số liệu (đọc lại mỗi 2s); chỉ banner
chuyển trạng thái mới là `role="alert"`/`"status"` (`components/ui/alert.tsx:32-34` tự chọn role theo variant: warning/error → alert,
info → status). Nhãn "Dữ liệu mẫu" dùng variant `info` hoặc `Badge`, không dùng `alert`. Trang có `<h1>` là tên sự kiện.

**m-9 — Fetcher mặc định chạy mock ngay trong trình duyệt.** Client component → hook → `getOpsWarRoom` nên các lần poll chạy
mock phía client, `generatedAt` là giờ trình duyệt; spec AC-10 nói "dữ liệu mẫu chạy trong server component" chỉ đúng với lần tải
đầu. Khi có API thật, thân hàm chạy ở cả hai phía (khác base URL/auth). Sửa: ghi chú trong §1.1 (giống EV-186), không cần đổi thiết kế.

### Thông tin (không cần sửa)

**I-1 — Test nav/layout không cần đổi khi bật `ready: true`.**
`app/(ops)/layout.test.tsx:142-156` lặp theo dữ liệu `OPS_NAV` (ready:false → không có `<a>`, ready:true → có `<a>`), nên tự
đúng. `components/layout/nav-config.test.ts:82-86` cần "ít nhất một item `ready: false`" — vẫn xanh vì `/ops/ai` còn false.
`workspace-shell.test.tsx` dùng danh sách nav riêng. Plan nên ghi "dự kiến không sửa test layout/nav" thay vì để ngỏ; nếu
Implementer thấy cần sửa thì là tín hiệu sai.

**I-2 — Phạm vi:** không thấy scope creep. `lastSuccessAt`, tách file component là hợp lý. `listOpsEvents` cần cho AC-8.

## 3. Danh sách file được phép — cần bổ sung

- `apps/web/src/lib/format.ts` + `format.test.ts` (nếu chọn m-1 hướng thêm hàm chung).
- Ghi rõ `app/(ops)/ops/events/[eventId]/not-found.tsx` (+ test) — đã nằm trong glob nhưng nên liệt kê (M-2).
- Bỏ "+ `app/(ops)/layout.test.tsx` nếu kỳ vọng `ready:false` cần đổi" hoặc đổi thành "không dự kiến sửa" (I-1).
- Không được đụng `tests/e2e/playwright.config.ts` (EV-198 đang sửa) — ghi thẳng vào §3.

## 4. Rủi ro cần thêm vào §4 của plan

| Rủi ro | Giảm thiểu |
|---|---|
| Hydration mismatch do giờ/tuổi dữ liệu (M-1) | Tính sau mount, format tất định theo `Asia/Ho_Chi_Minh`; E2E bắt console error |
| E2E xanh giả vì chỉ kiểm SSR (M-5) | Bước cần hydrate (bấm "Làm mới"), ghi lệnh chạy standalone vào evidence |
| Lệch đồng hồ server/client làm "dữ liệu cũ" sai (M-6) | Kẹp `ageMs >= 0`; ghi nợ khi có API thật |
| E2E nightly gãy khi thay dữ liệu mẫu bằng API thật (M-5) | Ghi nợ; assert nhãn mẫu có điều kiện hoặc cập nhật cùng PR thay API |
| Ops và Organizer hiện hai bộ số khác nhau cho cùng sự kiện (M-4) | Dùng chung quota nguồn từ organizer mock |
