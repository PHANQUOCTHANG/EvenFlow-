# EV-121 — Truy vết AC → test

Go (`services/event/internal/`): `A:` = `app/moderation_action_test.go`, `D:` = `app/moderation_detail_test.go`,
`H:` = `adapter/http/moderator_workspace_test.go`, `HT:` = `adapter/http/handler_test.go`,
`P:` = `adapter/postgres/decision_store_integration_test.go` (Postgres thật).
Web (`apps/web/src/`): `S:` = `lib/moderation-service.contract.test.ts`, `Q:` = `app/(moderator)/moderator/reviews/page.test.tsx`,
`R:` = `app/(moderator)/moderator/reviews/[eventId]/page.test.tsx`, `N:` = `components/layout/nav-config.test.ts`.

| AC | Test |
|---|---|
| AC-1 lật ngược AI cần lý do, ghi audit | A: `LatNguocAI_ThieuLyDo_BiTuChoi`, `LatNguocAI_CoLyDo_GhiAuditRieng`, `LatNguocQuyetDinhNguoi_CanLyDo`, `Reject_MissingReason_Fails` · H: `LatNguocAI_ThieuLyDo_422CoType` · S: `422 override-reason-required` · R: `lật ngược AI…`, `422 override-reason-required…`, `từ chối bắt buộc lý do…` |
| AC-2 chỉ Moderator | H: `ChiModeratorQuyetDinhDuoc`, `HoSoThamDinh_…` (organizer 403) · S: `403 -> ApiError` · Q: `403 từ API…`, `vai trò khác…/unauthorized` · R: `403: không có quyền…` |
| AC-3 một transaction | A: `ModeratorDecision_GhiHong_TraLoiKhongNuot` · P: `ApplyDecision_GhiCaBaTrongMotTransaction`, `ApplyDecision_AuditHong_KhongGhiGi` |
| AC-4 hàng đợi gồm AI đã từ chối | A: `ListModerationQueue_GomCanNguoiDuyetVaAIDaTuChoi`, `DaQuyetThiKhongConTrongHang` · P: `ListPendingHuman_ReviewMoiNhatCuaMoiSuKien` · H: `HangDoi_CoCaSuKienAIDaTuChoi` · HT: `TestHTTP_ModerationQueue` |
| AC-5 API hồ sơ thẩm định | D: `GomSuKienReviewTrucVaAudit`, `ChuaCoReviewVaTruc_VanTraVe`, `KhongTonTai` · H: `HoSoThamDinh_TachNoiDungGocVaAI` · S: `getModerationDetail` (3 test) |
| AC-6 UI hàng đợi | Q: liệt kê, thẻ chỉ số, đánh dấu lật ngược, lọc, tìm không dấu, lọc rỗng, hàng đợi trống, lỗi + thử lại |
| AC-7 UI thẩm định | R: tách gốc / AI, 4 trục, lịch sử audit, duyệt có xác nhận, khoá khi đã xử lý, không tìm thấy, không có review AI · N: `MODERATOR_NAV` |
| AC-8 AI không phải người quyết | R: `AI chỉ là khuyến nghị: có câu miễn trừ, không có nút chấp nhận theo AI` |
| AC-9 hai Moderator cùng quyết | P: `ApplyDecision_HaiQuyetDinhDongThoi_MotThang`, `ApplyDecision_TrangThaiDaDoi_BiKhoa` · A: `NguoiDenSau_LaDaQuyetChuKhongPhaiLoiTrangThai`, `Reject_SuKienDaBiTuChoi_LaDaQuyet`, `TranhChap_TraLoiDaQuyet` · H: `QuyetDinhBiTranhChap_409CoType` · S: `409 already-decided` · R: `409: …tải lại` · chạy thật evidence §2 |
