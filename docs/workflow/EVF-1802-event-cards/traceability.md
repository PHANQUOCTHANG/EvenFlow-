# EVF-1802/1804 (card + order summary) — traceability AC ↔ test

Nguồn AC: `docs/workflow/EVF-1802-event-cards/spec-plan.md` §4.
Nguồn nghiệp vụ chuẩn: `docs/01-nghiep-vu.md` (BR-E1..E5 dòng 28–32, BR-O1..O8 dòng 84–91, BR-Q6/Q7 dòng 58–59).

Trạng thái mọi dòng là `chưa chạy`: test được viết song song với implementation, chưa chạy lần nào
(theo yêu cầu, không chạy `npm test` trong slice này).

| AC | Nội dung ngắn | Test file | Tên test | Trạng thái |
|---|---|---|---|---|
| AC-1 | `(1250000,"VND")` → `"1.250.000 đ"` | `apps/web/src/lib/money.test.ts` | `(1250000, 'VND') -> '1.250.000 đ'` | chưa chạy |
| AC-1 | `(0,"VND")` → `"0 đ"` | `apps/web/src/lib/money.test.ts` | `(0, 'VND') -> '0 đ' (khong phai '—': 0 dong la mot gia hop le, ve mien phi)` | chưa chạy |
| AC-1 | VND dùng `formatVnd`, không tự nhóm lại | `apps/web/src/lib/money.test.ts` | `VND di qua dung formatVnd, khong tu cai dat lai cach nhom chu so` | chưa chạy |
| AC-1 | USD **không** chứa `"đ"` | `apps/web/src/lib/money.test.ts` | `(1250000, 'USD') KHONG chua 'đ'` | chưa chạy |
| AC-1 | USD có mã tiền tệ | `apps/web/src/lib/money.test.ts` | `(1250000, 'USD') co chua ma tien te 'USD'` | chưa chạy |
| AC-1 | USD giữ đúng chữ số | `apps/web/src/lib/money.test.ts` | `(1250000, 'USD') giu dung cac chu so cua so tien` | chưa chạy |
| AC-1 | USD có nhóm chữ số | `apps/web/src/lib/money.test.ts` | `(1250000, 'USD') co nhom chu so (khong in lien mot khoi 7 chu so)` | chưa chạy |
| AC-1 | không bịa ký hiệu tiền tệ (§2.4, §7) | `apps/web/src/lib/money.test.ts` | `KHONG bia ky hieu tien te cho bat ky ma nao` | chưa chạy |
| AC-1 | `(0,"USD")` không thành `"—"` | `apps/web/src/lib/money.test.ts` | `(0, 'USD') van la mot gia hop le, khong phai '—'` | chưa chạy |
| AC-1 | không đổi mã tiền tệ của caller | `apps/web/src/lib/money.test.ts` | `ma tien te la cua chinh caller, khong bi doi sang ma khac` | chưa chạy |
| AC-1 | `currency=undefined` → `"—"` | `apps/web/src/lib/money.test.ts` | `currency = undefined -> '—'` | chưa chạy |
| AC-1 | `currency=null` → `"—"` | `apps/web/src/lib/money.test.ts` | `currency = null -> '—'` | chưa chạy |
| AC-1 | `currency=""` → `"—"` | `apps/web/src/lib/money.test.ts` | `currency = '' -> '—'` | chưa chạy |
| AC-1 | thiếu currency **không** mặc định VND | `apps/web/src/lib/money.test.ts` | `thieu currency: KHONG in 'đ' va KHONG in con so nao (so tran la lap lung)` | chưa chạy |
| AC-1 | thiếu currency + amount 0 vẫn `"—"` | `apps/web/src/lib/money.test.ts` | `thieu currency voi so tien 0 cung '—' (khong tron '0 đ')` | chưa chạy |
| AC-1 | amount không hữu hạn (VND) → `"—"` | `apps/web/src/lib/money.test.ts` | `NaN / Infinity / -Infinity voi VND -> '—'` | chưa chạy |
| AC-1 | amount không hữu hạn (USD) → `"—"` | `apps/web/src/lib/money.test.ts` | `NaN / Infinity / -Infinity voi USD -> '—' (khong phai 'NaN USD')` | chưa chạy |
| AC-1 | không bao giờ `"NaN"` / `"Infinity"` | `apps/web/src/lib/money.test.ts` | `khong bao gio xuat hien chuoi 'NaN' hay 'Infinity' trong ket qua` | chưa chạy |
| AC-1 | số âm không ra `"NaN"`, không mất dấu | `apps/web/src/lib/money.test.ts` | `so am van ra mot chuoi co chu so, khong ra 'NaN'` | chưa chạy |
| AC-1 | `"vnd"`/`"Vnd"`/`" VND "` ≡ `"VND"` (chuẩn hoá) | `apps/web/src/lib/money.test.ts` | `'vnd' / 'Vnd' / ' VND ' cho ket qua Y HET 'VND'` | chưa chạy |
| AC-1 | `"vnd"` vẫn in hậu tố `đ` | `apps/web/src/lib/money.test.ts` | `'vnd' in dung hau to 'đ' (khong roi sang nhanh ma tien te)` | chưa chạy |
| AC-1 | `"usd"` in mã **viết hoa** | `apps/web/src/lib/money.test.ts` | `'usd' in ma tien te VIET HOA` | chưa chạy |
| AC-1 | `" usd "` ≡ `"USD"` | `apps/web/src/lib/money.test.ts` | `' usd ' cho ket qua y het 'USD'` | chưa chạy |
| AC-1 | currency toàn khoảng trắng → `"—"` | `apps/web/src/lib/money.test.ts` | `currency toan khoang trang -> coi nhu THIEU, tra '—'` | chưa chạy |
| AC-2 | render được cả 6 trạng thái | `apps/web/src/components/event/event-card.test.tsx` | `render duoc ca sau trang thai cua docs/01 ma khong vo` | chưa chạy |
| AC-2 | mỗi trạng thái có nhãn **text** | `apps/web/src/components/event/event-card.test.tsx` | `moi trang thai co phan chu rieng, khong rong (nhan la TEXT, khong chi mau)` | chưa chạy |
| AC-2 | `SOLD_OUT`/`CLOSED`/`COMPLETED` khác nhau | `apps/web/src/components/event/event-card.test.tsx` | `SOLD_OUT, CLOSED va COMPLETED cho ra BA thong diep khac nhau (het ve / het gio / da xong)` | chưa chạy |
| AC-2 | 6 trạng thái khác nhau đôi một | `apps/web/src/components/event/event-card.test.tsx` | `ca sau trang thai khac nhau doi mot (khong trang thai nao bi gop)` | chưa chạy |
| AC-2 | `CANCELLED` nói rõ được hoàn tiền | `apps/web/src/components/event/event-card.test.tsx` | `CANCELLED noi ro duoc hoan tien (docs/01 dong 23: 'hoan tien toan bo')` | chưa chạy |
| AC-2 | `CANCELLED` nói rõ phạm vi **toàn bộ** | `apps/web/src/components/event/event-card.test.tsx` | `CANCELLED noi ro pham vi hoan la TOAN BO, khong de khach doan` | chưa chạy |
| AC-2 | `CANCELLED` nói **quyền** được hoàn, không nói **đã** hoàn xong (docs/01 dòng 78) | `apps/web/src/components/event/event-card.test.tsx` | `CANCELLED noi QUYEN duoc hoan, KHONG noi la DA hoan xong (docs/01 dong 78)` | chưa chạy |
| AC-2 | trạng thái bán được không nói về hoàn tiền | `apps/web/src/components/event/event-card.test.tsx` | `trang thai con ban duoc KHONG noi gi ve hoan tien (khong canh bao sai)` | chưa chạy |
| AC-2 | 4 trạng thái chặn vẫn hiện tên | `apps/web/src/components/event/event-card.test.tsx` | `SOLD_OUT / CLOSED / COMPLETED / CANCELLED van hien ten su kien` | chưa chạy |
| AC-2 | thẻ không bị ẩn khỏi DOM / AT | `apps/web/src/components/event/event-card.test.tsx` | `the khong bi an khoi DOM va khong bi an khoi screen reader` | chưa chạy |
| AC-2 | đúng **một** link chính | `apps/web/src/components/event/event-card.test.tsx` | `co href -> dung mot link` | chưa chạy |
| AC-2 | link trỏ đúng `href` | `apps/web/src/components/event/event-card.test.tsx` | `link tro dung dia chi duoc truyen` | chưa chạy |
| AC-2 | thẻ không phải một link bọc cả khối | `apps/web/src/components/event/event-card.test.tsx` | `the KHONG phai mot link boc ca khoi (click target khong mo ho)` | chưa chạy |
| AC-2 | link có tên đọc được | `apps/web/src/components/event/event-card.test.tsx` | `link co ten doc duoc (khong phai anchor rong)` | chưa chạy |
| AC-2 | thiếu `href` → **không** link | `apps/web/src/components/event/event-card.test.tsx` | `thieu href -> KHONG co link nao (khong link chet)` | chưa chạy |
| AC-2 | thiếu `href` ở mọi trạng thái | `apps/web/src/components/event/event-card.test.tsx` | `thieu href o MOI trang thai deu khong co link` | chưa chạy |
| AC-2 | không `<a>` thiếu/rỗng `href` | `apps/web/src/components/event/event-card.test.tsx` | `khong bao gio render <a> thieu href hoac href rong` | chưa chạy |
| AC-2 | có `imageUrl` → `<img>` đúng src | `apps/web/src/components/event/event-card.test.tsx` | `co imageUrl -> render anh voi src do` | chưa chạy |
| AC-2 | ảnh có thuộc tính `alt` | `apps/web/src/components/event/event-card.test.tsx` | `co imageUrl -> anh co alt (rong hay khong deu duoc, nhung phai co thuoc tinh)` | chưa chạy |
| AC-2 | thiếu `imageUrl` → không `<img>` | `apps/web/src/components/event/event-card.test.tsx` | `thieu imageUrl -> KHONG co <img> nao` | chưa chạy |
| AC-2 | placeholder có `aria-hidden` | `apps/web/src/components/event/event-card.test.tsx` | `thieu imageUrl -> co placeholder va placeholder bi an khoi screen reader` | chưa chạy |
| AC-2 | `imageUrl` rỗng/khoảng trắng không thành `src=""` | `apps/web/src/components/event/event-card.test.tsx` | `imageUrl rong hoac toan khoang trang cung khong duoc thanh <img src=''>` | chưa chạy |
| AC-2 | mọi `<img>` có src không rỗng | `apps/web/src/components/event/event-card.test.tsx` | `moi <img> o moi trang thai deu co src khong rong` | chưa chạy |
| AC-2 | `startsAt` epoch ms | `apps/web/src/components/event/event-card.test.tsx` | `nhan epoch ms` | chưa chạy |
| AC-2 | `startsAt` ISO có múi giờ | `apps/web/src/components/event/event-card.test.tsx` | `nhan chuoi ISO CO mui gio` | chưa chạy |
| AC-2 | mốc không đọc được không ra `NaN` | `apps/web/src/components/event/event-card.test.tsx` | `moc khong doc duoc -> khong hien 'NaN'/'Invalid Date', va van hien ten su kien` | chưa chạy |
| AC-2 | thiếu `startsAt` không bịa ngày | `apps/web/src/components/event/event-card.test.tsx` | `thieu startsAt -> khong bia ngay va van render duoc` | chưa chạy |
| AC-2 | **`SOLD_OUT` không một chiều** (BR-E3 + BR-O1) | `apps/web/src/components/event/event-card.test.tsx` | `SOLD_OUT -> rerender ON_SALE: noi dung doi het theo, khong giu nhan cu` | chưa chạy |
| AC-2 | đi và về lại `ON_SALE` | `apps/web/src/components/event/event-card.test.tsx` | `ON_SALE -> SOLD_OUT -> ON_SALE: ve dung trang thai ban dau` | chưa chạy |
| AC-2 | không trạng thái nào bị dính | `apps/web/src/components/event/event-card.test.tsx` | `tu SOLD_OUT doi sang BAT KY trang thai nao cung duoc (khong trang thai nao bi dinh)` | chưa chạy |
| AC-3 | skeleton có `aria-busy="true"` | `apps/web/src/components/event/event-card.test.tsx` | `co aria-busy="true"` | chưa chạy |
| AC-3 | skeleton không render tên/địa điểm | `apps/web/src/components/event/event-card.test.tsx` | `KHONG render ten su kien hay dia diem` | chưa chạy |
| AC-3 | skeleton không render link | `apps/web/src/components/event/event-card.test.tsx` | `KHONG render link, du co href` | chưa chạy |
| AC-3 | skeleton không render ảnh | `apps/web/src/components/event/event-card.test.tsx` | `KHONG render anh, du co imageUrl` | chưa chạy |
| AC-3 | skeleton không rò rỉ nhãn trạng thái (6 state cùng text) | `apps/web/src/components/event/event-card.test.tsx` | `KHONG ro ri nhan trang thai: skeleton cua ca sau trang thai cho ra text y nhau` | chưa chạy |
| AC-3 | skeleton không chứa chuỗi của trạng thái thật | `apps/web/src/components/event/event-card.test.tsx` | `KHONG ro ri nhan trang thai: khong chuoi nao cua trang thai that xuat hien` | chưa chạy |
| AC-3 | skeleton không render giá / số | `apps/web/src/components/event/event-card.test.tsx` | `KHONG render gia / so tien nao trong skeleton` | chưa chạy |
| AC-3 | thoát `loading` thì nội dung thật quay lại | `apps/web/src/components/event/event-card.test.tsx` | `thoat loading thi noi dung that quay lai` | chưa chạy |
| AC-3 | không `loading` thì không `aria-busy` | `apps/web/src/components/event/event-card.test.tsx` | `khong loading thi KHONG dat aria-busy="true"` | chưa chạy |
| AC-4 | giá VND qua `formatMoney` | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `VND -> hien dung dinh dang '500.000 đ'` | chưa chạy |
| AC-4 | `currency` do caller truyền, USD không có `"đ"` | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `USD -> KHONG in 'đ' va co ma tien te` | chưa chạy |
| AC-4 | giá 0 vẫn hiện | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `gia 0 van hien ro la 0, khong an di (ve mien phi la hop le)` | chưa chạy |
| AC-4 | giá không hữu hạn không ra `NaN` | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `gia khong huu han -> khong bao gio ro ri 'NaN'` | chưa chạy |
| AC-4 | hiện tên hạng vé | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `hien ten hang ve` | chưa chạy |
| AC-4 | 3 nhãn khác nhau đôi một | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `ba nhan cho ra ba thong diep khac nhau doi mot, deu khong rong` | chưa chạy |
| AC-4 | **`limited` không chứa chữ số nào** (BR-O1) | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `limited -> nhan dinh tinh, KHONG chua bat ky chu so nao` | chưa chạy |
| AC-4 | `available`/`sold_out` cũng không chứa số | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `available va sold_out cung khong chua con so nao` | chưa chạy |
| AC-4 | **không có prop số vé còn lại** (adversarial) | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `caller co the lo truyen prop so ve con lai -> con so do KHONG duoc hien o dau` | chưa chạy |
| AC-4 | `sold_out` nói rõ hết vé | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `sold_out noi ro la het ve` | chưa chạy |
| AC-4 | thiếu `availability` → "đang cập nhật" | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `thieu availability -> hien 'dang cap nhat' (handoff §5 nguyen tac 3)` | chưa chạy |
| AC-4 | **thiếu `availability` không khẳng định còn vé** | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `thieu availability -> KHONG khang dinh con ve` | chưa chạy |
| AC-4 | "không biết" khác "còn vé" | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `thieu availability khac han available: hai text khong duoc giong nhau` | chưa chạy |
| AC-4 | **`sold_out` không một chiều: điều khiển quay lại** (BR-E3 + BR-O1) | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `sold_out -> rerender available: dieu khien chon so luong quay lai` | chưa chạy |
| AC-4 | `onQuantityChange` gọi được lại sau khi hồi phục | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `sold_out -> rerender available: onQuantityChange goi duoc binh thuong` | chưa chạy |
| AC-4 | không giữ lại nhãn "hết vé" | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `sold_out -> rerender available: nhan ton kho doi theo, khong giu 'het ve'` | chưa chạy |
| AC-4 | đi và về lại `available` | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `available -> sold_out -> available: ve dung trang thai ban dau` | chưa chạy |
| AC-5 | **thiếu `maxSelectable` → không có điều khiển** | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `thieu maxSelectable -> KHONG co widget chon so luong nao` | chưa chạy |
| AC-5 | thiếu `maxSelectable` → bấm mọi nút không gọi callback | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `thieu maxSelectable -> bam het moi nut cung khong goi onQuantityChange` | chưa chạy |
| AC-5 | **không đoán default 4** | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `thieu maxSelectable -> khong he hien so '4' o dau (khong doan default cua he thong)` | chưa chạy |
| AC-5 | `maxSelectable={0}` → không điều khiển | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `maxSelectable={0} -> KHONG render dieu khien (nguoi nay het suat mua)` | chưa chạy |
| AC-5 | `maxSelectable={0}` có giải thích bằng chữ | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `maxSelectable={0} -> co giai thich bang chu, khong de khach doan vi sao` | chưa chạy |
| AC-5 | `maxSelectable={3}` có điều khiển | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `maxSelectable={3} -> co dieu khien chon so luong` | chưa chạy |
| AC-5 | tăng → gọi callback với số mới | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `bam tang -> goi onQuantityChange voi so MOI` | chưa chạy |
| AC-5 | giảm → gọi callback với số mới | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `bam giam -> goi onQuantityChange voi so MOI` | chưa chạy |
| AC-5 | giá trị do prop quyết định (không state nội bộ) | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `gia tri do prop quyet dinh: bam tang hai lan voi quantity={1} co dinh -> ca hai lan deu la 2` | chưa chạy |
| AC-5 | nút tăng disable ở tối đa | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `o toi da -> nut tang bi disable` | chưa chạy |
| AC-5 | không vượt `maxSelectable` | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `o toi da -> bam tang khong goi onQuantityChange (khong vuot qua 3)` | chưa chạy |
| AC-5 | nút giảm disable ở 0 | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `o 0 -> nut giam bi disable` | chưa chạy |
| AC-5 | không gọi với số âm | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `o 0 -> khong bao gio goi onQuantityChange voi so am` | chưa chạy |
| AC-5 | server hạ giới hạn giữa phiên | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `quantity vuot maxSelectable (server vua ha gioi han) -> van khong cho tang them` | chưa chạy |
| AC-5 | nút tăng/giảm có nhãn đọc được | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `nut tang va nut giam deu co ten doc duoc, khong rong` | chưa chạy |
| AC-5 | giá trị hiện tại thông báo được cho SR | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `gia tri hien tai den duoc screen reader (khong bi nhet trong nhanh aria-hidden)` | chưa chạy |
| AC-5 | giá trị đổi theo prop | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `gia tri hien tai doi theo prop quantity` | chưa chạy |
| AC-5 | `sold_out` → không điều khiển, không callback | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `availability="sold_out" -> khong render dieu khien, khong goi onQuantityChange` | chưa chạy |
| AC-5 | `disabled` → không điều khiển, không callback | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `disabled -> khong render dieu khien, khong goi onQuantityChange` | chưa chạy |
| AC-5 | `sold_out` + `maxSelectable` lớn vẫn không mở đường | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `sold_out co maxSelectable lon van khong mo duong chon` | chưa chạy |
| AC-5 | `sold_out` vẫn hiện tên + giá | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `sold_out van hien ten hang va gia (giu bo cuc, khong an the)` | chưa chạy |
| AC-6 | cảnh báo không đổi được hạng cho tới khi thanh toán / hết hạn, **không** nói huỷ (BR-O3, sửa 2026-10-08) | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `hasActiveHoldElsewhere -> canh bao bang CHU noi cach doi duoc hang: thanh toan hoac het han` + `canh bao KHONG duoc noi hold dang co se bi huy` | chưa chạy |
| AC-6 | cảnh báo nói rõ liên quan vé đang giữ | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `canh bao noi ro la lien quan den ve dang giu` | chưa chạy |
| AC-6 | không có hold khác thì không cảnh báo | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `KHONG hasActiveHoldElsewhere -> khong co canh bao do (test phan biet duoc hai nhanh)` | chưa chạy |
| AC-6 | cảnh báo là **text**, không chỉ màu | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `canh bao la text thuc su, khong phai chi mot thuoc tinh mau/data` | chưa chạy |
| AC-6 | cảnh báo đến được screen reader | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `canh bao den duoc screen reader (khong nam trong nhanh aria-hidden)` | chưa chạy |
| AC-6 | cảnh báo không khoá lựa chọn | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `canh bao khong lam mat dieu khien chon so luong (canh bao, khong phai chan)` | chưa chạy |
| AC-7 | `selected` → `aria-pressed`/`aria-selected` | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `selected -> co aria-pressed="true" HOAC aria-selected="true"` | chưa chạy |
| AC-7 | không `selected` → không tín hiệu | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `khong selected -> khong co tin hieu da chon nao` | chưa chạy |
| AC-7 | tín hiệu là `false` chứ không phải thiếu thuộc tính | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `control mang tin hieu do co trang thai ro rang ca khi chua chon (false, khong phai thieu)` | chưa chạy |
| AC-7 | đổi `selected` thì tín hiệu đổi | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `doi selected khi rerender thi tin hieu doi theo` | chưa chạy |
| AC-7 | control có tên đọc được | `apps/web/src/components/checkout/ticket-tier-card.test.tsx` | `selected cung hien ra bang chu / cau truc, khong chi mot thuoc tinh ARIA` | chưa chạy |
| AC-8 | mỗi dòng: tên, số lượng, đơn giá, tổng dòng | `apps/web/src/components/checkout/order-summary.test.tsx` | `moi dong hien ten hang, so luong, don gia va tong dong` | chưa chạy |
| AC-8 | số lượng đến được screen reader | `apps/web/src/components/checkout/order-summary.test.tsx` | `so luong cua tung dong den duoc screen reader kem ten hang` | chưa chạy |
| AC-8 | `subtotal = Σ(unitAmount × quantity)` | `apps/web/src/components/checkout/order-summary.test.tsx` | `subtotal = Σ(unitAmount × quantity)` | chưa chạy |
| AC-8 | subtotal với một dòng | `apps/web/src/components/checkout/order-summary.test.tsx` | `subtotal dung voi mot dong duy nhat` | chưa chạy |
| AC-8 | mọi số tiền qua `formatMoney` cùng currency | `apps/web/src/components/checkout/order-summary.test.tsx` | `moi so tien di qua formatMoney voi dung currency cua item (USD -> khong co 'đ')` | chưa chạy |
| AC-8 | đơn giá không hữu hạn không ra `NaN` | `apps/web/src/components/checkout/order-summary.test.tsx` | `don gia khong huu han -> khong bao gio ro ri 'NaN'` | chưa chạy |
| AC-8 | **thiếu `fees` → không dòng phí** | `apps/web/src/components/checkout/order-summary.test.tsx` | `thieu fees -> KHONG render dong phi nao` | chưa chạy |
| AC-8 | **không bao giờ "Phí: 0 đ"** | `apps/web/src/components/checkout/order-summary.test.tsx` | `thieu fees -> khong bao gio xuat hien 'Phi: 0 đ' hay '0 đ'` | chưa chạy |
| AC-8 | thiếu `fees` → `total === subtotal` | `apps/web/src/components/checkout/order-summary.test.tsx` | `thieu fees -> total === subtotal (khong co con so thu hai khac)` | chưa chạy |
| AC-8 | có `fees` → `total = subtotal + fees` | `apps/web/src/components/checkout/order-summary.test.tsx` | `co fees -> render dong phi va total = subtotal + fees` | chưa chạy |
| AC-8 | `fees={0}` tường minh thì được hiện | `apps/web/src/components/checkout/order-summary.test.tsx` | `fees={0} duoc truyen TUONG MINH thi duoc hien (0 do server khang dinh, khong phai UI doan)` | chưa chạy |
| AC-8 | không bao giờ render dòng thuế | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG bao gio render dong thue khi khong co prop thue` | chưa chạy |
| AC-8 | `items` rỗng → trạng thái rỗng có chữ | `apps/web/src/components/checkout/order-summary.test.tsx` | `hien mot trang thai rong co chu` | chưa chạy |
| AC-8 | `items` rỗng → **không "0 đ"** | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG hien '0 đ' nhu mot tong hop le` | chưa chạy |
| AC-8 | `items` rỗng → không số tiền nào | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG hien chuoi tong nao (khong co so tien nao ca)` | chưa chạy |
| AC-8 | rỗng khác có dòng | `apps/web/src/components/checkout/order-summary.test.tsx` | `items rong khac han items co dong (khong dung chung mot layout tong 0)` | chưa chạy |
| AC-8 | **trộn currency → không cộng lẫn** | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG hien tong cong lan (150.000 / 150000 khong duoc xuat hien)` | chưa chạy |
| AC-8 | trộn currency → có thông báo lỗi | `apps/web/src/components/checkout/order-summary.test.tsx` | `co thong bao loi den duoc tro ho tro (role alert/status hoac chu ro rang)` | chưa chạy |
| AC-8 | trộn currency vẫn hiện đúng từng dòng | `apps/web/src/components/checkout/order-summary.test.tsx` | `tung dong van hien dung tien te cua rieng no (khong mat thong tin, chi khong cong)` | chưa chạy |
| AC-8 | cùng currency thì không báo lỗi | `apps/web/src/components/checkout/order-summary.test.tsx` | `cung mot currency o moi item thi KHONG bao loi (test phan biet duoc hai nhanh)` | chưa chạy |
| AC-9 | phủ cả 6 trạng thái hold | `apps/web/src/components/checkout/order-summary.test.tsx` | `render duoc ca sau trang thai ma khong vo` | chưa chạy |
| AC-9 | mỗi trạng thái có chữ riêng | `apps/web/src/components/checkout/order-summary.test.tsx` | `moi trang thai co phan chu rieng, khong rong` | chưa chạy |
| AC-9 | **`ambiguous` không bị gộp với `expired`** | `apps/web/src/components/checkout/order-summary.test.tsx` | `ambiguous KHONG bi gop voi expired (hai thong diep khac nhau)` | chưa chạy |
| AC-9 | `ambiguous` khác `creating`/`active` | `apps/web/src/components/checkout/order-summary.test.tsx` | `ambiguous cung khac creating va khac active` | chưa chạy |
| AC-9 | `expired` khác `sold_out` | `apps/web/src/components/checkout/order-summary.test.tsx` | `expired khac sold_out (tra kho vi het gio khac het ve)` | chưa chạy |
| AC-9 | `creating`: nút chính disable | `apps/web/src/components/checkout/order-summary.test.tsx` | `nut chinh bi disable` | chưa chạy |
| AC-9 | `creating`: không tạo hold thứ hai | `apps/web/src/components/checkout/order-summary.test.tsx` | `bam nut chinh khong goi onPrimaryAction (khong tao hold thu hai)` | chưa chạy |
| AC-9 | `creating`: có chỉ báo | `apps/web/src/components/checkout/order-summary.test.tsx` | `co chi bao dang xu ly den duoc tro ho tro` | chưa chạy |
| AC-9 | `creating`: **không nói đã giữ được vé** | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG noi la da giu duoc ve` | chưa chạy |
| AC-9 | `creating`: không render đồng hồ giữ vé | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG render dong ho giu ve (chua co hold thi chua co moc het han)` | chưa chạy |
| AC-9 | **`active`: nhãn `hold`** (BR-Q7) | `apps/web/src/components/checkout/order-summary.test.tsx` | `text chua nhan cua variant 'hold'` | chưa chạy |
| AC-9 | **`active`: không nhãn `admission`** | `apps/web/src/components/checkout/order-summary.test.tsx` | `text KHONG chua nhan cua variant 'admission' (hai dong ho khong duoc lan)` | chưa chạy |
| AC-9 | `active`: không nhãn `sale-start` | `apps/web/src/components/checkout/order-summary.test.tsx` | `text KHONG chua nhan cua variant 'sale-start'` | chưa chạy |
| AC-9 | `active`: `expiresAt` từ prop (BR-O2) | `apps/web/src/components/checkout/order-summary.test.tsx` | `dung dung holdExpiresAt cua prop (09:30), khong tu tinh 10 phut (BR-O2)` | chưa chạy |
| AC-9 | `active`: nhận ISO có múi giờ | `apps/web/src/components/checkout/order-summary.test.tsx` | `nhan holdExpiresAt dang chuoi ISO CO mui gio` | chưa chạy |
| AC-9 | `active`: đếm ngược theo thời gian trôi | `apps/web/src/components/checkout/order-summary.test.tsx` | `dem nguoc theo thoi gian troi that` | chưa chạy |
| AC-9 | **đồng hồ không reset khi rerender** (§2.7) | `apps/web/src/components/checkout/order-summary.test.tsx` | `dong ho KHONG reset khi rerender (handoff §11.6: 'Do not reset timer on rerender')` | chưa chạy |
| AC-9 | `offsetMs` được chuyển xuống | `apps/web/src/components/checkout/order-summary.test.tsx` | `chuyen offsetMs xuong dong ho (lech gio may khach khong lam sai so con lai)` | chưa chạy |
| AC-9 | `holdWarningThresholdMs` được chuyển xuống | `apps/web/src/components/checkout/order-summary.test.tsx` | `chuyen holdWarningThresholdMs xuong dong ho (nguong la policy cua caller)` | chưa chạy |
| AC-9 | không truyền ngưỡng → không cảnh báo | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG truyen nguong -> khong co trang thai canh bao nao` | chưa chạy |
| AC-9 | thiếu `holdExpiresAt` → không bịa mốc | `apps/web/src/components/checkout/order-summary.test.tsx` | `thieu holdExpiresAt -> khong bia moc, khong hien 00:00 nhu da het han` | chưa chạy |
| AC-9 | `active`: nút chính dùng được | `apps/web/src/components/checkout/order-summary.test.tsx` | `nut chinh dung duoc va goi onPrimaryAction` | chưa chạy |
| AC-9 | `expired`: nói rõ vé đã trả kho | `apps/web/src/components/checkout/order-summary.test.tsx` | `noi ro ve da duoc tra lai kho / khong con giu` | chưa chạy |
| AC-9 | `expired`: nói rõ phải chọn lại | `apps/web/src/components/checkout/order-summary.test.tsx` | `noi ro phai chon lai` | chưa chạy |
| AC-9 | `expired`: nút chính không còn là thanh toán | `apps/web/src/components/checkout/order-summary.test.tsx` | `nut chinh KHONG con la nut thanh toan duoc truyen vao` | chưa chạy |
| AC-9 | `expired`: không mời thanh toán | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG moi thanh toan bang chu` | chưa chạy |
| AC-9 | `sold_out`: không nút thanh toán (BR-Q6) | `apps/web/src/components/checkout/order-summary.test.tsx` | `khong co nut thanh toan dung duoc` | chưa chạy |
| AC-9 | `sold_out`: nói rõ hết vé | `apps/web/src/components/checkout/order-summary.test.tsx` | `noi ro la het ve` | chưa chạy |
| AC-9 | `sold_out`: không đồng hồ giữ vé | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG render dong ho giu ve` | chưa chạy |
| AC-9 | **`sold_out` không một chiều: nút chính quay lại** (BR-E3 + BR-O1) | `apps/web/src/components/checkout/order-summary.test.tsx` | `sold_out -> rerender idle: nut chinh quay lai va bam duoc` | chưa chạy |
| AC-9 | không giữ lại thông điệp "hết vé" | `apps/web/src/components/checkout/order-summary.test.tsx` | `sold_out -> rerender idle: khong giu lai thong diep 'het ve'` | chưa chạy |
| AC-9 | đi và về lại `idle` | `apps/web/src/components/checkout/order-summary.test.tsx` | `idle -> sold_out -> idle: ve dung trang thai ban dau` | chưa chạy |
| AC-9 | **`ambiguous`: không nói thành công** (BR-O5) | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG noi thanh cong` | chưa chạy |
| AC-9 | **`ambiguous`: không nói thất bại** | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG noi that bai` | chưa chạy |
| AC-9 | **`ambiguous`: không nút nào tạo hold mới** | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG render nut nao se tao hold moi (bam het moi nut cung khong goi onPrimaryAction)` | chưa chạy |
| AC-9 | `ambiguous`: không nút thử lại/thanh toán | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG co nut nao mang nghia thu lai / tao lai / thanh toan` | chưa chạy |
| AC-9 | `ambiguous`: không render nút chính của caller | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG render nut chinh duoc truyen vao (du caller van truyen label)` | chưa chạy |
| AC-9 | `ambiguous`: chỉ mời chờ xác nhận | `apps/web/src/components/checkout/order-summary.test.tsx` | `moi khach CHO he thong xac nhan` | chưa chạy |
| AC-9 | `ambiguous`: thông báo đến được AT | `apps/web/src/components/checkout/order-summary.test.tsx` | `thong bao den duoc tro ho tro (co vung live hoac role status/alert)` | chưa chạy |
| AC-9 | `ambiguous`: không đồng hồ giữ vé | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG render dong ho giu ve (chua biet co hold hay khong)` | chưa chạy |
| AC-9 | `idle`: không đồng hồ giữ vé | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG render dong ho giu ve khi chua co hold` | chưa chạy |
| AC-9 | `idle`: không nói đã giữ được vé | `apps/web/src/components/checkout/order-summary.test.tsx` | `KHONG noi la da giu duoc ve` | chưa chạy |
| AC-9 | `idle`: nút chính dùng được | `apps/web/src/components/checkout/order-summary.test.tsx` | `nut chinh dung duoc de bat dau giu ve` | chưa chạy |
| AC-10 | có `aria-expanded` | `apps/web/src/components/checkout/order-summary.test.tsx` | `co dieu khien mo rong voi aria-expanded` | chưa chạy |
| AC-10 | **`aria-controls` trỏ phần tử tồn tại** | `apps/web/src/components/checkout/order-summary.test.tsx` | `aria-controls tro vao mot phan tu CO THUC trong DOM` | chưa chạy |
| AC-10 | toggle có tên đọc được | `apps/web/src/components/checkout/order-summary.test.tsx` | `dieu khien mo rong co ten doc duoc` | chưa chạy |
| AC-10 | `aria-expanded` đổi khi bấm | `apps/web/src/components/checkout/order-summary.test.tsx` | `bam dieu khien thi aria-expanded doi gia tri` | chưa chạy |
| AC-10 | **nút chính có mặt ở cả hai trạng thái** | `apps/web/src/components/checkout/order-summary.test.tsx` | `nut chinh co mat o CA hai trang thai thu gon va mo rong` | chưa chạy |
| AC-10 | nút chính bấm được khi thu gọn | `apps/web/src/components/checkout/order-summary.test.tsx` | `nut chinh van bam duoc khi dang thu gon` | chưa chạy |
| AC-10 | đồng hồ không reset khi thu gọn/mở rộng | `apps/web/src/components/checkout/order-summary.test.tsx` | `dong ho giu ve khong bi reset khi thu gon/mo rong` | chưa chạy |
| AC-8 ∩ AC-10 | `items` rỗng → nút chính **vẫn render** (không bị giấu ở mobile) | `apps/web/src/components/checkout/order-summary.test.tsx` | `nut chinh VAN render khi items rong (khong bi giau o mobile)` | chưa chạy |
| AC-8 ∩ AC-10 | `items` rỗng → nút chính **disabled** | `apps/web/src/components/checkout/order-summary.test.tsx` | `nut chinh bi DISABLED khi items rong` | chưa chạy |
| AC-8 ∩ AC-10 | `items` rỗng → không gọi `onPrimaryAction` | `apps/web/src/components/checkout/order-summary.test.tsx` | `bam het moi nut khi items rong cung khong goi onPrimaryAction` | chưa chạy |
| AC-8 ∩ AC-10 | có `items` trở lại → bấm được (không khoá vĩnh viễn) | `apps/web/src/components/checkout/order-summary.test.tsx` | `co items tro lai -> nut chinh bam duoc (khong khoa vinh vien)` | chưa chạy |
| AC-11 | gates: lint / typecheck / build / coverage | *(command-verified, không có test file)* | `cd apps/web && npm run lint` · `npm run typecheck` · `npm run build` · `npm run test:coverage` (ngưỡng vitest.config.ts: lines/functions/statements 70, branches 60); 830 test hiện có vẫn pass | chưa chạy |
| AC-11 | grep màu hard-code trong 3 file mới = rỗng | *(command-verified, không có test file)* | `git grep -nE '#[0-9a-fA-F]{3,8}\|rgba?\(\|hsla?\(' -- apps/web/src/components/event/event-card.tsx apps/web/src/components/checkout/ticket-tier-card.tsx apps/web/src/components/checkout/order-summary.tsx` → không có dòng nào | chưa chạy |

Tổng: `money.test.ts` 24 test · `event-card.test.tsx` 39 test · `ticket-tier-card.test.tsx` 49 test ·
`order-summary.test.tsx` 74 test = **186 test**.

---

## Không test được ở tầng unit — và vì sao

| Mục | Lý do |
|---|---|
| **AC-3 "skeleton tôn trọng `prefers-reduced-motion`"** | Trong jsdom không có layout engine và không có CSS cascade, nên cách duy nhất để "chứng minh" là assert tên class (`motion-reduce:*`) hoặc assert `window.matchMedia` được gọi — cả hai đều là assert cách cài đặt, trái với yêu cầu "assert behavior, not CSS classes". Phải kiểm bằng mắt / Playwright với `emulateMedia({ reducedMotion: "reduce" })`, hoặc bằng review `tokens.css`. **Chưa có test nào phủ mục này.** |
| **AC-2 "giảm nhấn thị giác" cho 4 trạng thái chặn** | "Giảm nhấn" là độ mờ/độ tương phản — thuộc tính CSS. Test chỉ phủ được nửa kiểm chứng được: thẻ **vẫn hiển thị**, vẫn có tên, không bị `hidden`/`aria-hidden`. Phần "giảm nhấn" để review thị giác. |
| **AC-10 "sticky"** | `position: sticky` không quan sát được trong jsdom. Chỉ phủ được phần thu gọn/mở rộng + nút chính luôn hiện. |
| **AC-10 "trên mobile"** | Không có viewport thật trong jsdom; test phủ cơ chế `aria-expanded`/`aria-controls` và sự hiện diện của nút chính, không phủ breakpoint nào kích hoạt dạng thu gọn. |
| **AC-4 "component không có prop số vé còn lại"** | Không tồn tại prop là điều `tsc` kiểm, không phải vitest kiểm. Test chỉ dựng được phiên bản đối kháng: truyền prop lạ qua cast rồi bắt buộc con số không được xuất hiện. Hàng rào thật là `TicketTierCardProps` + `npm run typecheck` (AC-11). |
| **AC-11** | Là gate chạy bằng command (`lint`, `typecheck`, `build`, `test:coverage`, `git grep`), không có test file. Hai dòng cuối bảng ghi đúng command. |
| **BR-O3** (bản cũ: "hold cũ bị huỷ và trả kho ngay"; sửa 2026-10-08: "trả về đúng hold đang có") | Hành vi backend. UI chỉ phủ được phần cảnh báo (AC-6). Việc hold cũ có thực sự được trả kho thuộc test của Ticketing service. |
| **BR-O5 `Idempotency-Key`** | Slice này không gọi API (spec §1 "Ngoài phạm vi"). Test chỉ phủ được *hệ quả UI* của việc không biết kết quả (trạng thái `ambiguous`), không phủ được header nào được gửi. |

## Khớp lỏng (loose match) — thất bại ở đây nghĩa là **kiểm lại câu chữ trước khi nới regex**

Spec không chốt câu chữ tiếng Việt cho hầu hết nhãn, nên những assertion dưới đây dùng regex rộng.
Nếu một trong số này đỏ: **đọc lại copy trong implementation trước**. Chỉ nới regex khi câu chữ mới
vẫn truyền tải đúng ý nghiệp vụ; nếu câu chữ đã đánh mất ý đó thì sửa copy, không sửa test.

| Nơi | Regex / chuỗi | Ý nghiệp vụ không được mất |
|---|---|---|
| `event-card` CANCELLED | `/ho[àa]n\s*(l[ạa]i\s*)?(ti[ềe]n\|100)/i` | Khách bị huỷ sự kiện phải biết **mình được hoàn tiền** (docs/01 dòng 23). Nếu copy dùng từ khác ("bồi hoàn", "trả lại khoản đã thanh toán") thì nới regex — nhưng copy vẫn phải nói rõ có tiền về. |
| `event-card` CANCELLED | `/to[àa]n b[ộo]\|100\s*%\|đ[ầa]y đ[ủu]\|t[ấa]t c[ảa]/i` | Phạm vi hoàn là **toàn bộ**, không để khách đoán là một phần. |
| `ticket-tier-card` | `/đang c[ậa]p nh[ậa]t/i` | AC-4 ghi thẳng "đang cập nhật"; handoff §5 nguyên tắc 3 cũng dùng chính chữ này. Đây là khớp gần như chốt, nhưng vẫn là câu chữ. |
| `ticket-tier-card` | `/c[òo]n v[ée]\|c[òo]n ch[ỗo]\|s[ẵa]n s[àa]ng\|available/i` (**phủ định**) | Thiếu `availability` **không được** khẳng định còn vé (BR-O1). Nếu đỏ vì copy của trạng thái "chưa biết" vô tình chứa chữ "còn vé" thì đó là **bug thật**, không phải regex sai. |
| `ticket-tier-card` | `/h[ếe]t v[ée]\|h[ếe]t ch[ỗo]\|s[ốo]ld.?out/i` | `sold_out` phải nói rõ là hết vé. |
| `ticket-tier-card` | `/hu[ỷyỳ]/i` và `/gi[ữu] v[ée]\|đang gi[ữu]\|hold/i` | BR-O3: cảnh báo phải nói **hold hiện tại bị huỷ**. Chú ý test đối xứng: khi không có `hasActiveHoldElsewhere` thì chữ "huỷ" **không được** xuất hiện — nếu component dùng chữ "huỷ" cho việc khác thì cả hai test này cần xem lại cùng lúc. |
| `ticket-tier-card` | `INCREASE = /t[ăa]ng\|c[ộo]ng\|increase\|plus\|^\+$/i`, `DECREASE = /gi[ảa]m\|b[ớo]t\|tr[ừu]\|decrease\|minus\|^[-−–]$/i` | Cố tình **không** nhận "thêm": một nút "Thêm vào đơn" sẽ bị nhận diện nhầm là nút tăng. Nếu implementation đặt tên nút tăng là "Thêm 1 vé" thì nới `INCREASE` **và** kiểm lại rằng không nút nào khác khớp. |
| `order-summary` | `/ph[íi]\|fee/i` (**phủ định** khi thiếu `fees`) | AC-8: thiếu `fees` thì không có dòng phí nào. **Cố tình không dùng `\b` sau chữ có dấu**: `\b` trong JS là ASCII nên `/ph[íi]\b/` không bao giờ khớp `"Phí "` và cả assertion phủ định sẽ luôn pass một cách vô nghĩa. Đổi lại phải chấp nhận khớp nhầm "phía"/"phiên" nếu copy sau này dùng những chữ đó. |
| `order-summary` | `/thu[ếe]\|\bVAT\b\|\btax\b/i` (**phủ định**) | Không bao giờ bịa dòng thuế. Cùng lý do `\b` như trên. |
| `order-summary` | `/(^\|[^.\d])0\s*đ/` (**phủ định**) | Phải neo vào ranh giới số: `"200.000 đ"` cũng chứa chuỗi `"0 đ"`, nên không được dùng `toContain("0 đ")` khi `items` không rỗng. |
| `order-summary` mixed currency | `/l[ỗo]i\|kh[ôo]ng (th[ểe]\|h[ợo]p l[ệe])\|kh[áa]c nhau\|kh[ôo]ng kh[ớo]p/i` **hoặc** `role=alert/status` | Trộn currency phải báo cho khách, không im lặng. Chấp nhận hai cách cài đặt. |
| `order-summary` `creating`/`idle` | `/đ[ãa] gi[ữu]\|gi[ữu] (v[ée] )?th[àa]nh c[ôo]ng/i` (**phủ định**) | Chưa có hold thì không được nói đã giữ được vé. |
| `order-summary` `expired` | `/tr[ảa] (l[ạa]i \|v[ềe] )?kho\|kh[ôo]ng c[òo]n gi[ữu]\|đ[ãa] đ[ưu][ợo]c tr[ảa]/i` + `/ch[ọo]n l[ạa]i\|ch[ọo]n v[ée] l[ạa]i\|b[ắa]t đ[ầa]u l[ạa]i/i` | AC-9: phải nói rõ **vé đã về kho** và **phải chọn lại**. |
| `order-summary` `ambiguous` | `/th[àa]nh c[ôo]ng\|đ[ãa] gi[ữu]\|đ[ãa] thanh to[áa]n\|đ[ãa] x[áa]c nh[ậa]n/i` (**phủ định**) và `/th[ấa]t b[ạa]i\|kh[ôo]ng th[àa]nh c[ôo]ng\|l[ỗo]i\|b[ịi] t[ừu] ch[ốo]i\|hu[ỷy]\|h[ủu]y/i` (**phủ định**) | BR-O5: không khẳng định cả hai chiều. Regex thứ hai cấm cả chữ "lỗi" và "huỷ" — nếu copy của `ambiguous` muốn dùng những chữ đó thì gần như chắc chắn nó đang nói sang một chiều. |
| `order-summary` `ambiguous` | `/ch[ờo]\|đang x[áa]c nh[ậa]n\|đang ki[ểe]m tra\|đang x[ửu] l[ýy]/i` | Phải mời khách **chờ hệ thống xác nhận**. Regex này rộng (chữ "cho" cũng khớp), nên nó là test yếu: ý nghiệp vụ thật được bảo vệ bởi các test phủ định ở trên và test "không nút nào tạo hold mới". |
| `order-summary` `ambiguous`/`expired`/`sold_out` | `/th[ửu] l[ạa]i\|l[àa]m l[ạa]i\|t[ạa]o l[ạa]i\|gi[ữu] l[ạa]i\|thanh to[áa]n\|ti[ếe]p t[ụu]c\|retry/i` trên **tên nút** | AC-9: không mời tạo hold mới / không mời thanh toán. Hàng rào mạnh hơn nằm ở test bấm-hết-mọi-nút rồi assert `onPrimaryAction` không được gọi — test đó không phụ thuộc câu chữ. |
| `order-summary` / `ticket-tier-card` / `event-card` | **Không bao giờ** `not.toContain("đ")` trên `textContent`, và **không bao giờ** `\b` trong regex. Dùng `textRuns(container)` (từng text node) + `/\d[\d.,]*\s*(?:đ(?!\p{L})|[A-Z]{3})/u` | **Hai bẫy đã thực sự làm đỏ test ở đây, ghi lại cả hai.**<br><br>**(a) `đ` là chữ thường tiếng Việt.** "đơn hàng", "đã", "đang cập nhật" đều chứa nó, nên `not.toContain("đ")` *không thể* pass. Chỉ được chốt **hậu tố tiền tệ**: một chữ số rồi tới `đ`.<br><br>**(b) `\b` trong JS là ASCII, và `textContent` nối các node liền nhau mà không chèn khoảng trắng.** Hai cách hỏng khác nhau:<br>&nbsp;&nbsp;• `\b` **sau chữ có dấu** (`/phí\b/`, `/thuế\b/`): `í`/`ế` không phải ASCII word char nên theo sau là dấu cách thì *không có* boundary → regex không bao giờ khớp → **assertion phủ định luôn pass một cách vô nghĩa**.<br>&nbsp;&nbsp;• `\b` **sau token ASCII** (`/\bUSD\b/`, `/\bVAT\b/`): trên chuỗi gộp, `"500.000 USD"` đứng ngay trước `"Chọn hạng này"` thành `"...USDChọn..."`; `D` và `C` đều là word char nên *không có* boundary → **assertion khẳng định trượt oan** (và assertion phủ định thì lại pass oan).<br><br>**Cách dùng đã chốt:** mọi ràng buộc "có / không có SỐ TIỀN" chạy trên **từng text node** (`textRuns`), vì một text node luôn là một đoạn liên tục. Lookahead `(?!\p{L})` chỉ đặt **sau `đ`** (để loại "2 đơn"), **không** đặt sau mã tiền tệ (vì mã nằm cuối node sẽ bị chữ đầu của node kế tiếp nối vào và lookahead sẽ giết chính assertion). Ở nơi "không có chữ số nào" đã đủ mạnh (skeleton `EventCard`) thì dùng `/\d/` và không cần regex tiền tệ. |

## Giả định của test, nếu implementation chọn khác thì phải đối chiếu spec trước

1. **`TicketTierCard` là controlled component khi có prop `quantity`.** Test `"bam tang hai lan voi quantity={1} co dinh -> ca hai lan deu la 2"` bắt component không giữ bản sao số lượng trong state riêng. Spec §5 cho `quantity?: number` + `onQuantityChange`, đúng khuôn controlled; nhưng spec không nói thẳng. Nếu implementation chọn uncontrolled thì **dừng và đối chiếu spec** — một bản sao số lượng trong UI lệch với đơn hàng là rủi ro tiền.
2. **`fees={0}` tường minh thì được hiện.** AC-8 phân biệt "thiếu" với "có"; `0` là "có". Test này bắt cả lỗi `if (fees)` (0 là falsy).
3. **`imageUrl` toàn khoảng trắng cũng không được thành `<img src="   ">`.** AC-2 chỉ nói "`src` rỗng"; test mở rộng sang khoảng trắng vì hệ quả trên trình duyệt là như nhau (tải lại chính trang hiện tại).
4. **`aria-pressed`/`aria-selected` khi chưa chọn phải là `"false"`, không phải thiếu thuộc tính.** AC-7 nói "`aria-pressed` hoặc `aria-selected` **đúng**"; thiếu thuộc tính nghĩa là control không khai báo mình là toggle.

## Điểm hợp đồng đã được chốt sau vòng chạy đầu (trước đó là mơ hồ trong spec)

Những mục này ban đầu nằm trong báo cáo "spec mơ hồ"; coordinator đã chốt và test đã bám theo.
Ghi lại ở đây để lần sau không phải tranh luận lại.

| Điểm | Chốt |
|---|---|
| Tiêu đề AC-9 ghi "5 trạng thái hold" nhưng union có **6** | Hợp đồng là **6** (`idle creating active expired sold_out ambiguous`); tiêu đề spec là lỗi chữ, sẽ sửa ở spec. |
| `formatMoney` so sánh `currency === "VND"` → `"vnd"` sẽ in `1.250.000 vnd` | `formatMoney` **chuẩn hoá** (trim + uppercase) trước khi so sánh. `"   "` coi như thiếu → `"—"`. Phủ bởi describe "chuan hoa ma tien te". |
| AC-10 "nút chính **luôn hiển thị**" vs AC-8 "`items` rỗng → trạng thái rỗng" | "Luôn hiển thị" là ràng buộc **bố cục mobile** (không bị giấu sau phần thu gọn), **không** phải "luôn bấm được". `items` rỗng → nút **vẫn render** nhưng **disabled**. Phủ bởi describe "AC-8 ∩ AC-10". |
| BR-Q6 "được mời vào **waitlist**" — `OrderSummary` `sold_out` có phải mời waitlist? | **Không.** Nửa sau của BR-Q6 thuộc phòng chờ; `QueueStatusPanel` (slice trước) đã có `onJoinWaitlist`. `OrderSummary` là bề mặt checkout. Không thêm AC. |
| `CANCELLED` nói "hoàn tiền toàn bộ" — có ngụ ý tiền đã về chưa? | **Chưa.** `docs/01` dòng 78: `ISSUED → REFUNDING → REFUNDED`, hoàn tiền là **quá trình có trạng thái**. Copy phải ở thể **quyền lợi** ("Hoàn tiền toàn bộ cho mọi vé đã mua"), không ở thể hoàn thành. Phủ bởi test phủ định `/đã\s*(được\s*)?hoàn\s*(lại\s*)?tiền/i`. |
| BR-E3 + BR-O1: `sold_out` có phải trạng thái một chiều? | **Không.** Quota chỉ được tăng (BR-E3) và job đối soát sửa lại "báo hết vé sớm" (BR-O1), nên `sold_out → available` xảy ra **ngay trong phiên**. AC được mở rộng; phủ bởi ba describe "không một chiều" ở cả ba component. |
| Cảnh báo BR-O3 là tĩnh hay chặn? | **Tĩnh, không chặn.** Khách vẫn đổi được hạng vé, chỉ phải biết trước mình mất hold cũ. |
| `TicketTierCard` controlled hay uncontrolled? | **Controlled** khi có prop `quantity`. Giữ test chống component tự giữ bản sao số lượng. |
| `ambiguous`: "không render nút tạo hold mới" nghĩa là gì khi props chỉ có `onPrimaryAction`? | Không render **nút chính nào**. Suy luận của test khớp hiện thực. |
