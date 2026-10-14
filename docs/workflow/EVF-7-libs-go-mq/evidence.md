# EVF-7 — Bằng chứng xác minh (Verify)

| Mục | Giá trị |
|---|---|
| Thời điểm | 2026-10-13 09:38 (+07:00) |
| Người xác minh | Verifier độc lập (agent, context sạch, chỉ đọc; không viết dòng code nào của slice) |
| Branch | `feature/EVF-7-libs-go-mq` (worktree `EvenFlow-mq`), PR #52 (draft, base `develop`) |
| HEAD | `abcda0f2bacb8cfa03cdfbbc03325c2204d4925d` (= `origin/feature/EVF-7-libs-go-mq`) |
| Môi trường | Go 1.27.0 windows/amd64, `GOTOOLCHAIN=local`; Docker daemon không chạy → test tích hợp lấy từ CI |
| Kết luận | **PASS** |

## 1. Lệnh đã chạy (trong `libs/go`, trừ khi ghi khác)

| # | Lệnh | Kết quả |
|---|---|---|
| 1 | `gofmt -l mq` | Không có file nào |
| 2 | `go vet ./...` | Sạch |
| 3 | `go vet -tags=integration ./...` | Sạch |
| 4 | `go test -race -count=3 ./...` | `httpx` ok, `mq` ok (10.0 s), `redisx` không có test, **`otelx` FAIL** — xem §4 (có từ trước, không thuộc slice) |
| 5 | `go test -race -count=1 ./...` | Tất cả ok (`httpx`, `mq`, `otelx`) |
| 6 | `go test -race -count=3 ./mq/ ./httpx/` | ok |
| 7 | `go test -count=1 -v ./mq/` | 52 test cấp cao nhất PASS (111 dòng PASS kể cả subtest), 0 FAIL, 0 SKIP |
| 8 | `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./mq/...` | `0 issues.` |
| 9 | Như trên + `--build-tags integration` | `0 issues.` |
| 10 | `grep -nE '^(go \|toolchain)' go.mod` | Chỉ `go 1.25.0`, **không** có dòng `toolchain` |
| 11 | Gốc repo: `for d in services/*/; do (cd $d && go build ./... && go vet ./...); done` (workspace mode, `go.work` có `libs/go` + 8 service) | `antibot`, `event`, `gateway`, `identity`, `notification`, `payment`, `ticketing`, `waitingroom`: **8/8 OK** |
| 12 | `git diff --name-only origin/develop...HEAD` lọc theo danh sách cho phép | Không có file ngoài phạm vi (24 file: `libs/go/{go.mod,go.sum,mq/**}`, `deploy/rabbitmq/definitions.json`, `.github/workflows/integration.yml`, `docs/02-kien-truc.md`, `docs/03-cau-truc-src.md`, `docs/09-ke-hoach-hoan-thien-backend.md`, `docs/workflow/EVF-7-libs-go-mq/**`). Không đụng `services/**`, `go.work`, `go.work.sum` |
| 13 | Đối chiếu mọi tên test trong `traceability.md` với log PASS (unit local + log CI) | 62/62 tên test có thật và PASS (2 kết quả "thiếu" của regex là `TestTichHop_*` và `newTestTopo`, không phải tên test) |

## 2. CI (PR #52, HEAD `abcda0f`)

`gh pr checks 52 --repo PHANQUOCTHANG/EvenFlow- --watch` kết thúc, mọi check **pass** hoặc **skipping** (web/python
không đổi), không có check fail.

| Workflow | Run | Job liên quan | Kết quả |
|---|---|---|---|
| CI | [38337439055](https://github.com/PHANQUOCTHANG/EvenFlow-/actions/runs/38337439055) | `test-go` (58 s), `lint-go` (37 s), `lint-ci`, `build` | pass |
| Integration | [38337439044](https://github.com/PHANQUOCTHANG/EvenFlow-/actions/runs/38337439044) | `integration` (43 s) | pass |
| (security) | [38337439006](https://github.com/PHANQUOCTHANG/EvenFlow-/actions/runs/38337439006) | trivy × 4, `trivy-gate` | pass |

Cả ba run đều có `headSha = abcda0f2…`.

Log run Integration 38337439044 (`gh run view --log`), bước `go test -tags=integration (libs/go/mq)` trên RabbitMQ
3.13 thật (testcontainers):

- `--- PASS: TestTichHop` = **11**, `--- FAIL` = **0**, `--- SKIP: TestTichHop` = **0**; `ok github.com/eventflow/eventflow/libs/go/mq 21.365s`.
- Danh sách: `PublishDuocAckVaVaoQueue`, `PublishKeyKhongDinhTuyenDuoc`, `PublishExchangeKhongTonTaiTraLoi` (0.00 s, không treo),
  `BrokerNackThanhErrNacked`, `PublishHuyCtxKhiDangChoConfirm`, `PublishCtxDaHuyTraLoi`, `LoiBaLanVaoDLQGiuTrace` (1.05 s),
  `LoiVinhVienVaoDLQNgay`, `ThanhCongAckKhongRetry`, `PublishRetryLoiKhongMatMessage`, `DeclareTopologyThatHaiLan`.
- Bước `go vet -tags=integration (libs/go/mq)` chạy và pass.

Lịch sử run Integration trên branch: [38333288806](https://github.com/PHANQUOCTHANG/EvenFlow-/actions/runs/38333288806)
(`c69c9e2`) **fail** — `TestTichHop_PublishDuocAckVaVaoQueue: thieu traceparent` vì test đầu tiên chưa cài propagator
toàn cục; sửa ở `e8ee62b`. [38333640434](https://github.com/PHANQUOCTHANG/EvenFlow-/actions/runs/38333640434) (`e8ee62b`)
pass 11/11. Run mới nhất 38337439044 (`abcda0f`) pass 11/11.

## 3. Đối chiếu AC

| AC | Bằng chứng | Kết quả |
|---|---|---|
| AC-1 Publisher confirm | Tích hợp: `TestTichHop_PublishDuocAckVaVaoQueue`, `_BrokerNackThanhErrNacked`, `_PublishExchangeKhongTonTaiTraLoi` (`ErrClosed` + `amqp091.ErrClosed`), `_PublishHuyCtxKhiDangChoConfirm`, `_PublishCtxDaHuyTraLoi`, `_PublishKeyKhongDinhTuyenDuoc`; unit `TestPublisher_*` (nack, kênh đóng, ctx, return) | PASS |
| AC-2 Lỗi 3 lần vào DLQ | **Tích hợp broker thật: `TestTichHop_LoiBaLanVaoDLQGiuTrace`** — đã đọc test: handler gọi đúng attempts `[1 2 3]`, DLQ có `x-ef-attempt = 3`, body + content-type nguyên vẹn, hàng gốc và hàng trễ rỗng, chờ thêm 3×TTL để chắc không có lần 4. Unit: `TestDecide_*`, `TestAttemptOf_DocHeader`, `TestCopyHeaders_*`, `TestConsumerConfig_Validate` | PASS |
| AC-3 Ack / ErrPermanent | `TestTichHop_ThanhCongAckKhongRetry`, `TestTichHop_LoiVinhVienVaoDLQNgay`; unit `TestDecide_BangQuyetDinh`, `TestCallHandler_*` | PASS |
| AC-4 Trace xuyên suốt | **Tích hợp broker thật: `TestTichHop_LoiBaLanVaoDLQGiuTrace`** — trace ID trong ctx của handler ở cả 3 lần (qua hàng trễ) bằng trace ID của span người gọi. Unit: `TestTrace_PublishRoiConsumeCungTraceID`, `TestTrace_GiuTraceQuaBaLanRetry`, `TestTrace_HeadersNilAnToan` | PASS |
| AC-5 Topology bằng code khớp file | Unit `TestAITopology_*`, `TestDeclare_*`, `TestDefinitionsJSON_KhopTopologyCode` (+ `parseDefinitions`: khoá trùng, trường lạ, `policies` rỗng); tích hợp `TestTichHop_DeclareTopologyThatHaiLan` (`Merge(AI, Ticketing, Notification)` hai lần trên 3.13) | PASS |
| AC-6 Không mất message | `TestTichHop_PublishRetryLoiKhongMatMessage` (nack requeue, `Consume` trả lỗi, message còn ở hàng gốc); unit `TestConsumeWiring_*` | PASS |
| AC-7 Chất lượng | §1 #2–#11, §2 (`test-go`, `lint-go` pass; `integration.yml` chạy test `mq`). Không chạy `go work sync` (plan §7 M1 đã thay điều kiện này) | PASS |

## 4. Finding của các vòng review

| Nguồn | Finding | Trạng thái |
|---|---|---|
| plan-review | M1 (`go work sync`) | Sửa: không chạy; diff không có `go.work*`/`services/**`; 8 service build + vet OK (§1 #11–12) |
| plan-review | M2 (mandatory + return) | Sửa trong `3c83611` (mandatory + `NotifyReturn`, tương quan bằng `x-ef-publish-id`); test `TestTichHop_PublishKeyKhongDinhTuyenDuoc`, `TestPublisher_Return*` |
| plan-review | M3 (at-least-once + reject-publish) | Sửa trong code, `definitions.json` (`38291f4`), test `TestAITopology_HangTreRiengTungCongViec`, `TestDefinitionsJSON_KhopTopologyCode` |
| plan-review | M4, M5 | Sửa: forward lỗi → nack requeue + `Consume` trả lỗi; thiếu DLX bị từ chối (`TestTichHop_PublishRetryLoiKhongMatMessage`, `TestConsumerConfig_Validate`) |
| plan-review | M6 | Sửa `docs/02` L5 + `_comment` (`c69c9e2`, `38291f4`) |
| plan-review | m1–m9 | Đã xử lý theo plan §7 (xem `traceability.md`); m2 lệch có chủ đích, ghi trong doc comment `Publisher` và plan §8 m1 |
| code-review | B1 (quay vòng khi kênh đóng) | Sửa (trước khi commit, có trong `3c83611`): `r, ok := <-returns`, `returns = nil` khi đóng; `TestPublisher_KenhDongKhiChoConfirmKhongQuayVong`, `_ReturnsDaDongTruocKhiPublishKhongQuayVong`; CI `PublishExchangeKhongTonTai` 0.00 s |
| code-review | M1 (gán nhầm return) | Sửa (`3c83611`): `x-ef-publish-id`; `TestPublisher_ReturnCuKhacIDBiBoQua` |
| code-review | M2 (`attemptOf` kiểu không dấu) | Sửa: `attemptOf` nhận `uint*`, chặn tràn; `TestAttemptOf_DocHeader`; `docs/02` ghi int32 có dấu |
| code-review | m1–m10 | Sửa (xem bảng "Vòng 2" `traceability.md`); m8 docs/03, docs/09 đã sửa; m10 lint integration 0 issues |
| challenge | F1 (bộ đệm return đầy) | Sửa trong `311b7ab` (đọc returns trong lúc chờ confirm); `TestPublisher_NhieuReturnCuKhongLamMatReturnMoi` |
| challenge | F2–F6, P6 | Thêm test trong `b35bd47` (bảng "Vòng 3" `traceability.md`); 16/16 mutant sống trước đó bị giết (theo ghi chép của implementer — verifier chỉ xác nhận các test tồn tại và PASS) |
| challenge | F7, F9 | Ghi trong `doc.go` (không bật `Config.Recovery`; `x-ef-publish-id` thấy được; float → 1) |
| challenge | F8 (oversell smoke 40 SKIP) | Có từ trước, ngoài phạm vi — xác nhận lại ở run 38337439044: `0 lan PASS, 0 lan FAIL, 40 lan SKIP` |

## 5. Vấn đề có từ trước / ngoài phạm vi (không chặn)

1. **`otelx` fail với `-count>1`**: `TestHTTP_SpanVaMetricTheoMauRoute`, `TestHTTP_TrongChuoiMiddlewareThat_VanThayMauRoute`,
   `TestHTTP_KhongKhopRoute_NhanUnmatched` ("histogram ... = 2/3 mau, muon 1") — state metric toàn cục cộng dồn giữa
   các lần lặp. Tái hiện trên bản `git archive origin/develop` (`GOWORK=off go test -race -count=3 ./otelx/` → FAIL);
   branch không sửa `libs/go/otelx` (diff rỗng). `-count=1` (chế độ CI) pass. Nên mở ticket riêng.
2. **Oversell smoke của ticketing** trong `integration.yml`: 40 SKIP, không kiểm gì (F8, EVF-39).
3. Cảnh báo Node 20 deprecated trong action GitHub (thông tin).

## 6. Quan sát của verifier (không chặn, ghi nợ)

- Truyền trace phụ thuộc propagator toàn cục (`otelx.Setup` gọi `otel.SetTextMapPropagator`, `otelx/otelx.go:50`).
  Service dùng `mq` mà không gọi `otelx.Setup` sẽ không có `traceparent` — chính lỗi đã làm fail run 38333288806 ở
  phía test. Nên ghi câu này vào `doc.go` khi nối service (EVF-36/80).
- Test tích hợp AC-2/AC-4 dùng topology thử (`newTestTopo`, TTL 200 ms) có cùng cấu trúc với `AITopology` (hàng trễ
  riêng, dead-letter về direct với key của hàng, at-least-once, reject-publish), còn `AITopology` thật được declare trên
  broker ở `TestTichHop_DeclareTopologyThatHaiLan`. Chưa có test chạy vòng retry trọn vẹn trên chính tên `ai.*` (TTL 5 s
  – 2 m) — chấp nhận được.
- `Message.Headers` không được chuẩn hoá map lồng (`amqp091.Table.Validate` từ chối `map[string]any` lồng); plan §7
  m3 nói "chỉ kiểu phẳng" nhưng doc comment của `Message` chưa ghi. Nhỏ.
- Mục "Khoảng trống đã biết" ở đầu `traceability.md` đã lỗi thời (các khoảng trống đã được phủ ở vòng 2 — chính file đã
  ghi chú điều này).

## 7. Kết luận

**PASS.** Mọi AC-1..AC-7 có bằng chứng PASS; AC-2 và AC-4 có test tích hợp với RabbitMQ thật
(`TestTichHop_LoiBaLanVaoDLQGiuTrace`) PASS trong CI run 38337439044. Gates cục bộ sạch (gofmt, vet ×2, golangci-lint ×2
0 issues, `mq` race ×3 PASS, `go 1.25.0` không `toolchain`, 8/8 service build + vet). Diff đúng phạm vi. Mọi finding
của plan-review, code-review, challenge đã sửa có test, hoặc được ghi là thông tin/ngoài phạm vi. Lỗi `otelx -count=3`
có từ trước trên `develop`, không thuộc slice.
