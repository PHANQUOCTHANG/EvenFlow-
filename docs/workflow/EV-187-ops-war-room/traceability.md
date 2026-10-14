# EV-187 — Ma trận truy vết AC ↔ Test

Viết bởi test designer từ `spec.md` + `plan.md` §1 và §6, trước khi implement. Cột "Kết quả" cập nhật ở bước verify.
Viết tắt đường dẫn: `svc` = `apps/web/src/lib/ops-service.test.ts`, `hook` = `apps/web/src/hooks/use-ops-war-room.test.tsx`,
`ui` = `apps/web/src/components/ops/ops-war-room.test.tsx`, `fmt` = `apps/web/src/lib/format.test.ts`,
`list` = `apps/web/src/app/(ops)/ops/events/page.test.tsx`, `room` = `apps/web/src/app/(ops)/ops/events/[eventId]/page.test.tsx`,
`nf` = `apps/web/src/app/(ops)/ops/events/[eventId]/not-found.test.tsx`, `e2e` = `tests/e2e/specs/ops-war-room.spec.ts`.

| AC | Yêu cầu | File | Test | Kết quả |
|---|---|---|---|---|
| AC-1 | Một màn hình đủ số liệu, có đơn vị | `ui`, `svc`, `fmt`, `room`, `e2e` | `ui` › AC-1: "h1 là tên sự kiện, có trạng thái bán", "phễu 6 bước hiện số tuyệt đối…", "tuyển sinh: admit rate kèm đơn vị khách/giây…", "chế độ MANUAL…", "bảng tồn kho có đủ cột…", "AI: nấc suy biến, độ trễ chat (giây), % ngân sách token", "mỗi khối có dòng phụ 'làm mới mỗi 2 giây'"; `svc` › "getOpsWarRoom hình dạng snapshot", "bảng số liệu mẫu tất định (M-4)" (5 test), "map trạng thái công khai" (7 test); `fmt` › formatCount (4 test); `room` › "sự kiện có thật: render war room…"; `e2e` › "ops vào war room từ danh sách…" | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |
| AC-2 | Làm mới 2s, "cập nhật lúc", cảnh báo cũ > 5s hoặc lỗi, giữ số | `svc`, `hook`, `ui`, `fmt` | `svc` › "STALE_AFTER_MS = 5000", "OPS_POLL_INTERVAL_MS = 2000", snapshotAgeMs (5 test); `hook` › "gọi fetcher mỗi intervalMs", "mặc định intervalMs = OPS_POLL_INTERVAL_MS", "ageMs = null ở lần render đầu…", "ageMs và isStale đổi khi now() vượt mốc 5000ms", "generatedAt ở tương lai… kẹp về 0", "snapshot có generatedAt cũ thì isStale…", "snapshot mới hơn từ fetcher làm hết cảnh báo cũ"; `ui` › AC-2 (6 test: "cập nhật lúc theo giờ Việt Nam", "dữ liệu quá 5s: banner…", "vượt mốc 5s khi request treo…", "lần làm mới gần nhất lỗi…", "tự làm mới mỗi 2s…", "số mới từ lần làm mới tự động…"); `fmt` › formatTimeVn (6 test) | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |
| AC-3 | Nhãn "Dữ liệu mẫu"; nguồn unavailable → "Không có dữ liệu"; stale → "cũ"; không tô xanh giả | `svc`, `ui`, `room`, `e2e` | `svc` › "isSample = true cho dữ liệu mẫu", "số liệu tồn kho mẫu không vi phạm BR-O1"; `ui` › AC-3: "isSample = true: nhãn 'Dữ liệu mẫu' role status…", "isSample = false…", "waitingroom unavailable…", "ticketing unavailable…", "ai unavailable…", "mọi nguồn ok…", "nguồn %s stale…" (×3), "nguồn %s = %s: không có câu 'hệ thống bình thường'" (×5) | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |
| AC-4 | BR-O1: hàng vi phạm đánh dấu bằng chữ + cảnh báo "lệch tồn kho" | `svc`, `ui` | `svc` › tierInventoryViolation (5 test, cả "vi phạm cả hai điều kiện thì trả OVER_QUOTA"), "patch có thể đặt tồn kho vi phạm…"; `ui` › AC-4: "held + paid + issued > quota…", "available < 0…", "tồn kho hợp lệ…", "ticketing %s: KHÔNG kiểm/không báo BR-O1…" (×2) | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |
| AC-5 | Nấc suy biến AI + câu tác động + "Ops đặt tay" | `ui` | `ui` › AC-5: "mode %s: hiện đúng nấc đó và câu tác động nguyên văn" (×4), "manualOverride = true…", "manualOverride = false…" | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |
| AC-6 | Khu điều khiển tách riêng, nút vô hiệu + "EVF-60", không gửi request | `ui`, `e2e` | `ui` › AC-6: "có khu 'Giám sát' và khu 'Điều khiển' riêng", "nút ghi đè… bị vô hiệu, kèm giải thích EVF-60", "bấm nút điều khiển không gửi request nào"; `e2e` | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |
| AC-7 | 403 / 404 / lỗi khác + thử lại; saleState ≠ ON_SALE | `svc`, `hook`, `ui`, `room`, `nf` | `svc` › "ném OpsServiceError với status 404", `__setOpsSampleOverride` {error:403/500}; `hook` › "lỗi 403 giữa chừng… DỪNG polling", "lỗi 403 ở lần tải đầu…", "lỗi 500 (OpsServiceError) vẫn tiếp tục poll"; `ui` › AC-7 (11 test: initialError 403/404/500 + thử lại, lần tải đầu 403/404/lỗi khác, 403 giữa chừng, saleState ×3, ON_SALE); `room` › "sự kiện không tồn tại (404): gọi notFound()", "lỗi 403…", "lỗi khác (500)…"; `nf` (2 test) | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |
| AC-8 | `/ops/events` liệt kê + link; War room `ready: true` | `svc`, `list`, `e2e` | `svc` › listOpsEvents (5 test); `list` › "liệt kê mọi sự kiện… link tới /ops/events/{id}", "số link war room đúng bằng số sự kiện", "có tiêu đề trang", "mục War room trong OPS_NAV đã ready: true…"; test có sẵn `nav-config.test.ts` + `(ops)/layout.test.tsx` (không sửa, xem `baseline.md` §3) | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |
| AC-9 | Polling an toàn: dừng khi unmount, không chồng, nút "Làm mới", lỗi giữ snapshot | `hook`, `ui` | `hook` › "có initialSnapshot thì KHÔNG fetch khi mount…", "không có initialSnapshot thì fetch ngay…", "StrictMode…" (×2), "fetch chậm hơn interval…", "refresh() khi đang có request treo trả CÙNG promise…", "refresh() trong lúc poll tự động đang treo…", "sau refresh() thủ công, lần poll kế tiếp…", "unmount thì không gọi fetcher thêm…", "request treo xong sau unmount…", "lỗi thường: giữ snapshot cũ…", "sau lỗi vẫn poll…", "refresh() gọi fetcher ngay…", "refresh() lỗi thì giữ snapshot cũ…"; `ui` › "nút 'Làm mới' gọi getOpsWarRoom(eventId) ngay…" | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |
| AC-10 | Không hồi quy + E2E | toàn bộ `apps/web`, `e2e` | `npx vitest run` (baseline 1528 xanh); `e2e` › "ops vào war room từ danh sách, thấy dữ liệu mẫu và làm mới được" (bấm "Làm mới", chờ "cập nhật lúc" đổi, fail khi console error / pageerror) | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |
| AC-11 | Bảng có caption/header; chữ không chỉ màu; không cướp focus | `ui` | `ui` › AC-11: "bảng tồn kho là <table> có <caption> và header cột", "cập nhật tự động không cướp focus", "khi không có gì được focus…", "không đặt aria-live lên vùng số liệu (m-8)"; chữ thay màu kiểm ở AC-3/AC-4 | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |
| Hợp đồng phụ | Tất định, override chỉ dùng trong test, force-dynamic | `svc`, `list`, `room` | `svc` › "hai lần gọi… chỉ khác generatedAt", "listOpsEvents gọi hai lần…", "không suy trạng thái từ Date.now", `__setOpsSampleOverride` (7 test), "NODE_ENV=production thì override bị bỏ qua"; `list`/`room` › `export dynamic = "force-dynamic"` | **Đạt** (verify 2026-10-13, HEAD `73a7bca`; xem `evidence.md`) |

## Bổ sung sau code review vòng 1 (plan §7) — 24 test

| Mục | File | Test |
|---|---|---|
| M-1 formatDecimal / formatCount | `fmt` | "formatDecimal — chấm ngăn nghìn, phẩy thập phân" (7 test), "formatCount — số âm, -0, số rất lớn" (3 test); `ui` › "ratePerSec và chatQueueLagSeconds hiển thị bằng formatDecimal…", "ratePerSec nguyên vẫn có một chữ số thập phân…" |
| m-1 formatTimeVn | `fmt` | "formatTimeVn — chỉ nhận ISO có Z hoặc offset ±HH:MM" (3 test) |
| m-2 nút Làm mới aria-busy | `ui` | "nút 'Làm mới' khi đang làm mới: aria-busy='true', KHÔNG disabled, giữ focus…" |
| m-3 isRefreshing | `hook` | "refresh() khi poll tự động đang treo: isRefreshing = true…" |
| m-4 eventId cũ | `hook` | "kết quả của request cho eventId cũ bị bỏ qua khi eventId đổi" |
| m-5 cập nhật lúc cạnh nhãn mẫu | `ui` | "'cập nhật lúc …' nằm cùng khối với nhãn 'Dữ liệu mẫu'" |
| m-7 message lỗi | `room` | "lỗi không phải OpsServiceError: message chung…", "OpsServiceError (403): giữ status và message…" |
| m-8 timer tuổi dữ liệu dừng sau 403 | `hook` | "sau 403: dừng cả timer tuổi dữ liệu…" |
| m-9 aria-describedby | `ui` | "nút điều khiển vô hiệu có aria-describedby trỏ tới đoạn chứa 'EVF-60'" |
| t-1 | `hook` | "StrictMode không có initialSnapshot: đúng MỘT lần fetch khi mount…" (siết lại test cũ) |
| t-2 hydration | `ui` | "HTML server (renderToString) với generatedAt cũ…" |
| /17 giây/ | `ui` | "AI: nấc suy biến…" — thêm biên trái, xét từng phần tử |

## Bổ sung sau challenge (plan §8) — 28 test

| Mục | File | Test |
|---|---|---|
| C-1 timeoutMs | `hook` | "mặc định timeoutMs = 10000: request treo quá hạn → isStale, hết isRefreshing, poll tiếp sau intervalMs", "timeoutMs tuỳ chỉnh được tôn trọng", "kết quả đến muộn của request đã quá hạn không ghi đè snapshot", "refresh() thủ công bị treo quá hạn: isRefreshing về false" |
| C-2 lệch đồng hồ | — | **Nợ, không test**: tuổi dữ liệu so `generatedAt` của server với đồng hồ trình duyệt; lệch đồng hồ làm báo "cũ" sai. Khi có API thật dùng `useServerTimeSync` / offset. |
| C-3 giá trị lạ | `ui`, `fmt` | `ui` › "budgetUsedRatio không hữu hạn → '—'…", "budgetUsedRatio = Infinity cũng → '—'", "saleState lạ → nhãn 'Không rõ trạng thái'…", "ai.mode lạ → hiện mã kèm 'Không rõ tác động tới người mua.'", "inventory rỗng → một hàng 'Chưa có hạng vé.'…"; `fmt` › "formatCount / formatDecimal — vượt MAX_SAFE_INTEGER trả '—'" (3 test) |
| C-4 INVALID_DATA | `svc`, `ui` | `svc` › "%s không hữu hạn (NaN) trả INVALID_DATA" (×5), "Infinity cũng là INVALID_DATA", "INVALID_DATA ưu tiên trước OVER_QUOTA"; `ui` › "tier có số không hữu hạn: hàng ghi 'Dữ liệu không hợp lệ' + banner lệch tồn kho" |
| C-5 nút Thử lại | `ui` | "đang thử lại: aria-busy='true', KHÔNG disabled, giữ focus" |
| C-6 mutant M46 | `ui` | "M46: waitingroom stale → ô lobby/queued/admitted có '(cũ)'…", "M46: ticketing stale → ô holding/paid/issued có '(cũ)'…" |
| C-6 mutant M17 | `ui` | "M17: ticketing stale + số vi phạm → hàng KHÔNG có badge vi phạm" |
| C-6 mutant M47 | `ui` | "M47: initialError 403 → không có request nào từ client" |
| C-6 mutant X63 | `hook` | "đổi eventId sau khi 403 dừng polling → polling chạy lại cho eventId mới" |
| C-6 mutant M24, M28 | `fmt` | "formatCount(-0.5) trả '0'", "formatTimeVn('xZ') — đúng đuôi Z nhưng ngày hỏng — trả '--:--:--'" |
