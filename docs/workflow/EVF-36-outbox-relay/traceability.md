# EVF-36 — Ma trận truy vết AC → test

Ký hiệu: **U** = unit (không DB/broker), **I** = tích hợp (`//go:build integration`, Postgres 16 + RabbitMQ 3.13 thật, chạy ở CI).
Đường dẫn rút gọn: `ob/` = `libs/go/outbox/`, `tk/` = `services/ticketing/`.

| AC | Yêu cầu | Test | Loại |
|---|---|---|---|
| AC-1 | `hold.created` ghi cùng tx với PersistHold | `tk/internal/adapter/postgres/repository_integration_test.go`: `TestTichHop_PersistHoldGhiOutboxHoldCreatedCungTransaction` | I |
| AC-1 | Rollback ⇒ không có row | `TestTichHop_PersistHoldRollbackThiKhongCoRowOutbox` (vi phạm `uq_active_order_per_identity_event`, hết tồn kho) | I |
| AC-1 | `hold.released` ghi cùng tx với ReleaseHold, đúng một lần | `TestTichHop_ReleaseHoldGhiOutboxHoldReleasedMotLan`, `TestTichHop_ReleaseHoldLoiThiKhongCoRowOutbox`, `TestTichHop_ReleaseHoldDaConsumeKhongGhiOutbox` | I |
| AC-1 | Sweeper không publish thẳng | `tk/internal/worker/hold_sweeper_test.go` (chữ ký `NewHoldSweeper` không còn publisher; `TestHoldSweeper_BRO7_…`, `TestHoldSweeper_BRO1_…`, `TestHoldSweeper_LoiMotSuKien…`, `TestHoldSweeper_Metrics_…`) | U |
| AC-2 | `event_id` UUID v4 duy nhất, payload chỉ là `data` | `ob/write_test.go`: `TestWrite_GhiMotRowVoiEventIDVaPayloadChiLaData` | U |
| AC-2 | Phong bì `{event_id, occurred_at, trace_id, schema_version:1, type, data}`, `MessageId = event_id` | `ob/relay_test.go`: `TestRunOnce_ThanhCongPublishPhongBiVaDanhDau`; `ob/write_test.go`: `TestEnvelope_TenTruongJSON`; `ob/integration_test.go`: `TestTichHop_AC3_PublishVaDanhDau` | U, I |
| AC-2 | `sale_event_id` thay `event_id` trong data; không PII | `TestTichHop_PersistHoldGhiOutboxHoldCreatedCungTransaction`, `TestTichHop_ReleaseHoldGhiOutboxHoldReleasedMotLan` (`assertKeys` cấm `event_id`, `identity_id`); r3 hold id dạng chuẩn | I |
| AC-2 / M3 | Row cũ trước 0002 vẫn được bọc đúng phong bì | `TestRunOnce_ThanhCongPublishPhongBiVaDanhDau` (row cũ); `TestTichHop_AC8_RowCuTruoc0002DuocBocPhongBi` | U, I |
| AC-2 / m6 | Kiểm tra đầu vào: routing key, aggregate UUID, data ≤ 64 KiB | `TestWrite_KiemTraDauVao`, `TestWrite_TraLoiCuaExec` | U |
| AC-3 | Claim `SKIP LOCKED`, `ORDER BY next_attempt_at, id`, publish + đánh dấu `published_at` | `TestRunOnce_ThanhCongPublishPhongBiVaDanhDau`; `TestTichHop_AC3_PublishVaDanhDau` | U, I |
| AC-3 / R1 | Lỗi tạm tăng `attempts`, ghi `last_error`, không chặn row khác | `TestRunOnce_ErrNackedTangAttemptsGhiLastError`, `TestRunOnce_HetPublishTimeoutTangAttemptsKhiKenhConMo` | U |
| AC-3 / r1 | Mọi UPDATE đánh dấu có `AND published_at IS NULL` | `assertMarksGuarded` trong mọi test `RunOnce` | U |
| AC-3 / r2 | Backoff có trần, kẹp số mũ, jitter ±20%, attempts 0/1/21/MaxInt32 | `TestBackoff_LuyThuaCoTranVaJitter`, `TestBackoff_SoMuKepO20`, `TestBackoff_MaxMotGioKhongTran` | U |
| AC-3 / M4 | `MaxAttempts` ⇒ đỗ row `'infinity'`, giữ `last_error` | `TestRunOnce_LanThuThuMaxAttemptsThatBaiThiDoRow` | U |
| AC-3 / M2 | `last_error` sạch (UTF-8, NUL, ≤ 1 KiB) | `TestSanitizeErr_LamSachLastError` | U |
| AC-3 / r7 | Panic ở một row ⇒ recover, coi là lỗi | `TestRunOnce_PanicTrongMotRowDuocRecoverThanhLoi` | U |
| AC-3 / M1 | Lỗi DB khi đánh dấu ⇒ dừng lô | `TestRunOnce_LoiDBKhiDanhDauThiDungLo` | U |
| AC-3 / r1 | Hết `BatchDeadline` ⇒ trả lease phần còn lại, không tăng attempts | `TestRunOnce_HetBatchDeadlineTraLeasePhanCon` | U |
| AC-3 | Hai relay song song không gửi trùng trong một lease | `TestTichHop_HaiRelaySongSongKhongGuiTrungTrongMotLease` | I |
| AC-4 / R1 | `ErrClosed` / ctx huỷ: không tăng attempts, trả lease, vẫn đánh dấu phần đã gửi (WithoutCancel) | `TestRunOnce_ErrClosedTraLeaseKhongTangAttemptsRoiDialLai`, `TestRunOnce_HuyCtxVanDanhDauPhanDaGuiVaTraLeasePhanCon` | U |
| AC-4 | Kill-restart: đóng cứng AMQP giữa lô, relay mới gửi nốt; mọi `event_id` ≥ 1 lần, trùng ≤ lô | `TestTichHop_AC4_DongCungAMQPGiuaLoRoiKhoiDongLai` | I |
| AC-4 / R2 | Đã publish nhưng chưa đánh dấu (`pg_terminate_backend` kết nối relay) ⇒ gửi lại sau lease | `TestTichHop_AC4_R2_DaPublishChuaDanhDauThiGuiLai` | I |
| AC-4 | RabbitMQ `stop_app`/`start_app` khi relay chạy ⇒ không mất `hold.released`, không đỗ row | `TestTichHop_AC4_RabbitMQDungRoiChayLaiKhongMatHoldReleased` | I |
| AC-5 | Trace lưu cùng row (`headers.traceparent`, `trace_id`) | `TestWrite_LuuTraceparentVaTraceIDKhiCoSpan`; `TestTichHop_PersistHoldLuuTraceCuaRequest` | U, I |
| AC-5 | Trace khôi phục khi publish, consumer cùng trace ID | `TestRunOnce_ThanhCongPublishPhongBiVaDanhDau` (ctx publish); `TestTichHop_AC5_TraceIDGiuNguyenTuGhiOutboxToiConsumer` | U, I |
| AC-6 / M5 | Unroutable trong `DropUnroutable` ⇒ `published_at` + `last_error='unroutable'` + metric | `TestRunOnce_UnroutableTrongDanhSachDuocDanhDauBo`; `TestTichHop_AC6_UnroutableChiBoKeyTrongDanhSach` | U, I |
| AC-6 / M5 | Unroutable ngoài danh sách ⇒ lỗi tạm, sửa binding là gửi được | `TestRunOnce_UnroutableNgoaiDanhSachThuLai`; `TestTichHop_AC6_UnroutableChiBoKeyTrongDanhSach` | U, I |
| AC-6 | Worker chỉ bỏ `ticketing.hold.created` | `tk/cmd/worker/main_test.go`: `TestRelayConfig_ChiBoUnroutableChoHoldCreated` | U |
| AC-7 / B1 | Relay không kết nối được: `Run` không trả về tới khi ctx huỷ, gauge `connected` = 0, không claim | `TestRun_DialerLuonLoiKhongTraVeToiKhiHuyCtx`, `TestRunOnce_DialLoiThiKhongClaim`, `TestRun_HuyCtxTraNilVaDongKetNoi` | U |
| AC-7 / r6 | Worker: sweeper vẫn chạy khi relay không kết nối được | `TestRunLoops_RelayKhongKetNoiDuocSweeperVanChay`, `TestRunLoops_MotVongLapLoiThiDungTatCa`, `TestAmqpDialer_BrokerKhongToiDuocTraLoi` | U |
| AC-7 | Thiếu/sai `AMQP_URL` ⇒ fail fast, không lộ mật khẩu | `TestLoadConfig_AMQPURLHopLe`, `TestLoadConfig_ThieuAMQPURLFailFast`, `TestLoadConfig_AMQPURLSaiCuPhapKhongLoMatKhau` | U |
| AC-7 / r8 | Pool worker dùng `QueryExecModeCacheDescribe` | `TestNewPoolConfig_DungCacheDescribeChoPgBouncer` | U |
| AC-7 | Cấu hình relay hợp lệ (bất biến lease, giới hạn) | `TestNewRelay_KiemTraCauHinh`, `TestNewRelay_ThieuDBHoacDialerLaLoi`, `TestNewRelay_MacDinhThoaBatBienLease` | U |
| AC-8 | INSERT kiểu cũ chạy trên 0002, nhận `event_id`, UNIQUE có hiệu lực | `TestTichHop_AC8_InsertKieuCuChayTrenSchema0002VaNhanEventID` | I |
| AC-8 / r4 | Up → Down → Up trong tx tường minh; sau Down cột/index/constraint = 0001; dữ liệu giữ | `TestTichHop_AC8_UpDownUpTraDungSchema0001` | I |
| AC-8 / M7 | `lock_timeout` ⇒ migration bỏ cuộc (`55P03`) khi bảng đang bị khoá | `TestTichHop_AC8_Migration0002BoCuocKhiBangDangBiKhoa` | I |
| AC-9 | `go vet`, `-race`, lint, CI xanh | Gate của bước verify (không phải test riêng); xem `baseline.md` §3 | — |

## Chưa phủ / ghi chú

- Metric `outbox_pending`, `outbox_dead`, `outbox_publish_lag_seconds` chỉ được kiểm gián tiếp (không test tên/giá trị).
- Compose (`ticketing-worker` có `AMQP_URL`, `depends_on`) và goose thật (r9): kiểm ở bước smoke khi có Docker, không có test tự động.
- Thứ tự event không được bảo đảm (plan §6.2) — có chủ đích, không test.
