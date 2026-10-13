# EV-128 — Truy vết AC → test

Tên test tích hợp có tiền tố `TestTichHop_` (chạy qua `run-and-report-tests.sh` với `REQUIRE_PASS=1`).

| AC | Test đơn vị (`libs/go/idem/middleware_test.go`) | Test tích hợp (`integration_test.go`) | Khác |
|---|---|---|---|
| AC-1 | `TestThieuHoacSaiKhoa_Tra400VaKhongChayHandler`, `TestKhoaDoi_Tra400` | — | `handler_test.go`: `TestCreateHold_ThieuIdempotencyKey_400VaKhongTaoHold`; `smoke.sh` bước 6 |
| AC-2 | `TestGoiLai_TraDungResponseCuVaKhongChayLai`, `TestLoiNghiepVu4xx_DuocNhoDeKhachKhongBiLamPhienLai`, `TestKhacDuongDan_LaHaiKhoaDocLap`, `TestHetTTL_KhoaDuocDungLaiNhuMoi` | `TestTichHop_GoiLaiTraDungResponseCuTungByte`, `TestTichHop_HetHan_KhoaDuocDungLaiVoiNoiDungKhac` | `handler_test.go`: `TestCreateHold_GoiLaiCungKey_…`; `smoke.sh` "goi lai cung key" |
| AC-3 | `TestDongThoiCungKhoa_ChiMotHandlerChay` | `TestTichHop_DongThoiCungKhoaChiMotHandlerChay` | — |
| AC-4 | `TestCungKhoaKhacBody_Tra422`, `TestCungKhoaKhacNguoi_Tra422VaKhongLoResponse`, `TestHashPhanBietRanhGioiTruong` | `TestTichHop_KhacBodyHoacKhacNguoi_Tra422`, `TestTichHop_DinhDanhKhongPhaiUUID_…` | `smoke.sh` "cung key khac noi dung" |
| AC-5 | `TestLoi5xx_XoaKhoaDeThuLaiThatSu`, `TestPanic_XoaKhoaVaDePanicDiTiep` | `TestTichHop_Loi5xxVaPanic_XoaKhoa` | `handler_test.go`: `TestCreateHold_LoiHeThong503_KhongBiKhoaVaoKey` |
| AC-6 | `TestClientNgatGiuaChung_VanLuuResponse`, `TestQuaLease_RequestSauTiepQuanVaChuCuKhongGhiDe` | `TestTichHop_ClientNgat_VanLuuResponse`, `TestTichHop_QuaLease_TiepQuanVaChuCuKhongGhiDe` | — |
| AC-7 | `TestStoreLoi_Tra503VaKhongChayHandler` | `TestTichHop_StoreLoi_Tra503` | — |
| AC-8 | — | — | `handler_test.go` (qua `Routes(mux, idempotency)`); `smoke.sh` 18/18 |
| Web | — | — | `hold-client.test.ts`: 409 `idempotency-in-progress` → `AMBIGUOUS`; 409 khác → `SOLD_OUT` |
