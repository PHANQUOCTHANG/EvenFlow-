# EVF-1803/1804 (component miền) — ma trận truy vết AC ↔ test

Nguồn AC: `docs/workflow/EVF-1803-queue-countdown/spec-plan.md` §4.
Nguồn nghiệp vụ: `docs/01-nghiep-vu.md` BR-Q1..BR-Q7 (dòng 36–60), BR-O1..BR-O8 (dòng 84–91).

Test được viết **trước/song song** với cài đặt, chỉ dựa trên AC + business rule, không đọc code cài đặt.

**Trạng thái — hai bảng.** Bảng 1 là các dòng **đã chạy xanh** ở lần `717/717 pass` (sau khi code
review được sửa). Bảng 2 là phần **bổ sung sau code review độc lập** (4 major + 13 minor): 717/717 vẫn
pass sau khi sửa code, nghĩa là **không test nào phủ hành vi mới** — các dòng đó là `chưa chạy`.

Lịch sử: lần chạy đầu là **694/695**, test đỏ duy nhất là `LOBBY noi ro chua co so thu tu` — nguyên nhân
là **copy của panel sai** (nói "rồi mới cấp số thứ tự" ở tương lai mà không nói thẳng "bạn chưa có số thứ
tự"), không phải test sai. Copy đã sửa, test giữ nguyên.

## Công thức progressbar — ĐÃ CHỐT (đừng đoán lại)

Hợp đồng cũ dùng `totalAhead` ("số người đang ở phía trước") làm mẫu số. Đó là **lỗi thiết kế**: con số
đó giảm dần nên thanh tiến trình nhảy ngược, và vì nó luôn chênh `rank` đúng 1 nên `totalAhead - rank`
luôn ≈ 0 → thanh **đứng im ở 0 mãi mãi**. Đã thay bằng mẫu số cố định:

```ts
rank?: number          // rank HIEN TAI
initialRank?: number   // rank LUC VAO hang doi = mau so CO DINH
```

```
aria-valuemin = 0
aria-valuemax = initialRank
aria-valuenow = clamp(initialRank - rank, 0, initialRank)      // TANG khi rank GIAM
```

Thiếu `rank` **hoặc** thiếu `initialRank` → **không vẽ progress** (handoff §11.3 "no fake progress").
Ví dụ chốt bằng test: `initialRank=1000` → `rank=1000 ⇒ 0`; `rank=900 ⇒ 100`; `rank=42 ⇒ 958`;
`rank=1 ⇒ 999`; `rank=1200 ⇒ 0` (clamp, không ra số âm).

## Hợp đồng đã đổi sau code review (để người sau không đọc spec cũ rồi làm sai)

```ts
// hooks/use-server-countdown.ts
export interface ServerCountdown { remainingMs: number; expired: boolean; ready: boolean }
// Truoc khi mount:  { 0, false, false }  — KE CA khi expiresAt da qua.
// Moc khong doc duoc (null | undefined | chuoi khong parse duoc | chuoi khong co mui gio):
//                   { 0, false, false }  — ready:false = "CHUA BIET", khac "con 0", khac "da het han".

// lib/server-time.ts
export function toEpochMs(value: number | string | null | undefined): number | null
// Chi nhan chuoi ket thuc bang Z hoac ±HH:MM / ±HHMM. Khong co offset -> null.

// components/ui/server-expiry-countdown.tsx
// data-state: "unknown" | "running" | "warning" | "expired"   (4 gia tri, truoc la 3)
// unknown -> hien "--:--" (KHONG phai "00:00": 00:00 trong y nhu da het han)
// expired -> vung thong bao doi sang role="alert" + aria-live="assertive"
// warning -> van role="status" + aria-live="polite"

// components/queue/queue-status-panel.tsx
// rank/initialRank: Number.isInteger (initialRank con phai > 0); etaSeconds: Number.isFinite
// admissionWarningThresholdMs?: number ; onJoinWaitlist?: () => void
// lastUpdatedAt chi render SAU khi mount (tranh hydration mismatch UTC vs ICT)
```

**AC-4 đọc lại:** "`expired` đúng ngay lần render ĐẦU" nay nghĩa là **lần render đầu trên client sau khi
mount**. `renderHook` của RTL flush effect nên các test cũ vẫn đúng; phần trước mount được chốt riêng
bằng `renderToStaticMarkup`.

## Bảng 1 — AC đã có test và đã chạy xanh (717/717)

| AC | Nội dung ngắn | Test file | Tên test | Trạng thái |
|---|---|---|---|---|
| AC-1 | `formatClock` dạng `mm:ss` | `apps/web/src/lib/format.test.ts` | `formatClock — dang mm:ss (AC-1)` › 7 case bảng (`0`, `1000`, `59_000`, `60_000`, `90_000`, `600_000`, `3_599_000`) | pass |
| AC-1 | luôn đủ 2 chữ số | `apps/web/src/lib/format.test.ts` | `luon du 2 chu so cho phut va giay (khong ra '0:1')` | pass |
| AC-1 | **làm tròn XUỐNG** (không hứa thêm thời gian) | `apps/web/src/lib/format.test.ts` | `1500ms ra '00:01', KHONG phai '00:02'` | pass |
| AC-1 | floor ở các mốc biên | `apps/web/src/lib/format.test.ts` | `1999ms…`, `999ms…`, `59_999ms…`, `599_500ms…`, `3_599_999ms…` | pass |
| AC-1 | dạng `h:mm:ss` khi ≥ 1 giờ | `apps/web/src/lib/format.test.ts` | `formatClock — dang h:mm:ss khi >= 1 gio (AC-1)` › 4 case bảng + 2 case biên | pass |
| AC-1 | số âm → `00:00`, không đếm lên | `apps/web/src/lib/format.test.ts` | `so am ve 00:00, khong dem len` › 5 case bảng + `khong bao gio co dau tru trong ket qua` | pass |
| AC-1 | `NaN`/`Infinity` → `00:00`, không throw | `apps/web/src/lib/format.test.ts` | `gia tri khong huu hien (AC-1)` › 3 case bảng, `khong throw…`, `khong bao gio lot chuoi 'NaN' ra UI` | pass |
| AC-2 | `1250000` → `"1.250.000 đ"` (không phụ thuộc ICU) | `apps/web/src/lib/format.test.ts` | `1250000 ra dung chuoi '1.250.000 đ' (khong phu thuoc locale ICU)` | pass |
| AC-2 | `0 đ`, dấu chấm phân cách, hậu tố `đ` | `apps/web/src/lib/format.test.ts` | `formatVnd — dinh dang tien VND (AC-2)` › 7 case bảng + `khong dung dau phay…` + `co hau to 'đ'` | pass |
| AC-2 | số âm có dấu trừ đứng trước | `apps/web/src/lib/format.test.ts` | `formatVnd — so am (AC-2)` › 2 test | pass |
| AC-2 | không hữu hiện → `"—"`, không `NaN đ` | `apps/web/src/lib/format.test.ts` | `gia tri khong huu hien tra '—' (AC-2)` › 3 case bảng + `khong throw va KHONG hien 'NaN đ'` | pass |
| AC-3 | `serverOffsetMs` = hiệu, đúng dấu 2 chiều | `apps/web/src/lib/server-time.test.ts` | `serverOffsetMs (AC-3)` › 6 test | pass |
| AC-3 | `serverNow(offset)` = `Date.now() + offset` | `apps/web/src/lib/server-time.test.ts` | `= Date.now() + offsetMs`, `offset am tru dung`, `theo dong ho he thong…` | pass |
| AC-3 | offset thiếu / không hữu hiện → coi là 0 | `apps/web/src/lib/server-time.test.ts` | `khong truyen offset -> coi la 0…`, `offset undefined tuong minh…`, `offset khong huu hien (%s) -> coi la 0, KHONG tra NaN` (3 case) | pass |
| AC-3 | module **không** tự gọi API lấy giờ server | `apps/web/src/lib/server-time.test.ts` | `serverNow / serverOffsetMs khong goi fetch hay XMLHttpRequest`, `nap module cung khong gay request nao`, `khong tra ve Promise` | pass |
| AC-4 | `expiresAt` = now+10′ → `remainingMs` 600_000, `expired` false | `apps/web/src/hooks/use-server-countdown.test.tsx` | `expiresAt = now + 10 phut -> remainingMs 600_000, expired false` | pass |
| AC-4 | nhận cả `number` và ISO `string` | `apps/web/src/hooks/use-server-countdown.test.tsx` | `nhan expiresAt dang ISO string`, `nhan expiresAt dang epoch ms (number)` | pass |
| AC-4 | trôi 1s / 5 phút / hết giờ | `apps/web/src/hooks/use-server-countdown.test.tsx` | `troi 1 giay…`, `troi 5 phut…`, `troi het 10 phut…`, `remainingMs khong bao gio am sau khi qua han` | pass |
| AC-4 | **tab background: tính lại từ mốc, KHÔNG giảm dần** (BR-O2) | `apps/web/src/hooks/use-server-countdown.test.tsx` | `nhay dong ho 300s nhung chi chay 1 nhip -> remainingMs theo thoi gian THUC, khong theo so nhip` | pass |
| AC-4 | tab background — biến thể 9 phút / vượt hạn / nhảy nhiều lần | `apps/web/src/hooks/use-server-countdown.test.tsx` | `nhay dong ho 9 phut, 1 nhip…`, `nhay dong ho qua han, 1 nhip -> expired true ngay`, `nhieu lan nhay dong ho lien tiep van khong tich luy sai so` | pass |
| AC-4 | mốc đã qua → expired **ngay lần render đầu** | `apps/web/src/hooks/use-server-countdown.test.tsx` | `expiresAt trong qua khu -> remainingMs 0, expired true, KHONG can tick`, `expiresAt dung bang now…`, `expiresAt dang ISO trong qua khu…` | pass |
| AC-4 | `null`/`undefined` → 0 nhưng **expired false** | `apps/web/src/hooks/use-server-countdown.test.tsx` | `expiresAt null -> …expired FALSE`, `expiresAt undefined -> …expired FALSE`, `null + thoi gian troi van khong tu chuyen thanh expired` | pass |
| AC-4 | **string không parse được = chưa có mốc, KHÔNG phải hết hạn** (quyết định mới) | `apps/web/src/hooks/use-server-countdown.test.tsx` | `string khong parse duoc -> giong null: remainingMs 0, expired FALSE`, `string rong cung khong phai het han`, `string khong parse duoc + thoi gian troi van khong thanh expired` | pass |
| AC-4 | `offsetMs` làm mốc so sánh | `apps/web/src/hooks/use-server-countdown.test.tsx` | `server nhanh hon client 60s…`, `server cham hon client 60s…`, `offset lam moc da qua…`, `offset van duoc ap dung sau khi tick`, `offset khong huu hien -> coi nhu 0, KHONG ra NaN` | pass |
| AC-4 | clear interval khi unmount, dừng khi hết hạn | `apps/web/src/hooks/use-server-countdown.test.tsx` | `clear interval khi unmount`, `dung chay khi da het han, khong de interval song mai`, `doi expiresAt sang moc moi thi dem lai theo moc moi` | pass |
| AC-4 | hook **không** nhận `durationMs` | `apps/web/src/hooks/use-server-countdown.test.tsx` | `chu ky ham khong co tham so thu ba kieu durationMs` (chốt chính là `npm run typecheck`) | pass |
| AC-5 | hiển thị số còn lại, ISO/number, tick, `offsetMs` | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `hien thi so con lai (AC-5)` › 4 test đầu | pass |
| AC-5 | component cũng tính lại từ mốc khi tab background | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `tab bi background: nhay dong ho nhieu, chi 1 nhip -> so hien theo thoi gian THUC (BR-O2)` | pass |
| AC-5 | `variant` bắt buộc, 3 nhãn **khác nhau** (BR-Q7) | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `ba variant cho ra ba nhan khac nhau doi mot`, `moi variant co phan chu khong rong…`, `hai dong ho checkout (admission vs hold) khac nhan nhau` | pass |
| AC-5 | chữ số `aria-hidden` + text thay thế dạng chữ | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `vung chu so bi aria-hidden…`, `co text thay the dang chu, co nhac so PHUT`, `text thay the doi theo so phut con lai` | pass |
| AC-5 | có `aria-live`, **không** thông báo mỗi giây | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `co vung aria-live (polite hoac assertive)`, `vung aria-live KHONG doi moi giay (khong spam screen reader)` | pass |
| AC-5 | ngưỡng cảnh báo do caller quyết định, **không default** (tín hiệu: `aria-live`) | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `vao duoi nguong -> vung aria-live thong bao…`, `con tren nguong thi chua thong bao gi moi`, `KHONG truyen warningThresholdMs -> khong co canh bao o bat ky moc nao` | pass |
| AC-5 | ngưỡng cảnh báo — chốt bằng `data-state`, không qua câu chữ | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `con nhieu thoi gian -> running`, `remainingMs <= warningThresholdMs -> warning`, `chay tu running sang warning dung tai moc nguong`, `het han -> expired (khong con la warning)`, `KHONG truyen nguong -> khong bao gio warning, chi running roi expired` | pass |
| AC-5 | hết hạn: có text rõ ràng, **không đếm số âm** | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `expiresAt trong qua khu -> co text het han, KHONG dem so am`, `het han trong luc dang mount -> vung aria-live thong bao`, `sau khi het han khong tiep tuc dem xuong so am`, `het han o moi variant deu khong hien so am` | pass |
| AC-6 | phủ đủ 8 trạng thái, mỗi trạng thái có text riêng | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `state %s render duoc va co text khong rong` (8 case), `8 trang thai cho ra 8 noi dung khac nhau (khong truyen tin chi bang mau)`, `state %s khong lam ro ri chu 'undefined' / 'NaN' ra UI` (8 case) | pass |
| AC-7 | **LOBBY bỏ qua rank/ETA/progress dù được truyền** (BR-Q1) | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `truyen rank/etaSeconds/initialRank vao LOBBY thi panel BO QUA het` | pass |
| AC-7 | LOBBY không progressbar, không số thứ tự | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `LOBBY khong co progressbar du co ca rank va initialRank`, `LOBBY khong hien bat ky con so thu tu nao…` | pass |
| AC-7 | LOBBY nói rõ vào sớm không tạo lợi thế + chưa có số thứ tự | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `LOBBY noi ro vao som KHONG tao loi the (chong bot)`, `LOBBY noi ro chua co so thu tu` *(test này đã bắt được một lỗi copy thật)* | pass |
| AC-8 | QUEUED hiển thị rank | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `co rank thi hien rank` | pass |
| AC-8 | thiếu `etaSeconds` → "đang cập nhật", **không suy ETA từ rank** | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `thieu etaSeconds -> hien 'dang cap nhat' va KHONG tu tinh ETA tu rank` | pass |
| AC-8 | có `etaSeconds` → hiển thị ETA đó | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `co etaSeconds -> hien ETA do va bo chu 'dang cap nhat'`, `co etaSeconds thi noi dung khac han khi khong co` | pass |
| AC-8 | thiếu `initialRank` (hoặc thiếu `rank`) → **không** progressbar | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `thieu initialRank -> KHONG co progressbar (no fake progress)`, `thieu rank -> KHONG co progressbar du co initialRank` | pass |
| AC-8 | progressbar đúng **công thức đã chốt** (xem mục trên) | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `co ca rank va initialRank -> aria-valuemin/max/now dung CONG THUC da chot`, `rank=900/initialRank=1000 -> valuenow 100…`, `rank chua nhich (rank = initialRank) -> valuenow 0`, `rank=1 (gan toi luot) -> valuenow sat valuemax`, `rank > initialRank (server day lui) -> clamp ve 0, KHONG ra so am` | pass |
| AC-8 | tính đơn điệu: rank nhỏ hơn ⇒ valuenow lớn hơn | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `don dieu: cung initialRank, rank NHO hon thi valuenow LON hon` | pass |
| AC-9 | ADMITTED chỉ có đồng hồ khi có `admissionExpiresAt` | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `khong co admissionExpiresAt -> KHONG render dong ho`, `co admissionExpiresAt -> render dong ho dem nguoc` | pass |
| AC-9 | ADMITTED dùng variant `admission`, **không** `hold` (BR-Q7) | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `dong ho dung variant 'admission', KHONG phai 'hold' (BR-Q7)` | pass |
| AC-9 | ADMITTED nhận ISO + `offsetMs` | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `nhan admissionExpiresAt dang ISO string`, `ap dung offsetMs cho dong ho admit` | pass |
| AC-9 | `admissionWarningThresholdMs` truyền xuống đồng hồ; không truyền = không cảnh báo (prop mới) | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `nguong NHO hon thoi gian con lai -> chua canh bao`, `nguong LON hon thoi gian con lai -> canh bao ngay va co thong bao`, `chay tu tren nguong xuong duoi nguong thi doi sang canh bao`, `KHONG truyen admissionWarningThresholdMs -> khong canh bao o bat ky muc nao`, `khong co admissionExpiresAt thi nguong cung khong tao ra dong ho nao` | pass |
| AC-9 | EXPIRED: phải xếp lại, không được ưu tiên | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `EXPIRED noi ro phai xep lai va khong duoc uu tien (BR-Q7)` | pass |
| AC-9 | DROPPED ≠ EXPIRED, nêu nguyên nhân mất kết nối | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `DROPPED va EXPIRED cho ra thong diep KHAC nhau`, `DROPPED noi ro nguyen nhan la mat ket noi qua lau (docs/01 dong 48)` | pass |
| AC-9 | RECONNECTING: **chưa mất chỗ** | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `noi ro la CHUA mat cho`, `thong diep khac han DROPPED` | pass |
| AC-9 | SOLD_OUT: không CTA vào checkout | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `khong co CTA dan vao checkout / chon ve`, `noi ro la het ve`, `khong render dong ho dem nguoc du co admissionExpiresAt` | pass |
| AC-9 | SOLD_OUT: mời waitlist **chỉ khi** có `onJoinWaitlist` (BR-Q6, prop mới) | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `khong truyen onJoinWaitlist -> khong co button waitlist va khong nhac waitlist`, `co onJoinWaitlist -> co button waitlist`, `bam button waitlist goi dung callback mot lan`, `co waitlist thi van KHONG co CTA vao checkout`, `onJoinWaitlist o trang thai khac SOLD_OUT thi khong moi waitlist` | pass |
| AC-9 | UNKNOWN: không bịa trạng thái, mời thử lại | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `moi thu lai`, `khong hien rank du duoc truyen…` | pass |
| AC-10 | `lastUpdatedAt` có thì hiện, không có thì không bịa | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `co lastUpdatedAt thi hien thi`, `khong co lastUpdatedAt thi KHONG bia 'vua cap nhat'`, `nhan lastUpdatedAt dang ISO string ma khong lo 'Invalid Date'` | pass |
| AC-10 | panel có `aria-live="polite"` ở mọi trạng thái | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `state %s co vung aria-live="polite"` (8 case) | pass |
| AC-10 | `connectionState` hiện bằng **chữ**, không chỉ icon | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `connectionState %s van render duoc va co text` (3 case), `ba gia tri connectionState cho ra ba noi dung khac nhau (khong chi doi icon)`, `'offline' va 'reconnecting' them chu so voi khi khong truyen gi` | pass |
| AC-11 | Gates | **kiểm bằng lệnh** — không có test tự động | `cd apps/web && npm run lint`, `npm run typecheck`, `npm run build`, `npm run test:coverage` (ngưỡng `vitest.config.ts`: lines/functions/statements 70%, branches 60%); so số test pass với mốc 522 test hiện có để xác nhận không hồi quy | pass (717/717 ở lần gần nhất; **chạy lại** sau phần bổ sung ở Bảng 2) |

## Bảng 2 — bổ sung sau code review độc lập (4 major + 13 minor) — CHƯA CHẠY

Code review sửa code mà **717/717 vẫn pass** ⇒ không test nào phủ hành vi mới. Những dòng dưới đây là
phần bịt lỗ đó. Kèm 2 thay đổi hợp đồng: `ServerCountdown.ready` và `toEpochMs` từ chối chuỗi không
có múi giờ.

| AC | Nội dung ngắn | Test file | Tên test | Trạng thái |
|---|---|---|---|---|
| AC-3 | **`toEpochMs` từ chối chuỗi không có múi giờ** (ECMA: `"2026-10-03"` là UTC nhưng `"…T09:00:00"` là giờ địa phương ⇒ lệch 7 tiếng đúng mốc mở bán) | `apps/web/src/lib/server-time.test.ts` | `toEpochMs — chi nhan moc co mui gio tuong minh (AC-3)` › `tu choi '%s' …` (4 case: chỉ ngày / có giờ không offset / có ms không offset / có dấu cách) | chưa chạy |
| AC-3 | `toEpochMs` nhận `Z`, `±HH:MM`, `±HHMM`, giữ milisecond, `toISOString()` | `apps/web/src/lib/server-time.test.ts` | `nhan chuoi ket thuc bang Z`, `nhan offset dang ±HH:MM va tru dung mui gio`, `nhan offset dang ±HHMM`, `giu milisecond khi co offset`, `Date#toISOString() luon doc duoc` | chưa chạy |
| AC-3 | `toEpochMs` với số epoch / `null` / rác / số không hữu hiện | `apps/web/src/lib/server-time.test.ts` | `so epoch tra lai chinh no`, `%s -> null (chua co moc)` (2 case), `chuoi rac '%s' -> null, khong throw` (5 case), `so khong huu hien (%s) -> null` (3 case), `tra ve number hoac null, khong bao gio tra NaN` | chưa chạy |
| AC-4 | **`ready: false` trước khi mount** — prerender tĩnh không được đóng băng số của lúc build (M1) | `apps/web/src/hooks/use-server-countdown.test.tsx` | `truoc khi mount (prerender tinh): ready false (AC-4, M1)` › `moc tuong lai…`, `moc DA QUA: prerender van KHONG duoc noi la het han`, `khong co moc…`, `offsetMs khong lam thay doi gia tri truoc mount` | chưa chạy |
| AC-4 | `ready: true` sau mount; `ready: false` khi mốc không đọc được (≠ "hết hạn") | `apps/web/src/hooks/use-server-countdown.test.tsx` | `moc tuong lai hop le -> ready true`, `moc da qua -> ready true VA expired true`, `ready la boolean…`, `moc khong doc duoc (%s) -> ready FALSE va expired FALSE` (7 case), `ready khong bao gio true cung luc voi moc khong doc duoc…`, `doi tu moc khong doc duoc sang moc hop le -> ready chuyen thanh true` | chưa chạy |
| AC-4 | đổi mốc không được loé một frame "hết hạn" trên hold còn 10 phút (m6) | `apps/web/src/hooks/use-server-countdown.test.tsx` | `tu moc da qua sang moc tuong lai: expired false NGAY tai lan render do`, `tu hold cu (con 10s) sang hold moi (con 10 phut) khong di qua trang thai het han` | chưa chạy |
| AC-5 | **`spellDuration` — bảng trực tiếp** (toàn bộ nội dung screen reader nghe được về đồng hồ; trước đó 0 test) | `apps/web/src/lib/format.test.ts` | `spellDuration — doc thanh chu cho screen reader (AC-5)` › bảng 10 case + `KHONG doc don vi bang 0`, `GIU phan giay: 1 phut 59 giay khong duoc rut gon thanh '1 phút'`, `lam tron XUONG giong formatClock`, `khop voi formatClock tren cung mot gia tri`, `duoi 1 giay…`, `so am khong lot dau tru ra UI`, `gia tri khong huu hien (%s)…` (3 case) | chưa chạy |
| AC-5 | **`data-state="unknown"` + `--:--`** trước khi biết mốc; `unknown` không bao giờ thành `warning` | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `truoc khi biet moc: unknown + '--:--' (AC-5, M1)` › `markup prerender la unknown…`, `markup prerender cua moc DA QUA cung khong noi la het han`, `moc khong doc duoc (string rac) -> unknown va '--:--', khong phai 00:00`, `chuoi khong co mui gio bi coi la chua biet moc`, `unknown KHONG BAO GIO thanh warning…`, `unknown khong thong bao gi cho screen reader`, `unknown khong hien so am va khong hien 'NaN'` | chưa chạy |
| AC-5 | hết hạn ⇒ `role="alert"` + `aria-live="assertive"`; cảnh báo vẫn `status`/`polite` | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `het han -> vung thong bao la role=alert + aria-live=assertive va co noi dung`, `canh bao -> van la role=status + aria-live=polite, KHONG phai alert`, `chay tu canh bao sang het han thi doi tu status sang alert` | chưa chạy |
| AC-5 | screen reader chỉ đọc nhãn **một lần** (trước đó đọc 2–3 lần) | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `variant %s: khong co cum tu nao bi doc lap` (3 case), `vung chu so khong di vao noi dung AT doc`, `cau sr-only nhac don vi phut dung mot lan` | chưa chạy |
| AC-5 | icon ngữ nghĩa kèm văn bản, icon `aria-hidden` (DESIGN.md, AC-5 quên mang vào) | `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | `trang thai dang chay co icon va icon khong bi AT doc`, `trang thai canh bao co icon canh bao rieng va cung bi an khoi AT`, `thong tin khong bao gio chi nam o icon: bo icon di van con chu` | chưa chạy |
| AC-8 | **chặn số rác** (M2): `rank` NaN/Infinity/không nguyên ⇒ y như không truyền, **không** progressbar (bug cũ: `width:"NaN%"` ⇒ thanh hiện đầy 100%) | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `so rac phai bi coi nhu KHONG CO (AC-8, M2)` › `rank = %s … -> y het nhu khong truyen rank` (4 case), `rank = %s … -> KHONG co progressbar` (4 case) | chưa chạy |
| AC-8 | `initialRank` phải là số nguyên dương (`0.5` từng lọt vì `0.5 > 0`) | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `initialRank = %s (%s) -> KHONG co progressbar` (6 case: NaN, Infinity, 0, -5, 0.5, 999.9) | chưa chạy |
| AC-8 | `etaSeconds` NaN/Infinity ⇒ **"đang cập nhật"**, tuyệt đối không ra "~0 giây" | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `etaSeconds = %s (%s) -> 'dang cap nhat', TUYET DOI khong ra '0 giây'` (2 case), `etaSeconds = NaN cho ra ket qua y het khi khong truyen etaSeconds`, `etaSeconds am khong tao ra so thoi gian am tren UI` | chưa chạy |
| AC-8 | progressbar có `aria-valuetext` dạng chữ ("958" trần vô nghĩa với AT) | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `progressbar co aria-valuetext dang chu…` | chưa chạy |
| AC-9 | **ADMITTED + đồng hồ vừa hết hạn không được mâu thuẫn** (M3; cửa sổ tới 30s do `poll_after_ms` của BR-Q4) | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `ADMITTED vua het han: khong duoc mau thuan (AC-9, M3)` › `moc tuong lai -> van la copy moi mua binh thuong`, `moc da qua -> BO loi moi di thanh toan`, `moc da qua -> co copy chuyen tiep…`, `moc da qua cho noi dung KHAC han moc tuong lai`, `chay qua moc trong luc dang mount -> copy TU DOI`, `copy chuyen tiep KHAC han trang thai EXPIRED that` | chưa chạy |
| AC-9 | `admissionExpiresAt` là chuỗi rác không lọt "NaN"/"Invalid Date" | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `admissionExpiresAt la chuoi rac -> khong lo 'NaN'/'Invalid Date' ra UI` | chưa chạy |
| AC-10 | `lastUpdatedAt` chỉ render **sau mount** (build chạy UTC, browser ICT ⇒ lệch 7 tiếng ⇒ hydration mismatch) | `apps/web/src/components/queue/queue-status-panel.test.tsx` | `an toan hydration (prerender tinh)` › `markup prerender giong y nhau du co hay khong co lastUpdatedAt`, `sau khi mount thi lastUpdatedAt moi xuat hien`, `dong ho admit trong markup prerender la '--:--'`, `state %s prerender duoc ma khong throw` (8 case) | chưa chạy |

## Số lượng test

| File | Khối `it` | Số case sau khi bung `it.each` | Thêm ở đợt review |
|---|---|---|---|
| `apps/web/src/lib/format.test.ts` | 32 | 66 | +8 khối / +19 case |
| `apps/web/src/lib/server-time.test.ts` | 27 | 39 | +11 khối / +21 case |
| `apps/web/src/hooks/use-server-countdown.test.tsx` | 42 | 48 | +12 khối / +18 case |
| `apps/web/src/components/ui/server-expiry-countdown.test.tsx` | 41 | 43 | +16 khối / +18 case |
| `apps/web/src/components/queue/queue-status-panel.test.tsx` | 70 | 112 | +18 khối / +37 case |
| **Tổng** | **212** | **308** | **+65 khối / +113 case** |

## Giới hạn đã biết — HOÃN sang ticket riêng, không làm đợt này

1. **`reconnectDeadlineAt` cho cửa sổ giữ chỗ 5 phút (RECONNECTING).** `docs/01` dòng 48:
   `QUEUED --rot ket noi > 90s--> (giu cho 5 phut) --qua han--> DROPPED`. Người dùng cần biết **còn bao
   lâu** để quay lại, nhưng việc đó cần **variant đồng hồ thứ 4** mà handoff §11.4 chỉ định nghĩa 3, kèm
   nhãn mới. Không tự phát minh variant. Hiện tại RECONNECTING chỉ nói "chưa mất chỗ", không nói còn bao lâu.
2. **`poll_after_ms` dạng "cập nhật lại sau N giây" (BR-Q4).** Thiếu nó thì người dùng tự bấm tải lại —
   đúng hành vi mà BR-Q4 muốn chặn. Nhưng polling nằm ngoài phạm vi slice (§1 "Ngoài phạm vi").
3. **BR-Q2 — phân biệt nhóm lottery và người vào sau T0.** Người nối FIFO sau T0 sẽ thấy rank xấu hơn mà
   không hiểu vì sao; panel QUEUED hiện không phân biệt. Ưu tiên thấp.
4. **`formatClock` ≥ 24 giờ ra giờ tích luỹ** (ví dụ `"72:00:00"`), không đổi sang ngày. Chưa có AC cho
   định dạng ngày. Chấp nhận cho variant `sale-start` ở đợt này.
5. **`onRetry` cho `UNKNOWN`.** AC-9 nói "mời thử lại" nhưng hợp đồng không có callback; đợt này lời mời
   **chỉ là chữ**, không phải hành động. Test vì vậy chỉ assert có chữ `/thử lại|tải lại|làm mới/`.

## Phần KHÔNG test được (và lý do)

1. **AC-5 — tabular numbers.** Thuộc tính CSS (`font-variant-numeric: tabular-nums` / `tnum` theo
   `apps/web/doc/design/stitch/md/DESIGN.md §Typography`). Assert được thì cũng chỉ là assert class/style,
   trái nguyên tắc "assert hành vi"; jsdom không tính layout nên không đo được bề rộng chữ số.
   → **review bằng mắt + đọc CSS.**
2. **AC-5 — `--color-warning-fg` cho chữ, `--color-accent` chỉ cho viền/nền.** Cần computed style +
   bảng contrast; jsdom không nạp `tokens.css` qua Tailwind nên `getComputedStyle` trả rỗng.
   → **review code + đối chiếu `src/styles/tokens.css` dòng 43–46, 65.** Test chốt *việc vào trạng thái
   cảnh báo* bằng `data-state="warning"` + nội dung vùng `aria-live`, **không** chốt màu.
3. **AC-5 — `prefers-reduced-motion`.** `vitest.setup.ts` stub `matchMedia` luôn trả `matches: false`.
   → **review code.**
4. **`variant` bắt buộc / hook không nhận `durationMs`.** Ràng buộc kiểu, runtime không quan sát được.
   Chốt bằng `npm run typecheck`; test chỉ có chốt phụ yếu (`useServerCountdown.length <= 2`).
5. **AC-11 — gates.** Bản chất là lệnh CLI, không phải assertion. Lệnh đã ghi ở bảng trên.
6. **`className` passthrough** (có trong hợp đồng §5, không có AC): chỉ kiểm được bằng assert class → bỏ
   qua có chủ ý.
7. **Hành vi background tab thật** (browser throttle `setInterval` xuống ~1 lần/phút) chỉ được *mô phỏng*
   bằng fake timer + `vi.setSystemTime`. Mô phỏng bắt đúng lỗi "giảm dần thay vì tính lại", nhưng không
   bắt được lỗi chỉ xuất hiện với throttle thật (ví dụ `visibilitychange` gây remount). → kiểm tay một
   lần trên Chrome khi có page thật.
8. **Tổ hợp mâu thuẫn `state` × `connectionState`** (ví dụ `state="RECONNECTING"` + `connectionState="live"`).
   Quyết định hiện tại: `connectionState` chỉ là **nhãn phụ**, không ghi đè `state`. Không test tổ hợp mâu thuẫn.

## Chỗ test cố ý khớp lỏng (vì spec không chốt câu chữ)

Spec chốt **ý phải nói ra**, không chốt câu tiếng Việt. Những test sau dùng regex nhiều biến thể; nếu cài
đặt diễn đạt khác hẳn thì **sửa regex trong test, đừng uốn câu chữ UI cho khớp test** — miễn vẫn nói đúng
ý nghiệp vụ. (Ngoại lệ: `LOBBY noi ro chua co so thu tu` đã từng đỏ và **nguyên nhân thật là copy sai**,
không phải regex sai — khi một test loại này đỏ, kiểm nội dung nghiệp vụ trước khi kết luận là test chặt quá.)

- LOBBY: `/sớm/` + nhóm "không tạo lợi thế"; `/chưa có số thứ tự|…/`.
- QUEUED thiếu ETA: `/đang cập nhật/` (chuỗi này **được** spec §2.4 và AC-8 nêu trực tiếp).
- EXPIRED: `/xếp lại|…/` + `/không…ưu tiên/` (AC-9 nêu trực tiếp hai ý này).
- RECONNECTING: `/(chưa|không|vẫn)…(mất chỗ|giữ chỗ|…)/`.
- SOLD_OUT: `/hết vé|đã bán hết|…/`, danh sách từ khoá CTA bị cấm, và tên button waitlist `/danh sách chờ/i`.
- `ServerExpiryCountdown`: **không** hard-code nhãn 3 variant; chỉ chốt "3 nhãn khác nhau đôi một, đều
  không rỗng" + text thay thế có chứa số phút. Nhãn `admission` hiện là `"Suất mua của bạn còn"` theo
  BR-Q7 (**không** gọi là "phiên đăng nhập" như DESIGN.md §Typography mục 1 — hai nghĩa khác nhau cho
  cùng con số 15 phút).
- **Icon** (bổ sung đợt review): khớp theo *lớp ký tự* `/[⏱⏳⌛🕒]/u` cho trạng thái chạy và `/[⚠❗⏰]/u`
  cho cảnh báo, kèm assert icon **không** đi vào nội dung screen reader đọc. Nếu sau này đổi sang inline
  SVG thì phải đổi test sang kiểm `aria-hidden` + `role="img"` thay vì ký tự.
- **Copy chuyển tiếp của ADMITTED vừa hết hạn**: chốt bằng `/xác nhận/i`, `/hết hạn/i`, `/(chưa|không) cần/i`
  và **không** chứa `/đến lượt|hoàn tất/i`. Phần "không chứa lời mời" mới là phần bảo vệ nghiệp vụ; phần
  "chứa" là wording, sửa regex nếu đổi câu chữ.
- **Nhãn bị đọc lặp**: kiểm bằng `hasRepeatedPhrase()` (không có cụm ≥ 3 từ nào lặp lại trong nội dung AT
  đọc được), không hard-code nhãn. Cách này bắt đúng bug "Suất mua của bạn còn. Suất mua của bạn còn 9
  phút 30 giây" mà không phụ thuộc câu chữ.
- **`spellDuration` dưới 1 giây / NaN / Infinity**: spec không chốt chuỗi trả về, nên test chỉ chốt
  *bất biến* (0 và 999 cho cùng kết quả; không chứa "1 giây"; không lọt "NaN"/"Infinity"; không có dấu
  trừ), **không** chốt chuỗi cụ thể. Nếu muốn chốt chuỗi thì cần quyết định sản phẩm.
