# EV-187 — Kế hoạch thực thi (Plan)

Đầu vào: `spec.md` (AC-1..AC-11). Base: `develop` @ `51c4729`. Worktree `D:/Documents/Nam4-1/Project1/EvenFlow-187`.
Mẫu làm theo: EV-186 (`app/(organizer)/organizer/events/[eventId]/sales/page.tsx`, `hooks/use-organizer-sales.ts`,
`components/organizer/sales-dashboard.tsx`, `lib/organizer-service.ts`).

## 1. Hợp đồng chốt trước (test viết theo các hợp đồng này)

### 1.1 `apps/web/src/lib/ops-service.ts` (mới)

Như `spec.md` §2, thêm:

```ts
/** Kiểm tra bất biến BR-O1 cho một hạng vé. */
export function tierInventoryViolation(t: OpsTierInventory): "OVER_QUOTA" | "NEGATIVE_AVAILABLE" | null;
/** Tuổi dữ liệu (ms) = nowMs - Date.parse(generatedAt); generatedAt hỏng => Infinity. */
export function snapshotAgeMs(snapshot: Pick<OpsWarRoomSnapshot, "generatedAt">, nowMs: number): number;
export const STALE_AFTER_MS = 5_000;
export const OPS_POLL_INTERVAL_MS = 2_000;
/** Chỉ dùng trong test: đặt lại kho dữ liệu mẫu / ép lỗi. */
export function __setOpsSampleOverride(eventId: string, patch: Partial<OpsWarRoomSnapshot> | { error: number } | null): void;
```

- Dữ liệu mẫu cho các sự kiện trong `lib/event-service.ts` (dùng `getAllPublicEvents()` để lấy id/tên/hạng vé,
  không chép tay). `isSample: true`. `generatedAt = new Date().toISOString()` mỗi lần gọi. Số liệu tất định
  (không `Math.random`) để test và E2E ổn định.
- Sự kiện không tồn tại → `OpsServiceError(404)`.

### 1.2 `apps/web/src/hooks/use-ops-war-room.ts` (mới)

```ts
export interface UseOpsWarRoomOptions {
  eventId: string; initialSnapshot?: OpsWarRoomSnapshot | null;
  intervalMs?: number;            // mặc định OPS_POLL_INTERVAL_MS
  fetcher?: (eventId: string) => Promise<OpsWarRoomSnapshot>;   // mặc định getOpsWarRoom
  now?: () => number;             // mặc định Date.now
}
export interface UseOpsWarRoomReturn {
  snapshot: OpsWarRoomSnapshot | null; error: OpsServiceError | Error | null;
  isRefreshing: boolean; lastSuccessAt: number | null;
  ageMs: number;                  // tuổi dữ liệu theo snapshot.generatedAt, cập nhật mỗi giây
  isStale: boolean;               // ageMs > STALE_AFTER_MS || lần làm mới gần nhất lỗi
  refresh: () => Promise<void>;
}
```

- Một timer `setTimeout` tự lên lịch lại sau khi request xong (không chồng request, AC-9); một timer 1s cập nhật
  `ageMs`. Dọn cả hai khi unmount. Lỗi giữ snapshot cũ.

### 1.3 Component `apps/web/src/components/ops/` (mới)

- `ops-war-room.tsx` — `OpsWarRoom({ eventId, initialSnapshot })` client component: banner (dữ liệu mẫu,
  dữ liệu cũ, không có đợt bán, lệch tồn kho), khu "Giám sát" (phễu, tuyển sinh, tồn kho, AI) và khu "Điều khiển"
  (nút vô hiệu, AC-6). Trạng thái lỗi 403/404/lỗi khác (AC-7).
- Có thể tách `ops-funnel.tsx`, `ops-inventory-table.tsx`, `ops-ai-panel.tsx` nếu file quá dài; `index.ts` re-export.
- Dùng `components/ui` (Card, Badge, Alert, Button, Spinner) và `lib/format.ts`. Không thêm thư viện.
- `data-testid` chỉ khi không có role/label phù hợp.

### 1.4 Route

- `app/(ops)/ops/events/page.tsx` — server component, `listOpsEvents()`, danh sách link.
- `app/(ops)/ops/events/[eventId]/page.tsx` — server component, `getOpsWarRoom` lần đầu (404 → `notFound()`),
  render `OpsWarRoom` với `initialSnapshot`. `dynamic = "force-dynamic"` (số liệu sống, không prerender).
- `components/layout/nav-config.ts`: mục War room (`/ops/events`) `ready: true`.

### 1.5 E2E

`tests/e2e/specs/ops-war-room.spec.ts`: `/ops/events` → click sự kiện ON_SALE → thấy "Dữ liệu mẫu", 6 bước phễu,
bảng tồn kho, nút điều khiển bị vô hiệu. Không giả lập API (dữ liệu mẫu ở server). Không dùng `waitForTimeout`.

## 2. Thứ tự

1. Test design (agent riêng): unit test cho `ops-service`, hook, component; test trang; cập nhật test nav/layout nếu
   cần; E2E; `traceability.md`, `baseline.md`.
2. Implementation theo §1. Không sửa test; test sai → báo.
3. Gates: lint, typecheck, test, build; E2E (build production, standalone server).
4. Code review, challenge, verify → `code-review.md`, `challenge.md`, `evidence.md`.
5. Docs: `traceability.md`, `docs/kiem-thu/README.md` (§3 bảng số test, §5 dòng truy vết).

## 3. File được phép đổi

`apps/web/src/lib/ops-service.ts`(+test), `apps/web/src/hooks/use-ops-war-room.ts`(+test),
`apps/web/src/components/ops/**`, `apps/web/src/app/(ops)/ops/**`, `apps/web/src/components/layout/nav-config.ts`
(+ test của nó và `app/(ops)/layout.test.tsx` nếu kỳ vọng `ready:false` cần đổi), `tests/e2e/specs/ops-war-room.spec.ts`,
`docs/workflow/EV-187-ops-war-room/**`, `docs/kiem-thu/README.md`.

## 4. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Dữ liệu mẫu trông như số liệu sống | Nhãn "Dữ liệu mẫu" bắt buộc (AC-3), số tất định, không animate |
| Timer polling rò rỉ / chồng request | `setTimeout` tự lên lịch, cờ in-flight, dọn khi unmount; test bằng fake timer |
| Đụng `nav-config.ts` dùng chung | Chỉ đổi đúng một cờ `ready`; chạy toàn bộ test layout |
| Xung đột với EV-198 (đang mở PR) | Không đụng file của EV-198; `docs/kiem-thu/README.md` sẽ rebase khi merge |
| E2E chạy chung workflow `e2e-web.yml` của EV-198 | Spec mới không gọi `/v1/**`; nếu EV-198 chưa merge thì E2E chạy cục bộ, ghi vào evidence |

## 5. Rollback

Revert merge commit. Không có migration, không đổi dữ liệu.

## 6. Sửa sau plan review vòng 1 (`plan-review.md`: PASS WITH CHANGES)

| Finding | Quyết định (bổ sung vào hợp đồng) |
|---|---|
| M-1 hydration | `ageMs: number \| null` — `null` cho tới khi mount (server và lần render đầu của client giống nhau). Giờ hiển thị bằng hàm mới `formatTimeVn(iso: string): string` trong `lib/format.ts` → `"HH:MM:SS"` theo **UTC+7 tính tay** (không `toLocale*`, không ICU); ISO hỏng → `"--:--:--"`. E2E fail khi console có lỗi hydration. |
| M-2 lỗi lần tải đầu | Page `try/catch`: 404 → `notFound()`; lỗi khác → render `OpsWarRoom` với `initialError: { status: number; message: string }` (object thường, không truyền class). Thêm `app/(ops)/ops/events/[eventId]/not-found.tsx` hiện "Không tìm thấy sự kiện" + link về `/ops/events`. Test page mock `notFound` như `(checkout)/checkout/[eventId]/page.test.tsx:30`. Prop mới: `OpsWarRoom({ eventId, initialSnapshot, initialError })`. |
| M-3 ô ↔ nguồn | `waitingroom` → phễu lobby/queued/admitted + khối tuyển sinh (admit rate, mode, queue depth). `ticketing` → phễu holding/paid/issued + bảng tồn kho. `ai` → khối AI. Nguồn `unavailable` → mọi ô của nó hiện "Không có dữ liệu"; `stale` → nhãn "cũ". `ticketing` không `ok` thì **không** kiểm/không báo BR-O1. |
| M-4 số liệu mẫu | Không lấy quota từ dữ liệu công khai. Bảng tất định theo `availability` của hạng vé: `AVAILABLE` → quota 1000, held 40, paid 300, issued 250 (available 410); `FEW_LEFT` → quota 200, held 20, paid 90, issued 80 (available 10); `SOLD_OUT` → quota 500, held 0, paid 0, issued 500 (available 0). Phễu cho sự kiện `ON_SALE`: lobby 0, queued 4210, admitted 350, holding = Σheld, paid = Σpaid, issued = Σissued; admission `{ ratePerSec: 50, mode: "AUTO", queueDepth: 4210 }`; ai `{ mode: "NORMAL", manualOverride: false, chatQueueLagSeconds: 2, budgetUsedRatio: 0.35 }`; sources đều `ok`. Sự kiện không `ON_SALE`: queued/admitted/queueDepth/ratePerSec = 0. Map trạng thái công khai: `ON_SALE`→`ON_SALE`, `SOLD_OUT`→`SOLD_OUT`, trạng thái chưa mở bán→`SCHEDULED`, còn lại (`COMPLETED`, `CANCELLED`...)→`CLOSED`. Không dựa vào `Date.now` để suy trạng thái. |
| M-5 E2E chỉ thấy HTML server | E2E bấm "Làm mới" và chờ dấu hiệu cập nhật từ client (vd. nút hết trạng thái đang làm mới / "cập nhật lúc" được render bởi client); fail nếu console có lỗi (hydration, pageerror). Lệnh chạy standalone ghi vào evidence. |
| M-6 ngữ nghĩa hook | Có `initialSnapshot` → lần fetch đầu sau `intervalMs`; không có → fetch ngay khi mount. `refresh()` khi đang có request → trả **cùng** promise, không gọi fetcher lần hai. Sau mỗi lần fetch (tự động hay thủ công) lịch tiếp theo = `intervalMs` tính từ lúc xong. Lỗi 403 giữa chừng → `error` = lỗi 403, **dừng polling**, UI chuyển sang màn "Không có quyền". Lỗi khác → giữ snapshot, `isStale = true`, vẫn poll. `ageMs = max(0, now - generatedAt)`. Đổi `eventId`: page render `<OpsWarRoom key={eventId} …/>`, hook không cần tự reset. An toàn với StrictMode (effect chạy hai lần không tạo hai timer). Test dùng `advanceTimersByTimeAsync`. |
| m-1 | Thêm `formatCount(n: number): string` vào `lib/format.ts` (dấu chấm ngăn nghìn, tính tay, không `toLocaleString`; không hữu hạn → `"—"`) và `formatTimeVn`; test trong `lib/format.test.ts`. |
| m-2 | Sửa comment `nav-config.ts:8`, `:30-32` cho khớp. |
| m-3 | `export const dynamic = "force-dynamic"` ở cả `/ops/events` và `/ops/events/[eventId]`; test kiểm giá trị export. |
| m-4 | `__setOpsSampleOverride` là no-op khi `process.env.NODE_ENV === "production"`; test reset trong `afterEach`. |
| m-5 | Câu tác động (khớp nguyên văn): NORMAL "Trợ lý trả lời đầy đủ."; SAVING "Trợ lý trả lời ngắn gọn hơn để tiết kiệm tài nguyên."; FAQ_ONLY "Trợ lý chỉ trả lời các câu hỏi thường gặp."; OFF "Trợ lý tạm tắt, người mua được hướng dẫn liên hệ CSKH."; `manualOverride` → thêm "Ops đặt tay". |
| m-6 | Cả hai vi phạm → trả `OVER_QUOTA`. |
| m-7 | Mỗi khối có dòng phụ "Số liệu tức thời · làm mới mỗi 2 giây"; "cập nhật lúc …" đặt cạnh nhãn "Dữ liệu mẫu". |
| m-8 | Trang có `h1` = tên sự kiện. Banner dữ liệu cũ và lệch tồn kho: `Alert` (role `alert`); nhãn dữ liệu mẫu: role `status`. Không `aria-live` trên vùng số liệu. |
| m-9 | Ghi chú: lần tải đầu chạy mock ở server, polling chạy mock trong trình duyệt — chấp nhận vì là dữ liệu mẫu. |
| I-1 | Không dự kiến sửa test nav/layout. |
| File được phép thêm | `apps/web/src/lib/format.ts` (+ `format.test.ts`), `app/(ops)/ops/events/[eventId]/not-found.tsx`. **Không** đụng `tests/e2e/playwright.config.ts`, `tests/e2e/fixtures/**` (thuộc EV-198). |

## 7. Sửa sau code review vòng 1 (`code-review.md`: PASS, 1 major)

| Finding | Quyết định |
|---|---|
| M-1 | Thêm `formatDecimal(n: number, digits = 1): string` vào `lib/format.ts`: dấu **chấm** ngăn nghìn, dấu **phẩy** thập phân, làm tròn `digits` chữ số (`12.7` → `"12,7"`, `1234.56` → `"1.234,6"`, `5` → `"5,0"`), `-0`/kết quả làm tròn về 0 → `"0,0"`, không hữu hạn → `"—"`. Dùng cho `ratePerSec` và `chatQueueLagSeconds`. `formatCount` giữ cho số đếm; sửa để `-0` → `"0"`. |
| m-1 | `formatTimeVn` chỉ nhận ISO có `Z` hoặc offset `±HH:MM`; thiếu → `"--:--:--"`. |
| m-2 | Nút "Làm mới" **không** dùng `disabled` khi đang làm mới: dùng `aria-busy="true"` (và chữ "Đang làm mới…"), vẫn focus được; bấm lần nữa không gọi thêm request (hook đã dedupe). |
| m-3 | `refresh()` gọi khi đang có poll tự động → `isRefreshing = true` cho tới khi promise đó xong. |
| m-4 | Dedupe in-flight gắn với `eventId`: kết quả của request cho `eventId` cũ bị bỏ qua. |
| m-5 | "cập nhật lúc …" đặt cạnh nhãn "Dữ liệu mẫu" (cùng khối đầu trang). |
| m-6 | Dùng token có thật trong `tokens.css` (`text-headline-*`, `text-body-*`). |
| m-7 | Page chỉ đưa `message` của `OpsServiceError` ra client; lỗi khác → message chung "Không tải được số liệu war room." |
| m-8 | Dừng timer tuổi dữ liệu khi polling dừng vì 403. |
| m-9 | Nút điều khiển bị vô hiệu có `aria-describedby` trỏ tới đoạn giải thích EVF-60. |
| m-10 | Gộp `SALE_STATE_LABEL`, `statusOf` về một chỗ; `NotFoundScreen` dùng chung nội dung với `not-found.tsx` nếu gọn. |
| t-1..t-5 | Test design bổ sung: StrictMode không có snapshot ban đầu → đúng **1** lần fetch; test hydration bằng `renderToString` với `generatedAt` cũ (không có tuổi dữ liệu, không banner "Dữ liệu cũ" trong HTML server); nút làm mới `aria-busy` + giữ focus; `formatDecimal` / `formatCount` số âm, thập phân, rất lớn; `formatTimeVn` ISO không offset. |

## 8. Sửa sau challenge (vòng 2; `challenge.md`: PASS, chỉ minor)

| Finding | Quyết định |
|---|---|
| C-1 | Hook có `timeoutMs` (mặc định **10 000**). Request quá hạn → coi là lỗi (giữ snapshot, `isStale = true`, `isRefreshing = false`), kết quả đến muộn bị bỏ qua, lịch poll tiếp tục bình thường. |
| C-2 | **Nợ** (không sửa): tuổi dữ liệu so `generatedAt` của server với đồng hồ trình duyệt; lệch đồng hồ làm báo "cũ" sai. Cách đúng khi có API thật: dùng `useServerTimeSync` / offset. Ghi vào traceability. |
| C-3 | Giá trị lạ không lộ ra UI: `budgetUsedRatio` không hữu hạn → `"—"`; `saleState` lạ → nhãn `"Không rõ trạng thái"`; `ai.mode` lạ → hiện mã kèm câu `"Không rõ tác động tới người mua."`; `formatCount`/`formatDecimal` với `|n| > Number.MAX_SAFE_INTEGER` → `"—"`; inventory rỗng → một hàng `"Chưa có hạng vé."` trong bảng. |
| C-4 | `tierInventoryViolation` trả thêm `"INVALID_DATA"` khi một trong `quota/held/paid/issued/available` không hữu hạn (ưu tiên trước OVER_QUOTA); UI hiện "Dữ liệu không hợp lệ" và tính là lệch tồn kho. |
| C-5 | Nút "Thử lại" dùng cùng cách với "Làm mới": `aria-busy`, không `disabled`, giữ focus. |
| C-6 | `formatCount` làm tròn về 0 (kể cả `-0.5`) → `"0"`; `formatTimeVn` khi `Date` không hợp lệ dù đúng định dạng (vd. `"xZ"`) → `"--:--:--"`. Test design bổ sung test giết các mutant sống: M46 (ô funnel của nguồn stale phải có "(cũ)"), M17 (ticketing stale → không badge vi phạm), M47 (403 lần đầu → không fetch phía client), X63 (đổi `eventId` sau 403 → polling chạy lại), M24, M28. |
| C-7, C-8 | Info, không sửa. |
