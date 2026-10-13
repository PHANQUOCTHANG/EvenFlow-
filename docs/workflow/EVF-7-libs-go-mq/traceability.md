# EVF-7 — Truy vết AC → test

File test nằm trong `libs/go/mq/`. "Unit" chạy bằng `go test ./...` (CI `test-go`); "Tích hợp" cần
`go test -tags=integration ./mq/...` + Docker (CI `integration.yml`).

| AC | Nội dung | Test | Loại | Kết quả verify (2026-10-13, `abcda0f`) |
|---|---|---|---|---|
| AC-1 | Publish chỉ `nil` khi broker ack; message đi vào queue, persistent, giữ header | `TestTichHop_PublishDuocAckVaVaoQueue` | Tích hợp | PASS — CI run 38337439044 |
| AC-1 | ctx đã huỷ → lỗi | `TestTichHop_PublishCtxDaHuyTraLoi` | Tích hợp | PASS — CI run 38337439044 |
| AC-1 / m1 | Kênh bị broker đóng (exchange không tồn tại) → lỗi, không treo | `TestTichHop_PublishExchangeKhongTonTaiTraLoi` | Tích hợp | PASS — CI run 38337439044 |
| AC-1 / M2 | Key không định tuyến được → `ErrUnroutable`; publisher dùng tiếp được | `TestTichHop_PublishKeyKhongDinhTuyenDuoc` | Tích hợp | PASS — CI run 38337439044 |
| AC-2 | Lỗi 3 lần → đúng 3 lần xử lý (attempt 1,2,3), DLQ có `x-ef-attempt=3`, body + content-type nguyên vẹn, hàng gốc và hàng trễ rỗng | `TestTichHop_LoiBaLanVaoDLQGiuTrace` | Tích hợp | PASS — CI run 38337439044 |
| AC-2 | Logic quyết định: lỗi + attempt < max → Retry; attempt ≥ max → DeadLetter (m4); không retry exchange → DeadLetter | `TestDecide_BangQuyetDinh`, `TestDecide_BaHanhDongKhacNhau` | Unit | PASS — unit local `-race -count=3` |
| AC-2 | Đọc `x-ef-attempt` (thiếu/int32/int64/int/int16/chuỗi/rác/0/âm) | `TestAttemptHeader_TenHeader`, `TestAttemptOf_DocHeader` | Unit | PASS — unit local `-race -count=3` |
| AC-2 / m3 | Chép header sang bản retry/DLQ: bỏ `x-death`, `x-first-death-*`, `x-last-death-*`, `x-delivery-count`; là bản sao; nil an toàn | `TestCopyHeaders_BoHeaderBrokerGiuPhanCon`, `TestCopyHeaders_NilAnToan` | Unit | PASS — unit local `-race -count=3` |
| AC-2 / M5 | Mặc định `MaxAttempts` 3, `Prefetch` 10; thiếu DLX/Queue hoặc `MaxAttempts` âm bị từ chối; `Consume` từ chối trước khi dùng kênh | `TestConsumerConfig_Validate`, `TestConsume_TuChoiCauHinhThieuDLX` | Unit | PASS — unit local `-race -count=3` |
| AC-3 | Handler `nil` → ack, gọi đúng 1 lần, không retry, mọi queue rỗng; `Consume` trả `context.Canceled` khi huỷ | `TestTichHop_ThanhCongAckKhongRetry`, `TestDecide_BangQuyetDinh` (nil → Ack) | Tích hợp + Unit | PASS — unit local + CI run 38337439044 |
| AC-3 | Lỗi bọc `ErrPermanent` → DLQ ở lần đầu (`x-ef-attempt=1`), gọi 1 lần | `TestTichHop_LoiVinhVienVaoDLQNgay`, `TestDecide_BangQuyetDinh` (vĩnh viễn, bọc 1–2 lớp) | Tích hợp + Unit | PASS — unit local + CI run 38337439044 |
| AC-3 / m5 | Handler panic → lỗi thường (đi Retry), không phải `ErrPermanent` | `TestCallHandler_PanicThanhLoi`, `TestCallHandler_TraNguyenLoiVaNil` | Unit | PASS — unit local `-race -count=3` |
| AC-4 | Span consume cùng trace ID với span publish; kind Producer/Consumer; consume là con của publish; không sửa header người gọi | `TestTrace_PublishRoiConsumeCungTraceID`, `TestTrace_HeadersNilAnToan` | Unit | PASS — unit local `-race -count=3` |
| AC-4 | Trace giữ qua 3 lần retry (mô phỏng chép header + publish trong ctx consume) | `TestTrace_GiuTraceQuaBaLanRetry` | Unit | PASS — unit local `-race -count=3` |
| AC-4 | Trace ID ở cả 3 lần xử lý qua broker thật + hàng trễ bằng trace của người gọi | `TestTichHop_LoiBaLanVaoDLQGiuTrace` | Tích hợp | PASS — CI run 38337439044 |
| AC-5 | Topology AI đúng spec §2 + M3 (hàng trễ riêng, return key, at-least-once, reject-publish, DLQ bind từng key, hàng gốc không DLX/priority) | `TestAITopology_Exchanges`, `TestAITopology_DanhSachQueue`, `TestAITopology_HangGocKhongDLXKhongPriority`, `TestAITopology_HangTreRiengTungCongViec`, `TestAITopology_DLQ`, `TestAITopology_Bindings`, `TestAITopology_BindingKhongMoCoi` | Unit | PASS — unit local `-race -count=3` |
| AC-5 | Ticketing/Notification giữ nguyên; `Merge` | `TestTicketingTopology_GiuNguyen`, `TestNotificationTopology_GiuNguyen`, `TestMerge_GopTheoThuTu`, `TestMerge_TopologyThatDuSoLuong` | Unit | PASS — unit local `-race -count=3` |
| AC-5 | `Declare`: thứ tự exchanges → queues → bindings, đối số + arguments chính xác, lỗi bọc tên đối tượng và dừng, gọi hai lần giống nhau; `*amqp091.Channel` thỏa `Declarer` | `TestDeclare_ThuTuVaDoiSo`, `TestDeclare_LoiBocTenDoiTuongVaDungLai`, `TestDeclare_GoiHaiLanKhongLoi` | Unit | PASS — unit local `-race -count=3` |
| AC-5 | `definitions.json` khớp code (exchanges, queues + arguments, bindings, vhost, destination_type) | `TestDefinitionsJSON_KhopTopologyCode` (fail với file cũ — `baseline.md`; nay xanh) | Unit | PASS — unit local `-race -count=3` |
| AC-5 | `Declare(Merge(AI, Ticketing, Notification))` thành công hai lần trên RabbitMQ 3.13 thật (bắt T5); topology test cũng declare hai lần | `TestTichHop_DeclareTopologyThatHaiLan`, (mọi `TestTichHop_*` qua `newTestTopo`) | Tích hợp | PASS — CI run 38337439044 |
| AC-6 / M2 / M4 | Publish retry không định tuyến được → `Nack(requeue=true)`, `Consume` trả lỗi, message còn ở hàng gốc, DLQ và hàng trễ rỗng | `TestTichHop_PublishRetryLoiKhongMatMessage` | Tích hợp | PASS — CI run 38337439044 |
| AC-7 | `go vet`, `golangci-lint`, `go test -race`, CI | Không có test riêng — kiểm ở bước gates/verify (`go vet -tags=integration` cũng phải sạch, m7) | Gate | PASS — xem `evidence.md` §2–§3 |

## Khoảng trống đã biết

- **Nack thật của broker → `ErrNacked`** (AC-1): khó ép broker nack trong test (cần `x-overflow: reject-publish` trên
  queue đầy hoặc alarm). Chưa có test; reviewer nên đọc code nhánh `!ack`. Có thể thêm test tích hợp: queue
  `x-max-length: 0` + `x-overflow: reject-publish` → publish phải trả `ErrNacked`.
- **Kênh đóng giữa lúc chờ confirm → lỗi bọc `amqp091.ErrClosed`** (m1): chỉ phủ gián tiếp qua test exchange không tồn
  tại (kiểm "có lỗi", không kiểm loại lỗi).
- **Huỷ ctx khi đang chờ confirm**: chỉ test ctx đã huỷ trước khi publish.
- Thời gian: test tích hợp dùng `time.Sleep(3 × TTL hàng trễ)` (600 ms) sau khi message vào DLQ để khẳng định không có
  lần xử lý thứ 4 — sleep cố định duy nhất, ngắn.

## Vòng 2 (code review → plan §8)

| Finding | Test | Loại | Kết quả verify (2026-10-13, `abcda0f`) |
|---|---|---|---|
| B1 | `TestPublisher_KenhDongKhiChoConfirmKhongQuayVong`, `TestPublisher_ReturnsDaDongTruocKhiPublishKhongQuayVong` (giới hạn 2 s) | Unit (kênh giả) | PASS — unit local `-race -count=3` |
| M1 | `TestPublisher_PublishIDKhacNhauMoiLan`, `TestPublisher_ReturnCuKhacIDBiBoQua`, `TestPublisher_ReturnDungIDThanhErrUnroutable`, `TestCopyHeaders_BoHeaderBrokerGiuPhanCon` (bỏ `x-ef-publish-id`) | Unit | PASS — unit local `-race -count=3` |
| M2 | `TestAttemptOf_DocHeader` (int8/uint8/uint16/uint32/uint64, 0, âm, tràn) | Unit | PASS — unit local `-race -count=3` |
| m3 | `TestForwardMessage_GiuThuocTinh` | Unit | PASS — unit local `-race -count=3` |
| m4 | `TestNewDelivery_HeadersLaBanSao` | Unit | PASS — unit local `-race -count=3` |
| m5 | `TestPublisher_KenhDaDongTuTruoc` + hai test B1 (`errors.Is` cả `ErrClosed` và `amqp091.ErrClosed`) | Unit | PASS — unit local `-race -count=3` |
| m6 | `TestConsumerConfig_Validate` (`chi co RetryKey…`, `chi co RetryExchange…`) | Unit | PASS — unit local `-race -count=3` |
| m7 | `TestDefinitionsJSON_KhopTopologyCode` (so kiểu + giá trị, báo trùng, so cả auto_delete/internal/arguments exchange và arguments binding), `TestDefinitionsJSON_ChuanHoaPhanBietKieu`, `TestUniqueIndex_BaoTrung` | Unit | PASS — unit local `-race -count=3` |
| m9 | `TestPublisher_NackThanhErrNacked`, `TestPublisher_HuyCtxKhiDangChoConfirm` (unit); `TestTichHop_BrokerNackThanhErrNacked`, `TestTichHop_PublishExchangeKhongTonTaiTraLoi` (kiểm `ErrClosed` + `amqp091.ErrClosed`), `TestTichHop_PublishHuyCtxKhiDangChoConfirm` (memory alarm watermark 0) | Unit + Tích hợp | PASS — unit local + CI run 38337439044 |
| m10 | `depth` trong `integration_test.go` kiểm lỗi `ch.Close()` | Lint | PASS — golangci-lint `--build-tags integration` 0 issues |

Các khoảng trống "nack thật", "kênh đóng giữa lúc chờ", "huỷ ctx khi đang chờ" ở trên nay đã có test.

## Vòng 3 (challenge → plan §9)

| Finding | Test | Mutant bị giết | Kết quả verify (2026-10-13, `abcda0f`) |
|---|---|---|---|
| F1 | `TestPublisher_NhieuReturnCuKhongLamMatReturnMoi`: 20 publish bị huỷ ctx khi đang chờ, rồi một publish không định tuyến được; "goroutine đọc" mô phỏng `notifyAll` (chờ tối đa 5 s rồi bỏ) đẩy 20 return cũ + return của lần này trước ack → phải `ErrUnroutable` trong < 2 s, không return nào bị bỏ | F1-cũ (không đọc returns khi đang chờ) | PASS — unit local `-race -count=3` (mutant bị giết) |
| F2 | `TestPublisher_GuiDuThuocTinhAMQP`: mọi property của `Message` đi lên kênh (`ContentEncoding`, `CorrelationId`, `ReplyTo`, `Type`, `AppId`, `Priority`, `Timestamp`, persistent…) | P9–P13 | PASS — unit local `-race -count=3` (mutant bị giết) |
| F3 | `TestPublisher_PublishDongThoiChiLanUnroutableLoi`: hai kịch bản (A hoặc B không định tuyến được), mỗi kịch bản 5 lần; chỉ lần không định tuyến được nhận `ErrUnroutable` | P14 (bỏ mutex) | PASS — unit local `-race -count=3` (mutant bị giết) |
| F4 | `TestConsumeWiring_DeliveriesDongTraLoiQosVaCancel` (deliveries đóng → lỗi, `Qos(prefetch)` 7/mặc định 10, `Cancel(tag)` đúng tag), `TestConsumeWiring_HuyCtxCancelConsumer`, `TestConsumeWiring_KhongRetryExchangeLoiDauVaoDLQ`, `TestConsumeWiring_CoRetryExchangeLoiDauDiRetry` — qua seam `consumeChannel` + `consume(...)` | C20, C25, C26, C27 | PASS — unit local `-race -count=3` (mutant bị giết) |
| F5 | `TestDeclare_LoiChiOQueueDeclareKhongBiNuot` (fake chỉ lỗi ở `QueueDeclare`, có `failOp`) | T3 | PASS — unit local `-race -count=3` (mutant bị giết) |
| F6 | `parseDefinitions` (khoá JSON trùng qua decoder token, `DisallowUnknownFields` ở gốc và phần tử, `policies`/`parameters`/`global_parameters` phải rỗng) dùng trong `TestDefinitionsJSON_KhopTopologyCode`; `TestParseDefinitions_TuChoiFileSai`, `TestFindDuplicateKeys_LongNhau` | D5, D6, D7 | PASS — unit local `-race -count=3` (mutant bị giết) |
| P6 | `TestPublisher_LoiGuiKhiKenhVuaDongThanhErrClosed` (gửi lỗi `amqp091.ErrClosed` rồi `IsClosed` = true → bọc `ErrClosed`) | P6 | PASS — unit local `-race -count=3` (mutant bị giết) |

Chạy lại 16 mutant còn sống ở `challenge.md` (P6, P9–P14, C20, C25–C27, T3, D5–D7, và F1-cũ) trên **bản sao trong
scratchpad** (implementer đang sửa source song song), chỉ unit test `go test ./mq/`: **16/16 bị giết**. File bị mutate
được ghi lại nội dung gốc sau mỗi mutant (kiểm byte), và so khớp với worktree sau khi chạy. Ghi chú P14: với cách chờ
mới (đọc returns đồng thời), kênh Go giao giá trị cho người nhận chờ lâu nhất, nên kịch bản "A unroutable" một mình
không giết được P14; kịch bản "B unroutable" (A đang chờ nuốt return của B) giết được một cách tất định.
