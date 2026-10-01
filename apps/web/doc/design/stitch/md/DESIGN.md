---
name: High-Throughput Ticketing Design System
colors:
  surface: '#f8f9ff'
  surface-dim: '#cddbef'
  surface-bright: '#f8f9ff'
  surface-container-lowest: '#ffffff'
  surface-container-low: '#eef4ff'
  surface-container: '#e4efff'
  surface-container-high: '#dbe9fe'
  surface-container-highest: '#d5e4f8'
  on-surface: '#0e1d2b'
  on-surface-variant: '#45474d'
  inverse-surface: '#243241'
  inverse-on-surface: '#e9f1ff'
  outline: '#75777e'
  outline-variant: '#c5c6cd'
  surface-tint: '#535e77'
  primary: '#030e24'
  on-primary: '#ffffff'
  primary-container: '#18243a'
  on-primary-container: '#7f8ba6'
  inverse-primary: '#bbc6e3'
  secondary: '#4c44d8'
  on-secondary: '#ffffff'
  secondary-container: '#6560f2'
  on-secondary-container: '#fffbff'
  tertiary: '#190d00'
  on-tertiary: '#ffffff'
  tertiary-container: '#352000'
  on-tertiary-container: '#bd7f18'
  error: '#ba1a1a'
  on-error: '#ffffff'
  error-container: '#ffdad6'
  on-error-container: '#93000a'
  primary-fixed: '#d7e2ff'
  primary-fixed-dim: '#bbc6e3'
  on-primary-fixed: '#0f1b31'
  on-primary-fixed-variant: '#3b475f'
  secondary-fixed: '#e2dfff'
  secondary-fixed-dim: '#c3c0ff'
  on-secondary-fixed: '#0e006a'
  on-secondary-fixed-variant: '#3529c2'
  tertiary-fixed: '#ffddb4'
  tertiary-fixed-dim: '#ffb955'
  on-tertiary-fixed: '#291800'
  on-tertiary-fixed-variant: '#633f00'
  background: '#f8f9ff'
  on-background: '#0e1d2b'
  surface-variant: '#d5e4f8'
typography:
  headline-xl:
    fontFamily: Inter
    fontSize: 36px
    fontWeight: '700'
    lineHeight: 44px
    letterSpacing: -0.02em
  headline-xl-mobile:
    fontFamily: Inter
    fontSize: 28px
    fontWeight: '700'
    lineHeight: 36px
    letterSpacing: -0.01em
  headline-lg:
    fontFamily: Inter
    fontSize: 28px
    fontWeight: '600'
    lineHeight: 36px
    letterSpacing: -0.015em
  headline-lg-mobile:
    fontFamily: Inter
    fontSize: 24px
    fontWeight: '600'
    lineHeight: 32px
    letterSpacing: -0.01em
  headline-md:
    fontFamily: Inter
    fontSize: 20px
    fontWeight: '600'
    lineHeight: 28px
    letterSpacing: -0.01em
  headline-sm:
    fontFamily: Inter
    fontSize: 16px
    fontWeight: '600'
    lineHeight: 24px
  body-lg:
    fontFamily: Inter
    fontSize: 16px
    fontWeight: '400'
    lineHeight: 24px
  body-md:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: '400'
    lineHeight: 20px
  body-sm:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: '400'
    lineHeight: 18px
  label-lg:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: '600'
    lineHeight: 20px
  label-md:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: '600'
    lineHeight: 16px
  label-sm:
    fontFamily: Inter
    fontSize: 11px
    fontWeight: '600'
    lineHeight: 14px
    letterSpacing: 0.02em
  numeric-metric:
    fontFamily: Inter
    fontSize: 32px
    fontWeight: '700'
    lineHeight: 38px
    letterSpacing: -0.02em
  numeric-timer:
    fontFamily: Inter
    fontSize: 20px
    fontWeight: '700'
    lineHeight: 24px
    letterSpacing: 0.04em
rounded:
  sm: 0.25rem
  DEFAULT: 0.5rem
  md: 0.75rem
  lg: 1rem
  xl: 1.5rem
  full: 9999px
spacing:
  gutter: 1rem
  gutter-mobile: 0.75rem
  margin: 1.5rem
  margin-mobile: 1rem
  space-xs: 0.25rem
  space-sm: 0.5rem
  space-md: 1rem
  space-lg: 1.5rem
  space-xl: 2rem
---

## Brand & Style

### Personality & Core Philosophy
Thiết kế tập trung vào sự bình tĩnh (calm), minh bạch vận hành (operationally clear) và độ tin cậy tuyệt đối (institutional trust). Trong bối cảnh hàng trăm nghìn người cùng tranh vé trong vài phút, giao diện loại bỏ hoàn toàn các yếu tố gây nhiễu, chuyển động giật gân, hoặc hiệu ứng thị giác giả tạo. Mỗi chi tiết phục vụ mục đích truyền tải trạng thái tức thì, giảm tải áp lực tâm lý cho người mua và cung cấp khả năng quan sát chính xác cho đội ngũ vận hành.

### Visual Style
Kết hợp giữa sự tinh gọn hiện đại (Modern Enterprise) và độ tương phản có cấu trúc (Structured Contrast):
- **Bề mặt tối giản:** Nền canvas sáng nhạt (`#F5F7FA`) kết hợp các thẻ thông tin nổi (`#FFFFFF`) giúp phân tách nội dung rõ ràng mà không gây mỏi mắt dưới thời gian chờ đợi kéo dài.
- **Tập trung vào tính sẵn sàng:** Nút tương tác và chỉ số quan trọng (thời gian giữ chỗ, số thứ tự trong hàng chờ) được làm nổi bật với màu sắc tương phản cao, tuân thủ nghiêm ngặt chuẩn WCAG AA.
- **Tính toán trạng thái vận hành minh bạch:** Tách biệt rõ ràng giữa thời gian thực (real-time stream) và dữ liệu tạm thời (stale data indicator), bảo đảm trải nghiệm không gây hiểu lầm khi nghẽn mạng.

## Colors

Hệ màu được định vị để phục vụ các tình huống tải cao, đòi hỏi sự dứt khoát trong việc phân loại hành động và trạng thái giao dịch:

### Palette Structure
- **Deep Navy (`#18243A`):** Màu thương hiệu chủ đạo, sử dụng cho header, cấu trúc điều hướng cốt lõi và các tiêu đề chính nhằm khẳng định tính ổn định, quyền uy của hệ thống.
- **Action Indigo (`#5B55E7`):** Điểm neo hành động tương tác chính (Primary CTA, link trọng yếu, tiến trình bước tiếp theo). Luôn duy trì tỷ lệ tương phản trên 4.5:1 với nền sáng.
- **Pale Indigo (`#EEEDFF`):** Nền làm nổi bật cho các trạng thái chọn, thẻ tab đang kích hoạt, hoặc thông tin hỗ trợ tương tác tích cực.
- **Warm Amber (`#E6A23C`):** Màu trạng thái động cho hàng đợi công bằng (fair queue lobby), đồng hồ đếm ngược giữ vé, cảnh báo không khóa tải.
- **Canvas & Surface:** Nền ứng dụng đồng nhất sử dụng `#F5F7FA` để tạo chiều sâu tự nhiên cho các thẻ nội dung bề mặt `#FFFFFF`.
- **Text & Hierarchy:** Văn bản chính dùng `#182230` cho độ nét tối đa; văn bản phụ dùng `#526071` nhằm hạn chế cạnh tranh thị giác.
- **Semantic Trạng thái:**
  - **Success Green (`#18794E`):** Xác nhận thanh toán thành công, vé đã vào kho an toàn.
  - **Warning Amber (`#9A5B00`):** Cảnh báo hết hạn sắp xảy ra, gián đoạn mạng không nghiêm trọng.
  - **Danger / Critical (`#B42318`):** Hết hạn giữ chỗ, hủy giao dịch, hành động cưỡng chế hủy vé của Ops.

### Nguyên tắc áp dụng
Không sử dụng dải màu gradient cho các nút CTA trọng yếu trong luồng thanh toán hoặc chọn ghế. Mọi chỉ báo về countdown thời gian (Hold TTL, Session TTL) bắt buộc kết hợp văn bản rõ ràng và icon ngữ nghĩa thay vì chỉ dựa vào màu sắc.

## Typography

Hệ thống sử dụng duy nhất kiểu chữ **Inter** với tính năng `tnum` (tabular numbers) được kích hoạt mặc định cho tất cả các trường dữ liệu định lượng: đồng hồ đếm ngược, số vé, tiền tệ VNĐ và vị trí thứ tự hàng đợi (Queue Position).

### Nguyên tắc xử lý Ngôn ngữ Giao diện
1. **Số liệu định thời:** Tách bạch tuyệt đối giữa hai khái niệm:
   - *Thời hạn giữ vé (Hold TTL):* Mặc định 10 phút, đếm ngược tại khâu chọn chỗ/thanh toán.
   - *Phiên truy cập (Session TTL):* Mặc định 15 phút, quy định thời hạn của token xác thực luồng.
2. **Biểu thị số liệu:** Số tiền luôn sử dụng dấu chấm phân cách hàng nghìn và kèm hậu tố `đ` (ví dụ: `1.250.000 đ`).
3. **Tính rõ ràng của trạng thái:** Sử dụng nhãn dứt khoát: "Đang xếp hàng tự động", "Đã khóa 02 vé", "Chờ cổng thanh toán xác nhận". Tránh sử dụng từ ngữ hoa mỹ hoặc câu chữ mơ hồ gây hoang mang trong thời gian tải cao.

## Layout & Spacing

Hệ thống khoảng cách được xây dựng dựa trên nhịp cơ sở 4px/8px, bảo đảm độ chính xác cấu trúc giữa các khối hiển thị và khả năng co giãn linh hoạt trên mọi kích thước màn hình.

### Hệ thống lưới và giới hạn hiển thị
- **Desktop (>= 1280px):** Sử dụng hệ thống lưới 12 cột linh hoạt. Chiều rộng tối đa (Max Content Width) được giới hạn ở `1200px` đối với luồng đặt vé người dùng để thu hẹp tầm mắt, và `1440px` đối với bảng điều khiển vận hành (Ops Dashboard).
- **Tablet (768px - 1279px):** Hệ thống lưới 8 cột, lề ngoài (margin) `1.5rem`, khoảng cách cột (gutter) `1rem`.
- **Mobile (< 768px):** Hệ thống lưới 4 cột, lề ngoài `1rem`, khoảng cách cột `0.75rem`. Toàn bộ các thanh trạng thái đếm ngược giữ vé chuyển thành dạng neo cố định (sticky bottom bar) để tối ưu không gian thao tác ngón cái.

### Quy tắc sắp xếp khoảng cách
- Khoảng cách giữa các thành phần liên quan chặt chẽ (nhãn và trường nhập liệu, mã số ghế và giá vé): sử dụng `space-xs` (4px) hoặc `space-sm` (8px).
- Đệm nội bộ (padding) của card thông tin, bảng tóm tắt đơn hàng: sử dụng `space-md` (16px) trên thiết bị di động và `space-lg` (24px) trên desktop.
- Khoảng cách giữa các khối chức năng độc lập (Lobby queue progress, khu vực sơ đồ ghế, thông tin thanh toán): sử dụng `space-xl` (32px).

## Elevation & Depth

Thay vì dùng bóng đổ phân tán phong cách thương mại, hệ thống áp dụng kỹ thuật xếp tầng bề mặt (Tonal Layers) phối hợp cùng viền tương phản thấp (Ghost Borders) và bóng đổ tinh tế có chủ đích:

### Các cấp độ nổi
- **Cấp độ 0 (Flat Canvas):** Nền `#F5F7FA`, không có bóng, tạo phông nền vững chãi.
- **Cấp độ 1 (Base Container / Card):** Nền trắng `#FFFFFF`, viền mỏng `1px solid #E4E7EC`, bóng đổ siêu nhẹ: `0px 1px 3px rgba(16, 24, 40, 0.05)`. Sử dụng cho danh sách sự kiện, thẻ vé, bảng chi tiết giao dịch.
- **Cấp độ 2 (Floating Status Bar / Controls):** Nền `#FFFFFF`, viền `1px solid #D0D5DD`, bóng đổ: `0px 4px 12px -2px rgba(16, 24, 40, 0.08)`. Sử dụng cho thanh đếm ngược cố định chân trang (Sticky Checkout Summary), popover chi tiết sơ đồ ghế.
- **Cấp độ 3 (Modal / Queue Lobby Alert):** Nền `#FFFFFF`, bóng đổ rộng: `0px 12px 32px -4px rgba(16, 24, 40, 0.14)`. Sử dụng cho cửa sổ xác nhận hủy phiên, hộp thoại cảnh báo thanh toán timeout.

Mọi phần tử đè lên bản đồ ghế (Seatmap overlay) hoặc thông tin hàng đợi bắt buộc dùng thuộc tính backdrop-filter làm mờ nền nhẹ (`backdrop-blur: 8px`) với nền trắng độ trong suốt 90% (`rgba(255, 255, 255, 0.9)`) để giữ tính tập trung.

## Shapes

Ngôn ngữ hình học thể hiện tính kỹ thuật, cân bằng và thân thiện nhưng nghiêm cẩn:

### Quy chuẩn bo góc
- **10px (`--radius-control`):** Áp dụng đồng bộ cho tất cả các thành phần tương tác: Nút bấm (Button), trường nhập liệu (Input field), ô chọn (Dropdown), thẻ chọn nhanh loại vé (Ticket tier chips). Kích thước này tạo sự êm ái hơn góc vuông cổ điển nhưng không biến thành dạng tròn bong bóng thiếu nghiêm túc.
- **16px (`--radius-panel`):** Áp dụng cho các thùng chứa thông tin lớn: Khối thẻ thông tin sự kiện (Event Card), khung hiển thị bản đồ chỗ ngồi (Seatmap Container), hộp thoại Modal hàng đợi (Lobby Modal).
- **Trường hợp ngoại lệ đặc biệt:**
  - **Pill (Full-radius):** Chỉ áp dụng duy nhất cho các thẻ tag trạng thái vận hành ngắn gọn (Status Badges) như "ĐANG MỞ BÁN", "HẾT VÉ", "GIỮ CHỖ THÀNH CÔNG".
  - **Sharp (0px):** Không áp dụng cho bất kỳ khối tương tác trực quan nào.

## Components

### 1. Nút bấm (Buttons)
- **Primary Action (Action Indigo):** Nền `#5B55E7`, chữ trắng, bo góc 10px, chiều cao chuẩn 48px trên di động và 44px trên desktop. Trạng thái hover làm tối nhẹ 8% (`#4A44D4`), active thu nhỏ nhẹ 0.99x. Khi ở trạng thái chờ xử lý (isPending), hiển thị spinner vô định hình màu trắng, vô hiệu hóa click nhưng duy trì kích thước để tránh lay-out shift.
- **Critical / Danger Action:** Nền trong suốt với viền và chữ `#B42318`, hoặc nền đặc `#B42318` với chữ trắng chỉ khi áp dụng cho các thao tác Ops không thể khôi phục (Hủy lô vé đang giữ, cưỡng chế ngắt hàng chờ).
- **Secondary / Ghost:** Nền trong suốt, viền `1px solid #D0D5DD`, chữ `#182230`.

### 2. Thẻ vé & Hộp thông tin (Cards & Containers)
- **Event / Tier Card:** Bo góc 16px, viền 1px `#E4E7EC`. Khi một hạng vé được người dùng chọn, viền chuyển ngay lập tức sang `#5B55E7` dày 2px, nền phủ nhẹ một lớp `#EEEDFF` với độ mờ 40%.
- **Thẻ hết vé (Sold-out):** Nền giảm tương phản nhẹ, các chi tiết chữ giảm độ mờ (opacity 0.6), hiển thị nhãn pill "Hết vé" xám `#526071` thay vì ẩn hoàn toàn thẻ để bảo toàn bố cục trang.

### 3. Đồng hồ đếm ngược & Hàng đợi (Fair Queue Lobby Components)
- **Lobby Progress Card:** Hiển thị vị trí người dùng trong hàng đợi (Queue Position) với cỡ chữ `numeric-metric`, nhãn "Số thứ tự của bạn". Bên dưới là thanh tiến trình (progress bar) màu Action Indigo, kèm ước tính thời gian chờ trung bình dạng văn bản ổn định ("Thời gian ước tính: ~4 phút").
- **Hold TTL Countdown Widget:** Nằm cố định ở đầu màn hình hoặc dải bottom bar. Chữ số đếm ngược dùng font Inter Tabular (`numeric-timer`). Khi thời gian còn dưới 2 phút, widget tự động chuyển viền và nhãn sang Warm Amber (`#E6A23C`) kèm icon đồng hồ cảnh báo để người dùng nhanh chóng hoàn tất thanh toán.

### 4. Ô nhập liệu (Input Fields) & Form
- Chiều cao chuẩn 44px, bo góc 10px, nền trắng, viền `#D0D5DD`.
- Trạng thái Focus: Viền `#5B55E7`, hiệu ứng viền ngoài (focus-ring) mờ `rgba(91, 85, 231, 0.2)` dày 3px.
- Trạng thái Error: Viền `#B42318`, kèm thông báo lỗi cụ thể bên dưới bằng chữ `body-sm` đỏ.

### 5. Thành phần trạng thái đồng bộ & Vận hành (Async State Indicators)
- **Stale Data Warning Banner:** Xuất hiện dải ruy băng trên đỉnh màn hình màu `#E6A23C` khi kết nối socket bị ngắt quãng: "Đang cập nhật lại trạng thái ghế theo thời gian thực...".
- **Demo / Sandbox Indicator:** Dành cho môi trường diễn tập Ops, hiển thị viền sọc chéo tinh tế màu vàng đen tại góc màn hình để phân biệt tuyệt đối với môi trường thực tế.