# EV-114 — Truy vết AC → test

Unit: `services/event/internal/adapter/http/authz_test.go` (`Z:`), `handler_test.go` (`H:`, test cũ, nay gửi danh tính
như gateway).

| AC | Test |
|---|---|
| AC-1 ma trận role × route | Z: `MaTranQuyenTheoRole` — 9 route × 5 trường hợp (không danh tính, buyer, organizer, moderator, ops) = 45 ca; ca bị cấm kiểm 403 + trạng thái không đổi + audit `ACCESS_DENIED` |
| AC-2 cross-tenant | Z: `OrganizerKhacKhongDungDuocSuKienNguoiKhac` (sửa, thêm hạng vé, huỷ) |
| AC-3 id từ token | Z: `GiaDanhQuaBodyBiChan` (organizer_id lệch, không gửi, moderator_id lệch) |
| AC-4 moderator từ token | Z: `DuyetGhiModeratorTuToken` · H: `ModeratorApprove_And_Reject` |
| AC-5 ẩn sự kiện chưa duyệt | Z: `SuKienChuaDuyetBiAn` (6 vai × id/slug, sự kiện đã duyệt công khai) · H: `CreateAndGetEvent` |
| AC-6 danh sách | Z: `DanhSachSuKienTheoToken` |
| AC-7 header hỏng / 500 | Z: `HeaderDanhTinhHongLa401`, `Loi500KhongLoChiTiet` |
