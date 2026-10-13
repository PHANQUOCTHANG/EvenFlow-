# EVF-36 — Ma trận truy vết AC → test

Ký hiệu: **U** = unit (không DB/broker), **I** = tích hợp (`//go:build integration`, Postgres 16 + RabbitMQ 3.13 thật, chạy ở CI).
Đường dẫn rút gọn: `ob/` = `libs/go/outbox/`, `tk/` = `services/ticketing/`.

| AC | Yêu cầu | Test | Loại | Kết quả (verify 87969db) |
|---|---|---|---|---|
| AC-1 | `hold.created` ghi cùng tx với PersistHold | `tk/internal/adapter/postgres/repository_integration_test.go`: `TestTichHop_PersistHoldGhiOutboxHoldCreatedCungTransaction` | I | PASS — CI run 38354241696 |
| AC-1 | Rollback ⇒ không có row | `TestTichHop_PersistHoldRollbackThiKhongCoRowOutbox` (vi phạm `uq_active_order_per_identity_event`, hết tồn kho) | I | PASS — CI run 38354241696 |
| AC-1 | `hold.released` ghi cùng tx với ReleaseHold, đúng một lần | `TestTichHop_ReleaseHoldGhiOutboxHoldReleasedMotLan`, `TestTichHop_ReleaseHoldLoiThiKhongCoRowOutbox`, `TestTichHop_ReleaseHoldDaConsumeKhongGhiOutbox` | I | PASS — CI run 38354241696 |
| AC-1 | Sweeper không publish thẳng | `tk/internal/worker/hold_sweeper_test.go` (chữ ký `NewHoldSweeper` không còn publisher; `TestHoldSweeper_BRO7_…`, `TestHoldSweeper_BRO1_…`, `TestHoldSweeper_LoiMotSuKien…`, `TestHoldSweeper_Metrics_…`) | U | PASS — local `-race -count=3` |
| AC-2 | `event_id` UUID v4 duy nhất, payload chỉ là `data` | `ob/write_test.go`: `TestWrite_GhiMotRowVoiEventIDVaPayloadChiLaData` | U | PASS — local `-race -count=3` |
| AC-2 | Phong bì `{event_id, occurred_at, trace_id, schema_version:1, type, data}`, `MessageId = event_id` | `ob/relay_test.go`: `TestRunOnce_ThanhCongPublishPhongBiVaDanhDau`; `ob/write_test.go`: `TestEnvelope_TenTruongJSON`; `ob/integration_test.go`: `TestTichHop_AC3_PublishVaDanhDau` | U, I | PASS — local `-race -count=3` + CI run 38354241696 |
| AC-2 | `sale_event_id` thay `event_id` trong data; không PII | `TestTichHop_PersistHoldGhiOutboxHoldCreatedCungTransaction`, `TestTichHop_ReleaseHoldGhiOutboxHoldReleasedMotLan` (`assertKeys` cấm `event_id`, `identity_id`); r3 hold id dạng chuẩn | I | PASS — CI run 38354241696 |
| AC-2 / M3 | Row cũ trước 0002 vẫn được bọc đúng phong bì | `TestRunOnce_ThanhCongPublishPhongBiVaDanhDau` (row cũ); `TestTichHop_AC8_RowCuTruoc0002DuocBocPhongBi` | U, I | PASS — local `-race -count=3` + CI run 38354241696 |
| AC-2 / m6 | Kiểm tra đầu vào: routing key, aggregate UUID, data ≤ 64 KiB | `TestWrite_KiemTraDauVao`, `TestWrite_TraLoiCuaExec` | U | PASS — local `-race -count=3` |
| AC-3 | Claim `SKIP LOCKED`, `ORDER BY next_attempt_at, id`, publish + đánh dấu `published_at` | `TestRunOnce_ThanhCongPublishPhongBiVaDanhDau`; `TestTichHop_AC3_PublishVaDanhDau` | U, I | PASS — local `-race -count=3` + CI run 38354241696 |
| AC-3 / R1 | Lỗi tạm tăng `attempts`, ghi `last_error`, không chặn row khác | `TestRunOnce_ErrNackedTangAttemptsGhiLastError`, `TestRunOnce_HetPublishTimeoutTangAttemptsKhiKenhConMo` | U | PASS — local `-race -count=3` |
| AC-3 / r1 | Mọi UPDATE đánh dấu có `AND published_at IS NULL` | `assertMarksGuarded` trong mọi test `RunOnce` | U | PASS — local `-race -count=3` |
| AC-3 / r2 | Backoff có trần, kẹp số mũ, jitter ±20%, attempts 0/1/21/MaxInt32 | `TestBackoff_LuyThuaCoTranVaJitter`, `TestBackoff_SoMuKepO20`, `TestBackoff_MaxMotGioKhongTran` | U | PASS — local `-race -count=3` |
| AC-3 / M4 | `MaxAttempts` ⇒ đỗ row `'infinity'`, giữ `last_error` | `TestRunOnce_LanThuThuMaxAttemptsThatBaiThiDoRow` | U | PASS — local `-race -count=3` |
| AC-3 / M2 | `last_error` sạch (UTF-8, NUL, ≤ 1 KiB) | `TestSanitizeErr_LamSachLastError` | U | PASS — local `-race -count=3` |
| AC-3 / r7 | Panic ở một row ⇒ recover, coi là lỗi | `TestRunOnce_PanicTrongMotRowDuocRecoverThanhLoi` | U | PASS — local `-race -count=3` |
| AC-3 / M1 | Lỗi DB khi đánh dấu ⇒ dừng lô | `TestRunOnce_LoiDBKhiDanhDauThiDungLo` | U | PASS — local `-race -count=3` |
| AC-3 / r1 | Hết `BatchDeadline` ⇒ trả lease phần còn lại, không tăng attempts | `TestRunOnce_HetBatchDeadlineTraLeasePhanCon` | U | PASS — local `-race -count=3` |
| AC-3 | Hai relay song song không gửi trùng trong một lease | `TestTichHop_HaiRelaySongSongKhongGuiTrungTrongMotLease` | I | PASS — CI run 38354241696 |
| AC-4 / R1 | `ErrClosed` / ctx huỷ: không tăng attempts, trả lease, vẫn đánh dấu phần đã gửi (WithoutCancel) | `TestRunOnce_ErrClosedTraLeaseKhongTangAttemptsRoiDialLai`, `TestRunOnce_HuyCtxVanDanhDauPhanDaGuiVaTraLeasePhanCon` | U | PASS — local `-race -count=3` |
| AC-4 | Kill-restart: đóng cứng AMQP giữa lô, relay mới gửi nốt; mọi `event_id` ≥ 1 lần, trùng ≤ lô | `TestTichHop_AC4_DongCungAMQPGiuaLoRoiKhoiDongLai` | I | PASS — CI run 38354241696 |
| AC-4 / R2 | Đã publish nhưng chưa đánh dấu (`pg_terminate_backend` kết nối relay) ⇒ gửi lại sau lease | `TestTichHop_AC4_R2_DaPublishChuaDanhDauThiGuiLai` | I | PASS — CI run 38354241696 |
| AC-4 | RabbitMQ `stop_app`/`start_app` khi relay chạy ⇒ không mất `hold.released`, không đỗ row | `TestTichHop_AC4_RabbitMQDungRoiChayLaiKhongMatHoldReleased` | I | PASS — CI run 38354241696 |
| AC-5 | Trace lưu cùng row (`headers.traceparent`, `trace_id`) | `TestWrite_LuuTraceparentVaTraceIDKhiCoSpan`; `TestTichHop_PersistHoldLuuTraceCuaRequest` | U, I | PASS — local `-race -count=3` + CI run 38354241696 |
| AC-5 | Trace khôi phục khi publish, consumer cùng trace ID | `TestRunOnce_ThanhCongPublishPhongBiVaDanhDau` (ctx publish); `TestTichHop_AC5_TraceIDGiuNguyenTuGhiOutboxToiConsumer` | U, I | PASS — local `-race -count=3` + CI run 38354241696 |
| AC-6 / M5 | Unroutable trong `DropUnroutable` ⇒ `published_at` + `last_error='unroutable'` + metric | `TestRunOnce_UnroutableTrongDanhSachDuocDanhDauBo`; `TestTichHop_AC6_UnroutableChiBoKeyTrongDanhSach` | U, I | PASS — local `-race -count=3` + CI run 38354241696 |
| AC-6 / M5 | Unroutable ngoài danh sách ⇒ lỗi tạm, sửa binding là gửi được | `TestRunOnce_UnroutableNgoaiDanhSachThuLai`; `TestTichHop_AC6_UnroutableChiBoKeyTrongDanhSach` | U, I | PASS — local `-race -count=3` + CI run 38354241696 |
| AC-6 | Worker chỉ bỏ `ticketing.hold.created` | `tk/cmd/worker/main_test.go`: `TestRelayConfig_ChiBoUnroutableChoHoldCreated` | U | PASS — local `-race -count=3` |
| AC-7 / B1 | Relay không kết nối được: `Run` không trả về tới khi ctx huỷ, gauge `connected` = 0, không claim | `TestRun_DialerLuonLoiKhongTraVeToiKhiHuyCtx`, `TestRunOnce_DialLoiThiKhongClaim`, `TestRun_HuyCtxTraNilVaDongKetNoi` | U | PASS — local `-race -count=3` |
| AC-7 / r6 | Worker: sweeper vẫn chạy khi relay không kết nối được | `TestRunLoops_RelayKhongKetNoiDuocSweeperVanChay`, `TestRunLoops_MotVongLapLoiThiDungTatCa`, `TestAmqpDialer_BrokerKhongToiDuocTraLoi` | U | PASS — local `-race -count=3` |
| AC-7 | Thiếu/sai `AMQP_URL` ⇒ fail fast, không lộ mật khẩu | `TestLoadConfig_AMQPURLHopLe`, `TestLoadConfig_ThieuAMQPURLFailFast`, `TestLoadConfig_AMQPURLSaiCuPhapKhongLoMatKhau` | U | PASS — local `-race -count=3` |
| AC-7 / r8 | Pool worker dùng `QueryExecModeCacheDescribe` | `TestNewPoolConfig_DungCacheDescribeChoPgBouncer` | U | PASS — local `-race -count=3` |
| AC-7 | Cấu hình relay hợp lệ (bất biến lease, giới hạn) | `TestNewRelay_KiemTraCauHinh`, `TestNewRelay_ThieuDBHoacDialerLaLoi`, `TestNewRelay_MacDinhThoaBatBienLease` | U | PASS — local `-race -count=3` |
| AC-8 | INSERT kiểu cũ chạy trên 0002, nhận `event_id`, UNIQUE có hiệu lực | `TestTichHop_AC8_InsertKieuCuChayTrenSchema0002VaNhanEventID` | I | PASS — CI run 38354241696 |
| AC-8 / r4 | Up → Down → Up trong tx tường minh; sau Down cột/index/constraint = 0001; dữ liệu giữ | `TestTichHop_AC8_UpDownUpTraDungSchema0001` | I | PASS — CI run 38354241696 |
| AC-8 / M7 | `lock_timeout` ⇒ migration bỏ cuộc (`55P03`) khi bảng đang bị khoá | `TestTichHop_AC8_Migration0002BoCuocKhiBangDangBiKhoa` | I | PASS — CI run 38354241696 |
| AC-9 | `go vet`, `-race`, lint, CI xanh | Gate của bước verify (không phải test riêng); xem `baseline.md` §3 | — | PASS — gate verify (xem evidence.md §2) |

### Bổ sung sau code review (plan §8)

| Finding | Yêu cầu | Test | Loại | Kết quả (verify 87969db) |
|---|---|---|---|---|
| m1 | `NewRelay` từ chối `BatchDeadline + PublishTimeout + 2×5s >= Lease` | `TestNewRelay_KiemTraCauHinh` (4 case mới; `testConfig` Lease 15s, `itConfig` Lease 14s, test hạn lô Lease 11s) | U | PASS — local `-race -count=3` |
| m1 | Publish bị cắt tại hạn chót lô (ctx publish không vượt `BatchDeadline`) | `TestRunOnce_PublishBiCatTaiHanChotLo` | U | PASS — local `-race -count=3` |
| m1 | Trả lease không gỡ đỗ row `'infinity'` (`AND next_attempt_at <> 'infinity'`) | `TestRunOnce_TraLeaseKhongGoDoRowDaDo`; `assertMarksGuarded` kiểm mọi câu trả lease | U | PASS — local `-race -count=3` |
| m3 | Lỗi publish không phân loại ⇒ `attempts + 1`, ghi `last_error` | `TestRunOnce_LoiKhongPhanLoaiTangAttempts` | U | PASS — local `-race -count=3` |
| m4 | Kết nối báo `IsClosed()` trước claim ⇒ đóng, gauge 0, không claim/publish trên kết nối chết, dial lại | `TestRunOnce_KetNoiDaDongTruocClaimThiKhongClaimVaDialLai` | U | PASS — local `-race -count=3` |
| m5 | Chỉ cắt TCP (không huỷ ctx) ⇒ nhánh `ErrClosed`, relay tự dial lại, mọi event tới | `TestTichHop_AC4_ChiCatTCPRelayTuDialLaiGuiHet` | I | PASS — CI run 38354241696 |
| m8 | Metric published/lag không tăng khi UPDATE đánh dấu lỗi | `TestRunOnce_DanhDauLoiThiKhongTangMetricPublished` | U | PASS — local `-race -count=3` |
| m8 | `data.released_at` của `hold.released` = `seat_holds.released_at` (giờ DB) | `TestTichHop_ReleaseHoldGhiOutboxHoldReleasedMotLan` | I | PASS — CI run 38354241696 |

### Bổ sung sau challenge (plan §9)

| Finding / mutant | Yêu cầu | Test | Loại | Kết quả (verify 87969db) |
|---|---|---|---|---|
| F1 | Câu thử lại / đỗ không đụng row relay khác đã đỗ (`AND next_attempt_at <> 'infinity'`) | `TestRunOnce_ThuLaiVaDoKhongDungRowDaBiRelayKhacDo`; `assertMarksGuarded` đòi guard ở thử lại, đỗ, trả lease | U | PASS — local `-race -count=3` |
| F2 | `Write` từ chối data có `\u0000` | `TestWrite_KiemTraDauVao` (2 case F2) | U | PASS — local `-race -count=3` |
| F3, M42 | Lag theo đồng hồ DB trả từ claim; âm kẹp về 0 | `TestRunOnce_LagTheoDongHoDB`, `TestRunOnce_LagAmKepVe0` | U | PASS — local `-race -count=3` |
| M22, M48 | Huỷ ctx giữa publish: không tính attempts, trả lease cả row đó | `TestRunOnce_HuyCtxGiuaPublishKhongTinhAttempts` | U | PASS — local `-race -count=3` |
| M23, M47 | Hết hạn lô giữa publish: không tính attempts, trả lease | `TestRunOnce_PublishBiCatTaiHanChotLo` (thêm khẳng định) | U | PASS — local `-race -count=3` |
| M29 | SQL gauge tách row đỗ khỏi `outbox_pending` | `TestRun_GaugePendingKhongTinhRowDo` | U | PASS — local `-race -count=3` |
| M30 | Backoff dial lại tăng dần (seam `Relay.redialBase/redialMax`) | `TestRun_BackoffDialLaiTangDan` | U | PASS — local `-race -count=3` |
| M31 | Lô thành công (kể cả rỗng) đặt lại số lần thất bại | `TestRun_LoThanhCongDatLaiBackoff` | U | PASS — local `-race -count=3` |
| M36 | `SET LOCAL statement_timeout` trong tx trước claim | `TestRunOnce_ClaimCoStatementTimeoutTrongTx` | U | PASS — local `-race -count=3` |
| M43 | Lỗi tạm tăng `outbox_publish_errors_total{routing_key}`; huỷ ctx thì không | `TestRunOnce_LoiTamTangPublishErrorsTotal`, `TestRunOnce_HuyCtxGiuaPublishKhongTinhAttempts` | U | PASS — local `-race -count=3` |
| W1 | Biên đúng 64 KiB: 65536 byte được, 65537 bị từ chối | `TestWrite_KiemTraDauVao` | U | PASS — local `-race -count=3` |
| W6 | `AggregateType` rỗng bị từ chối | `TestWrite_KiemTraDauVao` | U | PASS — local `-race -count=3` |
| W7 | Headers giữ đủ `traceparent`, `tracestate`, `baggage`, bỏ khoá khác | `TestWrite_HeadersGiuDuBaKhoaW3CVaBoKhoaKhac` | U | PASS — local `-race -count=3` |

## Chưa phủ / ghi chú

- Metric `outbox_pending`, `outbox_dead`, `outbox_publish_lag_seconds` chỉ được kiểm gián tiếp (không test tên/giá trị).
- Compose (`ticketing-worker` có `AMQP_URL`, `depends_on`) và goose thật (r9): kiểm ở bước smoke khi có Docker, không có test tự động.
- Thứ tự event không được bảo đảm (plan §6.2) — có chủ đích, không test.
