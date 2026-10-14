# EV-187 — Thử phá (Challenge)

| Mục | Giá trị |
|---|---|
| Người thử phá | Agent challenger độc lập (context sạch) |
| Đầu vào | `spec.md`, `plan.md` (§6, §7 thay §1), `code-review.md`, diff chưa commit so với `origin/develop` @ `51c4729` |
| Worktree | `D:/Documents/Nam4-1/Project1/EvenFlow-187` (branch `feature/EV-187-ops-war-room`) |
| Ngày | 2026-10-13 |
| Phạm vi tấn công | `lib/ops-service.ts`, `lib/format.ts` (`formatCount`, `formatDecimal`, `formatTimeVn`), `hooks/use-ops-war-room.ts`, `components/ops/*`, `app/(ops)/ops/events/**`, `nav-config.ts` |

## Kết luận: **PASS**

Không tìm được lỗi nào vi phạm AC-1..AC-11 trên bản build production. 66 đột biến (mutation): 56 bị test giết,
10 sống sót (phần lớn là lỗ hổng test, không phải lỗi code). Hydration sạch với 3 múi giờ trình duyệt khác server,
nhịp làm mới thực tế ~2,0s không chồng request, focus nút "Làm mới" giữ nguyên. Các phát hiện dưới đây đều **Minor**
hoặc **Info**: phần lớn chỉ lộ khi nối Ops API thật, nên ghi nợ hoặc sửa rẻ trước merge.

## 1. Phát hiện

### C-1 (Minor) — Request treo thì polling dừng vĩnh viễn, nút "Làm mới" kẹt ở "Đang làm mới…"

Hook không có timeout/AbortController. Một request không bao giờ xong giữ `inflightRef` mãi: timer không được lên lịch lại
(chỉ lên lịch trong `finally`), còn `refresh()` thủ công nhận lại đúng promise treo đó. Banner "Dữ liệu cũ" vẫn hiện
(AC-2 vẫn đạt), nhưng Ops không có cách nào phục hồi ngoài tải lại trang.

- Repro (test tạm, đã xoá): `renderHook(useOpsWarRoom)` với `initialSnapshot`, fetcher lần 1 trả `new Promise(() => {})`,
  các lần sau trả snapshot mới; `advance(2000)`, gọi `refresh()`, `advance(60_000)`.
- Kết quả: `E1 calls after 62s: 1 isRefreshing: true isStale: true` — 62s chỉ có 1 lần gọi fetcher.
- Đề xuất: timeout cho mỗi lần fetch (vd. `AbortController` + ~`STALE_AFTER_MS`), hết hạn thì coi như lỗi thường và lên lịch lại.

### C-2 (Minor, thiết kế) — Tuổi dữ liệu so giờ server với đồng hồ trình duyệt

`ageMs = max(0, Date.now() của client − generatedAt của server)`. Lệch đồng hồ làm sai cả hai chiều:

- Client nhanh hơn server 6s, fetch luôn thành công: `E2 isStale each second (skew +6s): true,true,…,true fetches: 5 error: null` —
  banner "Dữ liệu cũ" hiện vĩnh viễn dù số liệu mới.
- `generatedAt` ở tương lai (+1h) và server trả cùng một snapshot đóng băng: `E3 after 30s … ageMs 0 isStale false` — dữ liệu
  đóng băng không bao giờ bị coi là cũ.
- Với dữ liệu mẫu hiện tại không lộ (cả `generatedAt` lẫn `now` đều là đồng hồ trình duyệt sau lần poll đầu). Lộ ngay khi nối API thật.
- Đề xuất: khi có API thật, server trả kèm `serverNow` (hoặc `ageMs`) để tính độ lệch, hoặc kết hợp với thời điểm client nhận
  (`lastSuccessAt`) để phát hiện snapshot đóng băng.

### C-3 (Minor) — Giá trị lạ từ nguồn thật lọt ra UI

Render `OpsWarRoom` với snapshot có `saleState: "PAUSED"`, `ai.mode: "THROTTLED"`, `budgetUsedRatio: NaN`,
`chatQueueLagSeconds: NaN`, `ratePerSec: 1e21`, `inventory: []` (test tạm, đã xoá):

- `"NaN%"` xuất hiện: `ops-panels.tsx` tính `${Math.round(ai.budgetUsedRatio * 100)}%` trực tiếp, không qua formatter
  (`formatCount`/`formatDecimal` đều trả `"—"` cho NaN).
- Badge trạng thái bán **rỗng** (`SALE_STATE_LABEL["PAUSED"]` là `undefined`) nhưng vẫn hiện "Không có đợt bán đang diễn ra".
- Khối AI: `"Nấc suy biếnTHROTTLED"` — câu tác động người mua (AC-5) biến mất, không có fallback.
- `formatDecimal(1e21)` / `formatCount(1e21)` → `"1e+21"` (do `String`/`toFixed` chuyển sang dạng mũ từ 1e21).
- `inventory: []` → bảng chỉ có header, 0 hàng, không có câu "Chưa có hạng vé".
- TypeScript chặn được các giá trị này ở compile-time, nhưng dữ liệu JSON từ API thì không. Đề xuất: đưa tỉ lệ ngân sách qua
  formatter (vd. `formatCount(ratio * 100)`), fallback nhãn cho enum lạ, hoặc kiểm schema (zod đã có trong deps) khi nối API.

### C-4 (Minor) — BR-O1 im lặng khi số liệu là NaN

`tierInventoryViolation` dùng so sánh `>`/`<`; mọi so sánh với `NaN` là `false`:

- `quota: NaN` → `null`; `available: NaN` → `null` (repro E5 trong test tạm).
- Hàng tồn kho có số hỏng hiện "—" (NaN qua `formatCount`) ở ô và cột Trạng thái "—", không có banner "Lệch tồn kho".
- Đề xuất: số không hữu hạn → coi là vi phạm (hoặc trạng thái riêng "Số liệu không hợp lệ").

### C-5 (Minor) — Nút "Thử lại" vẫn dùng `loading` → `disabled`

Code review m-2 chỉ sửa nút "Làm mới". `ops-war-room.tsx` (màn lỗi lần tải đầu) vẫn là
`<Button variant="primary" loading={isRefreshing} …>Thử lại</Button>`; `components/ui/button.tsx` đặt `disabled={blocked}` khi
`loading`, nên người dùng bàn phím mất focus khi bấm "Thử lại" (cùng cơ chế focus fixup mà m-2 mô tả). Trong test tạm (jsdom),
sau khi bấm: `disabled= true … active tag: BODY`. Màn này không dựng được trên bản production (override bị no-op), nên chỉ
kiểm ở mức unit. Đề xuất: dùng `aria-busy` như nút "Làm mới".

### C-6 (Minor, chất lượng test) — Đột biến sống sót

Mỗi đột biến: sửa đúng một chỗ, chạy `npx vitest run` cho 6 nhóm test liên quan (ops-service, format, hook, components/ops,
app/(ops), components/layout — 454 test), rồi trả file về bản gốc. Các đột biến sau **không** làm test nào đỏ:

| Mã | Đột biến | Hậu quả nếu lọt vào code | Mức |
|---|---|---|---|
| M46 | Bỏ `<StaleMark/>` trong `Metric` | Ô phễu của nguồn `stale` mất nhãn "(cũ)" (AC-3 "ô đó gắn nhãn cũ"); test chỉ tìm "cũ" ở bất kỳ đâu, mà tiêu đề khối khác đã có | Minor |
| M17 | `OpsInventoryPanel`: `checked = true` | Ticketing `stale` thì banner không báo BR-O1 nhưng hàng vẫn gắn "Vượt quota" (trái M-3); chưa có test cho hàng khi `stale` | Minor |
| M47 | Bỏ màn tĩnh cho `initialError` 403 | Client gọi thêm 1 request sau khi server đã trả 403; test lấy số lần gọi **sau** `flush()` nên không thấy | Minor |
| X63 | Không đặt lại `stoppedRef` khi đổi `eventId` | Sau 403, đổi sang sự kiện khác (không có `key`) thì không bao giờ poll lại; page dùng `key={eventId}` nên chưa lộ | Minor |
| M24 | `formatCount`: bỏ `abs !== 0` | `formatCount(-0.5)` → `"-0"`; test chỉ kiểm `formatCount(-0)` (mà `-0 < 0` là `false`) | Minor |
| M28 | `formatTimeVn`: bỏ kiểm `Number.isNaN` | Chuỗi hỏng có đuôi `Z` (vd. `"xZ"`) → `"NaN:NaN:NaN"`; test chuỗi hỏng không có `Z` nên regex đã chặn trước | Minor |
| M26 | Bỏ `+ DAY_MS) % DAY_MS` | Chỉ sai với mốc trước 1970 | Info |
| X59 | Bỏ `await null` trong `run` | Không đổi hành vi quan sát được với fetcher async | Info (gần như tương đương) |
| M52, X61 | `Math.floor` → `round`/`ceil` cho "Tuổi dữ liệu" | Lệch hiển thị tối đa 1s | Info |

Đề xuất thêm test: ô phễu `stale` có "(cũ)" ngay trong ô; hàng tồn kho khi ticketing `stale` không gắn badge; `initialError` 403 →
`mockGet` **không** được gọi lần nào; `formatCount(-0.5) === "0"`; `formatTimeVn("xZ") === "--:--:--"`.

### C-7 (Info) — Dữ liệu mẫu tự mâu thuẫn với sự kiện chưa mở bán

Trên bản build: `/ops/events/evt-grand-concert-2026` hiện badge "Chưa mở bán" + "Không có đợt bán đang diễn ra" nhưng phễu
có `Đang giữ vé 100 · Đã thanh toán 690 · Đã phát hành 580`. Đúng theo bảng số §6 M-4 (holding/paid/issued = tổng theo
`availability`, không phụ thuộc `saleState`) và có nhãn "Dữ liệu mẫu", nên chỉ ghi nhận.

### C-8 (Info) — `formatTimeVn` nhận ngày lịch không tồn tại

`formatTimeVn("2026-02-30T10:00:00Z")` → `"17:00:00"` (V8 cuộn sang 02/03). Không ảnh hưởng hydration (kết quả không phụ thuộc
múi giờ), chỉ là không bắt được ISO sai ngày.

## 2. Các tấn công KHÔNG phá được

| Tấn công | Cách làm | Kết quả |
|---|---|---|
| Hydration với múi giờ trình duyệt khác server | Spec Playwright tạm, `timezoneId` `America/New_York`, `Pacific/Kiritimati` (UTC+14), `UTC`, `locale: en-US`; bản build standalone | 0 lỗi console/pageerror; "Cập nhật lúc" hiển thị đúng giờ VN ở cả 3 múi giờ |
| `formatTimeVn` phụ thuộc múi giờ máy | Chạy 22 chuỗi (ISO `Z`, `±HH:MM`, `z` thường, legacy `GMT+07:00`, trước 1970, năm 275760, giờ 25…) dưới `process.env.TZ` = UTC, New_York, Ho_Chi_Minh, Kiritimati, Lord_Howe | Kết quả giống hệt nhau ở mọi múi giờ; nửa đêm `16:59:59Z`→`23:59:59`, `17:00:00Z`→`00:00:00`; thiếu offset → `--:--:--` |
| `formatDecimal` làm tròn / `-0` | `-0.04`→`0,0`, `-1e-7`→`0,0`, `999.95`→`1.000,0`, `1234.56`→`1.234,6`, `-0.05`→`-0,1` | Đúng hợp đồng §7 (sai số nhị phân của `toFixed` như `0.35`→`0,3` là chấp nhận được) |
| Nhịp polling thực tế | Mở war room 15s, `MutationObserver` ghi thời điểm "Cập nhật lúc" đổi | 7 lần cập nhật, khoảng cách `2001, 2002, 2011, 2004, 2013, 2010` ms; không lần nào hiện "Đang làm mới…" hay "Dữ liệu cũ" |
| Focus nút "Làm mới" | Focus nút, nhấn Enter 10 lần liên tiếp | Focus vẫn ở `BUTTON "Làm mới"`, `disabled=false`, không lỗi; Tab tiếp theo đi đúng thứ tự (nút là phần tử focus được cuối cùng) |
| Mount/unmount liên tục | Điều hướng danh sách ↔ war room 5 lần | 0 lỗi console, không cảnh báo setState sau unmount |
| 404 thật | `GET /ops/events/khong-ton-tai` | HTTP 404, heading "Không tìm thấy sự kiện" (chỉ có log trình duyệt "Failed to load resource 404" của chính document) |
| Sự kiện không `ON_SALE` | Mở sự kiện "Chưa mở bán" | Có "Không có đợt bán đang diễn ra", vẫn hiện số liệu; queued/admitted = 0 (đúng M-4) |
| 56 đột biến bị giết | Ngưỡng cũ 5000→6000 và `>`→`>=`; bỏ dừng khi 403, 403→401; bỏ dedupe in-flight, dedupe bỏ qua `eventId`, `isCurrent` bỏ qua `eventId`; BR-O1 `>`→`>=`, bỏ `available<0`, `<0`→`<=0`, đảo ưu tiên; đổi nguồn của lobby/admitted/holding/issued, khối tuyển sinh, AI, tồn kho; banner BR-O1 bỏ kiểm ticketing; `aria-busy`→`loading`, bỏ `aria-busy`, nút "Làm mới"/"Thử lại" không làm gì; `formatDecimal` `-0`, digits 2, bỏ ngăn nghìn, cắt thay vì làm tròn; `formatTimeVn` offset 8h, bỏ regex offset; bỏ `max(0)`; bỏ `isRefreshing` khi dedupe; timer tuổi chạy sau 403; `initialSnapshot` vẫn fetch ngay; không lên lịch lại; page lộ message thô; bỏ chặn production của override; thành công không xoá lỗi; nút điều khiển bật; 403 giữa chừng vẫn hiện dashboard; stale bỏ qua lỗi; unmount không `clearTimeout` / không đặt `mountedRef=false`; tick 1s→3s; nav `ready:false`; ẩn "Ops đặt tay", ẩn "Dữ liệu mẫu", ẩn "Không có đợt bán…"; nguồn `unavailable` vẫn hiện số; nuốt `notFound()`; chu kỳ 2s→3s; ngân sách ×10; `generatedAt` hỏng → 0; bỏ `lastSuccessAt`; bỏ `force-dynamic`; link danh sách sai | Mỗi đột biến làm 1–14 test đỏ |

## 3. E2E (bản build production, standalone)

- `npm run build` trong `apps/web`: thành công (`/ops/events`, `/ops/events/[eventId]` là `ƒ` dynamic).
- Server: `.next/standalone/apps/web/server.js` (đã copy `.next/static`, `public`), `HOSTNAME=127.0.0.1 PORT=3000`, `/api/healthz` → 200.
- `BASE_URL=http://localhost:3000 npx playwright test specs/ops-war-room.spec.ts --retries=0` → **1 passed** (1,1s).
- `… --repeat-each=20` → **20 passed** (9,4s).
- Spec tạm `specs/zz-challenge.spec.ts` (8 test: 3 múi giờ, nhịp 15s, focus, 404, điều hướng, không `ON_SALE`) → 8 passed; đã xoá.
- Sau khi chạy: chỉ kill PID của server ở `127.0.0.1:3000`, xoá `tests/e2e/test-results`; không sinh `package-lock.json`.

Ghi chú: bước "Làm mới" của E2E xong trong ~1s nhưng poll tự động 2s cũng làm "Cập nhật lúc" đổi, nên E2E không cô lập được
tác dụng của nút (đã ghi ở code review t-4). Unit test thì có cô lập: đột biến X60 (nút không làm gì) làm 2 test đỏ.

## 4. Khôi phục cây làm việc

Mọi đột biến được trả về bản gốc ngay sau mỗi lần chạy. Test tạm (`src/hooks/zz-challenge-edge.test.tsx`,
`tests/e2e/specs/zz-challenge.spec.ts`, `zz-focus.spec.ts`) đã xoá. Kiểm cuối: md5 của `git diff` = `9921f1b74cb984e4321f91c42625bfd4`
(giống lúc bắt đầu), md5 của toàn bộ 24 file chưa commit khớp ảnh chụp ban đầu, `git status` chỉ thêm file này.
