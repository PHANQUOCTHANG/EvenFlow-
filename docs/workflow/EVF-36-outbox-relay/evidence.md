# EVF-36 — Bằng chứng xác minh (verify)

| Mục | Giá trị |
|---|---|
| Thời điểm | 2026-10-13 (sau CI run lúc 05:59Z) |
| Người xác minh | Verifier độc lập, chỉ đọc (không sửa source/test/workflow/migration, không commit/push) |
| Worktree / branch | `D:/Documents/Nam4-1/Project1/EvenFlow-outbox`, `feature/EVF-36-outbox-relay` |
| HEAD | `87969dbb169d1d43a13b3f1dad26774aa9d52fef` (= `origin/feature/EVF-36-outbox-relay`, working tree sạch trước khi viết file này) |
| Nền xếp chồng | `origin/feature/EVF-7-libs-go-mq` = `407f5990aa86c18dfe2d82b75ade5be7926f33b1` (merge-base, PR #52) |
| PR | [#55](https://github.com/PHANQUOCTHANG/EvenFlow-/pull/55), draft, base `develop`, `mergeStateStatus: CLEAN` |
| Môi trường local | Go 1.27.0 windows/amd64, `GOTOOLCHAIN=local`, `GOPROXY=file://…`, `GOSUMDB=off`; Docker tắt ⇒ test tích hợp lấy từ CI |

## Kết luận: **PASS** (về kỹ thuật) — **merge vẫn bị chặn** cho tới khi người dùng duyệt migration và có `approval.md`

Mọi gate local và CI xanh, đủ 29/29 `TestTichHop` trên CI, mọi AC có bằng chứng đạt, mọi finding của plan review (vòng 1–2),
code review và challenge đều đã sửa (có commit + test) hoặc được ghi là nợ. `approval.md` **chưa tồn tại** (đúng, người dùng chưa
duyệt) ⇒ **không được merge** (spec tier High-risk, plan §6.1, §7 r10).

## 1. Phạm vi diff

`git diff --name-only origin/feature/EVF-7-libs-go-mq...HEAD` → 34 file, **tất cả** nằm trong danh sách cho phép (plan §3 + §6.6 + §7 r5):

- `migrations/0002_outbox_relay.sql`
- `libs/go/outbox/{doc,metrics,relay,write}.go` + `{fakes,write,relay,integration}_test.go`; `libs/go/testx/pgtest/pgtest.go`; `libs/go/go.mod`, `go.sum`
- `services/ticketing/**` (cmd/worker main + test, adapter/postgres repository + integration test, port/ports.go, worker/hold_sweeper + test, go.mod, go.sum)
- `deploy/compose/docker-compose.yml` — diff **chỉ** trong service `ticketing-worker` (comment, `AMQP_URL`, `depends_on: rabbitmq {service_healthy}`); service `rabbitmq` đã có healthcheck
- `.github/workflows/integration.yml`
- `docs/02-kien-truc.md`, `docs/03-cau-truc-src.md`, `docs/06-Thiet_ke_co_ban.md`, `docs/09-ke-hoach-hoan-thien-backend.md`
- `docs/workflow/EVF-36-outbox-relay/*`

Không đổi: `go.work`, `go.work.sum`. `libs/go/go.mod` và `services/ticketing/go.mod` vẫn `go 1.25.0`, không có dòng `toolchain`;
`go mod tidy -diff` sạch ở cả hai module.

## 2. Gate local (lệnh và kết quả)

| # | Lệnh | Kết quả |
|---|---|---|
| G1 | `gofmt -l` trên 16 file `.go` đổi (sau khi bỏ `\r`) | sạch |
| G1b | `gofmt -l` trên **blob đã commit** (`git show HEAD:…`) của toàn bộ 55 file `.go` trong `libs/go` + `services/ticketing` | sạch |
| G2 | `go vet ./...` và `go vet -tags=integration ./...` — `libs/go`, `services/ticketing` | sạch |
| G3 | `GOWORK=off go vet ./...` và `GOWORK=off go vet -tags=integration ./...` — `services/ticketing` | sạch |
| G4 | `libs/go`: `go test -race -count=3` (mọi package trừ otelx) + `go test -race -count=1 ./otelx/...` (otelx flaky với count>1 là có từ trước) | `httpx` ok, `mq` ok, `outbox` ok (15.8s), `otelx` ok |
| G5 | `services/ticketing`: `go test -race -count=3 ./...` | `cmd/worker`, `adapter/metrics`, `app`, `domain`, `worker` ok |
| G6 | Số unit test (`-v`, count=1): `libs/go/outbox` | 41 PASS top-level, 0 FAIL/SKIP; `cmd/worker` + `internal/worker`: 17 PASS |
| G7 | golangci-lint v2.12.2 `run --build-tags integration` — `libs/go ./outbox/... ./testx/...`, `services/ticketing ./...` | 1 + 16 hit, **toàn bộ là `gofmt`** trên file CRLF trong working tree. Phân loại: **artifact checkout** — `core.autocrlf=true`, `git ls-files --eol` cho thấy `i/lf w/crlf`; blob đã commit gofmt sạch (G1b). 16 hit của ticketing là file **không có** trong diff của nhánh; `libs/go/outbox/metrics.go` có trong diff nhưng index là LF và sạch. Không có hit của linter khác. CI `lint-go` (checkout LF) xanh |
| G8 | `go build ./... && go vet ./...` cho `libs/go` + 8 service (antibot, event, gateway, identity, notification, payment, ticketing, waitingroom), workspace mode | cả 9 module sạch |
| G9 | `bash .github/scripts/tests/test-workflows.sh` | `128 PASS, 0 FAIL` |
| G10 | Mọi tên test trong `traceability.md` có `func` tương ứng trong code | 71/71 tìm thấy |

## 3. CI (HEAD `87969db`)

| Workflow | Run | Kết quả |
|---|---|---|
| Integration | [38354241696](https://github.com/PHANQUOCTHANG/EvenFlow-/actions/runs/38354241696) | success |
| CI (`test-go`, `lint-go`, `lint-ci`, `build`, `validate-deploy`) | [38354241757](https://github.com/PHANQUOCTHANG/EvenFlow-/actions/runs/38354241757) | success (web/python job skip do không đổi) |
| Trivy scan (ai-worker, ticketing, waitingroom, web, `trivy-gate`) | [38354241759](https://github.com/PHANQUOCTHANG/EvenFlow-/actions/runs/38354241759) | success |

`gh pr checks 55`: mọi check `pass` (các job web/python `skipping`), Vercel pass.

**Đếm từ log Integration 38354241696:** `--- PASS: TestTichHop*` = **29**, `--- FAIL` = **0**, `--- SKIP: TestTichHop*` = **0**.
(42 dòng `--- SKIP` khác đều là `TestNoOversell_*` của bước oversell smoke, `TODO(EVF-39)`, có từ trước và ngoài phạm vi.)

Phân bổ 29 test:

- `libs/go/outbox` (12): `AC3_PublishVaDanhDau` (25 event, 0 trùng), **`AC4_DongCungAMQPGiuaLoRoiKhoiDongLai`** (kill-restart, 30/0),
  **`AC4_ChiCatTCPRelayTuDialLaiGuiHet`** (chỉ cắt TCP, 30/0), **`AC4_R2_DaPublishChuaDanhDauThiGuiLai`** (published-not-marked, 30 event,
  1 trùng ≤ lô), **`AC4_RabbitMQDungRoiChayLaiKhongMatHoldReleased`** (broker stop/start, 25/0), `AC5_TraceIDGiuNguyenTuGhiOutboxToiConsumer`,
  `AC6_UnroutableChiBoKeyTrongDanhSach`, **`HaiRelaySongSongKhongGuiTrungTrongMotLease`** (200 event, 0 trùng),
  **`AC8_UpDownUpTraDungSchema0001`**, `AC8_Migration0002BoCuocKhiBangDangBiKhoa` (5.41s ≈ `lock_timeout` 5s),
  `AC8_InsertKieuCuChayTrenSchema0002VaNhanEventID`, `AC8_RowCuTruoc0002DuocBocPhongBi`.
- `services/ticketing/internal/adapter/postgres` (6): `PersistHoldGhiOutboxHoldCreatedCungTransaction`, `PersistHoldLuuTraceCuaRequest`,
  `PersistHoldRollbackThiKhongCoRowOutbox`, `ReleaseHoldGhiOutboxHoldReleasedMotLan`, `ReleaseHoldLoiThiKhongCoRowOutbox`,
  `ReleaseHoldDaConsumeKhongGhiOutbox`.
- `libs/go/mq` (11, của EVF-7, không hồi quy): `BrokerNackThanhErrNacked`, `DeclareTopologyThatHaiLan`, `LoiBaLanVaoDLQGiuTrace`,
  `LoiVinhVienVaoDLQNgay`, `PublishCtxDaHuyTraLoi`, `PublishDuocAckVaVaoQueue`, `PublishExchangeKhongTonTaiTraLoi`,
  `PublishHuyCtxKhiDangChoConfirm`, `PublishKeyKhongDinhTuyenDuoc`, `PublishRetryLoiKhongMatMessage`, `ThanhCongAckKhongRetry`.

Kiểm lại tổng: outbox 12 + ticketing 6 + mq 11 = **29**. Bước ticketing và outbox đều bọc `run-and-report-tests.sh TestTichHop` với `-v`
(skip toàn bộ sẽ làm bước đỏ) và có `go vet -tags=integration` riêng.

Lịch sử: Integration 38343980004 (`6f29095`) FAIL ở AC8 scan catalog → sửa ở `553c828`; CI 38345872436 (`5cea3cd`) FAIL ở `lint-ci`
(thứ tự bước oversell smoke mà `test-workflows.sh` đọc) → sửa ở `cb4f598`. Cả hai đã được thay bằng run xanh.

## 4. Đối chiếu AC

| AC | Bằng chứng đạt |
|---|---|
| AC-1 | CI: 3 test `PersistHold*` + 3 test `ReleaseHold*` (cùng tx, rollback ⇒ 0 row, gọi lại không ghi thêm); local: `hold_sweeper_test.go` (sweeper không còn publisher, `port.Publisher` đã xoá) |
| AC-2 | Local: `TestWrite_*`, `TestEnvelope_TenTruongJSON`, `TestRunOnce_ThanhCongPublishPhongBiVaDanhDau`; CI: `AC3_PublishVaDanhDau`, `AC8_RowCuTruoc0002DuocBocPhongBi`, test ticketing (`sale_event_id`, không PII) |
| AC-3 | Local: `TestRunOnce_*` (retry, park, panic, lỗi DB, hạn lô, guard `published_at IS NULL`), `TestBackoff_*`, `TestSanitizeErr_*`; CI: `AC3_*`, `HaiRelaySongSong…` |
| AC-4 | CI: 4 test `AC4_*` (kill-restart, chỉ cắt TCP, published-not-marked, broker stop/start) |
| AC-5 | Local: `TestWrite_LuuTraceparentVaTraceIDKhiCoSpan`; CI: `AC5_TraceIDGiuNguyenTuGhiOutboxToiConsumer`, `PersistHoldLuuTraceCuaRequest` |
| AC-6 | Local: `TestRunOnce_Unroutable*`, `TestRelayConfig_ChiBoUnroutableChoHoldCreated`; CI: `AC6_UnroutableChiBoKeyTrongDanhSach` |
| AC-7 | Local: `TestLoadConfig_*` (fail fast, không lộ mật khẩu), `TestRunLoops_*`, `TestRun_*`, `TestNewPoolConfig_DungCacheDescribeChoPgBouncer`, `TestAmqpDialer_*`; metric `outbox_pending`, `outbox_dead`, `outbox_publish_errors_total`, `outbox_unroutable_total`, `outbox_publish_lag_seconds`, `outbox_relay_connected`, `outbox_published_total` có trong `metrics.go`; compose đúng (§1) |
| AC-8 | CI: 4 test `AC8_*` (Up→Down→Up so schema 0001, `55P03`, INSERT kiểu cũ, row cũ) |
| AC-9 | §2 G2–G9 + §3 |

Kết quả từng dòng đã ghi vào cột "Kết quả" của `traceability.md`.

## 5. Đối chiếu finding

**Plan review vòng 1** (B1, B2, M1–M9, m1–m13): tất cả "Đã sửa"/"Ghi nhận" theo bảng vòng 2 trong `plan-review.md`; m9, m10, m11 còn phần được
xử lý ở vòng 2 (r8, r9, R2) — xem dưới.

**Plan review vòng 2:**

| Finding | Trạng thái | Bằng chứng |
|---|---|---|
| R1 | Đã sửa (kèm lệch có chủ đích m3) | `relay.go` phân loại lỗi; `TestRunOnce_ErrClosed…`, `…HuyCtx…`, `…HetBatchDeadline…`, `…LoiKhongPhanLoaiTangAttempts`; gauge `outbox_dead`; runbook gỡ đỗ ở `docs/02` |
| R2 | Đã sửa | CI `AC4_R2_DaPublishChuaDanhDauThiGuiLai` |
| r1 | Đã sửa | `relay.go:107` (`BatchDeadline+PublishTimeout+2*markTimeout < Lease`), mọi UPDATE có `AND published_at IS NULL` (`relay.go:230-234`) |
| r2 | Đã sửa | `TestBackoff_*` |
| r3 | Đã sửa | `repository.go:232` `RETURNING h.id::text` |
| r4 | Đã sửa | CI `AC8_UpDownUpTraDungSchema0001` (sửa scan ở `553c828`) |
| r5 | Đã sửa | `libs/go/testx/pgtest/pgtest.go` (tag `integration`) |
| r6 | Đã sửa | `TestRun_DialerLuonLoiKhongTraVeToiKhiHuyCtx`, `TestRunLoops_RelayKhongKetNoiDuocSweeperVanChay` |
| r7 | Đã sửa | `TestRunOnce_PanicTrongMotRowDuocRecoverThanhLoi` |
| r8 | Đã sửa | `cmd/worker/main.go:169` + `TestNewPoolConfig_…` |
| r9 | **Nợ** | CI không chạy binary goose thật; kiểm ở smoke compose (`goose-docker:3.22.0`) khi có Docker |
| r10 | **Đang chờ** | `approval.md` chưa có ⇒ chặn merge |
| r11 | Đã sửa | `docs/03` (consumer đọc `event_id` từ phong bì) |

**Code review** (`7c54802…553c828`, sửa ở `82dc636`, `a77f560`, `6906b27`, `5cea3cd`):

| Finding | Trạng thái | Bằng chứng |
|---|---|---|
| m1 | Đã sửa (`82dc636`) | bất biến ở `relay.go:107`; `TestRunOnce_PublishBiCatTaiHanChotLo`, `TestRunOnce_TraLeaseKhongGoDoRowDaDo` |
| m2 | **Nợ** (broker blocked) | ghi ở `doc.go:30-32` và `docs/02:196`; đề xuất ticket theo dõi `NotifyBlocked` |
| m3 | Đã sửa (docs `5cea3cd` + test) | `doc.go:24`, `docs/02:195`; `TestRunOnce_LoiKhongPhanLoaiTangAttempts` |
| m4 | Đã sửa (`82dc636`) | `connDead` `relay.go:325-333`; `TestRunOnce_KetNoiDaDongTruocClaimThiKhongClaimVaDialLai` |
| m5 | Đã sửa (`a77f560`) | CI `AC4_ChiCatTCPRelayTuDialLaiGuiHet` |
| m6 | Đã sửa (`6906b27`) | `integration.yml` bọc ticketing bằng `run-and-report-tests.sh TestTichHop` `-v` |
| m7 | Không đổi (nit, tuỳ chọn) | CI hai relay 200 event 0 trùng |
| m8 | Đã sửa | metric sau mark (`TestRunOnce_DanhDauLoiThiKhongTangMetricPublished`); `released_at` từ `RETURNING h.released_at` (`repository.go:232`) |
| m9 | **Nợ** | (a) goose thật không chạy ở CI; (b) `approval.md` — chặn merge; (c) pool `cmd/server` vẫn exec mode mặc định qua `pgbouncer:latest` (không pin) — kiểm ở smoke compose |

**Challenge** (`cb4f598`, sửa ở `82ef07e`, `5f59313`):

| Finding | Trạng thái | Bằng chứng |
|---|---|---|
| F1 | Đã sửa | `markRetrySQL`/`markParkSQL` có `AND next_attempt_at <> 'infinity'` (`relay.go:232-233`); `TestRunOnce_ThuLaiVaDoKhongDungRowDaBiRelayKhacDo` |
| F2 | Đã sửa | `write.go:73-75`; `TestWrite_KiemTraDauVao` (case F2) |
| F3 | Đã sửa | claim trả `now()` của DB, `publishLag` kẹp 0 (`relay.go:551-554`); `TestRunOnce_LagTheoDongHoDB`, `TestRunOnce_LagAmKepVe0` |
| F4 | Đã sửa | M22/M23/M29/M30/M31/M36/M42/M43/M47/M48, W1/W6/W7 có test tương ứng (traceability "Bổ sung sau challenge"); M11/M45 tương đương; M20, M28, M35, M37 vẫn sống (đã lập luận: vô hại/không bảo đảm thứ tự) |

**Nợ tổng hợp (không chặn kỹ thuật, cần theo dõi):** m2 broker blocked có thể đỗ `hold.released` sau ~1h; `edoburu/pgbouncer:latest` không
pin + pool `cmd/server` exec mode mặc định; binary goose không chạy ở CI (r9); compose (`AMQP_URL`, `depends_on`) chưa smoke vì không có
Docker; metric `outbox_pending`/`outbox_dead`/lag chỉ kiểm gián tiếp; thứ tự event không bảo đảm (có chủ đích); oversell smoke vẫn skip (EVF-39).

## 6. Migration `0002_outbox_relay.sql` — checklist để người dùng ký duyệt

**Đối chiếu với mô tả PR #55** (`gh pr view 55 --json body`): phần **Up** trong PR khớp từng câu lệnh với file (file có thêm 2 dòng chú thích
`--`, không phải SQL). Phần **Down không có** trong mô tả PR — người duyệt cần đọc Down ngay dưới đây (hoặc bổ sung vào PR). Mô tả PR cũng
còn cũ ở hai chỗ: bảng "Verification so far" ghi integration "first run in this PR's CI" (nay đã 29/29 xanh), và "Stacked on #52" trong khi
base của PR là `develop` (diff của PR so với `develop` vì thế gồm cả EVF-7 cho tới khi #52 merge).

**Up** (một transaction của goose — file không có `NO TRANSACTION`):
```sql
SET LOCAL lock_timeout = '5s';
ALTER TABLE outbox_events
  ADD COLUMN event_id        UUID        NOT NULL DEFAULT gen_random_uuid(),
  ADD COLUMN headers         JSONB       NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN attempts        INT         NOT NULL DEFAULT 0,
  ADD COLUMN last_error      TEXT,
  ADD COLUMN next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE outbox_events ADD CONSTRAINT outbox_events_event_id_key UNIQUE (event_id);
DROP INDEX IF EXISTS idx_outbox_unpublished;
CREATE INDEX idx_outbox_due ON outbox_events (next_attempt_at, id) WHERE published_at IS NULL;
```
**Down** (chỉ dev/test):
```sql
SET LOCAL lock_timeout = '5s';
DROP INDEX IF EXISTS idx_outbox_due;
CREATE INDEX IF NOT EXISTS idx_outbox_unpublished ON outbox_events (id) WHERE published_at IS NULL;  -- y hệt 0001_core.sql:196
ALTER TABLE outbox_events DROP CONSTRAINT IF EXISTS outbox_events_event_id_key;
ALTER TABLE outbox_events DROP COLUMN IF EXISTS next_attempt_at, DROP COLUMN IF EXISTS last_error,
  DROP COLUMN IF EXISTS attempts, DROP COLUMN IF EXISTS headers, DROP COLUMN IF EXISTS event_id;
```

Checklist (đánh dấu khi duyệt):

- [ ] **Tác động khoá đã hiểu:** `gen_random_uuid()` là hàm volatile ⇒ `ADD COLUMN event_id` **ghi lại toàn bảng** `outbox_events` dưới
  `ACCESS EXCLUSIVE`, cộng thời gian dựng index UNIQUE và `idx_outbox_due`; khoá giữ tới COMMIT. Trong lúc đó mọi `PersistHold`/`ReleaseHold`
  (ghi outbox) bị chặn. `lock_timeout = 5s` chỉ giới hạn thời gian **chờ** lấy khoá (CI chứng minh lỗi `55P03` sau ≈5s), **không** giới hạn
  thời gian giữ khoá. Bảng khác không bị khoá.
- [ ] **Kiểm kích thước trước khi chạy:** `SELECT count(*) FROM outbox_events;` — bảng nhỏ thì chạy như trên; bảng lớn (hàng triệu row) thì
  dừng và dùng phương án cột nullable + backfill theo lô (ghi ở `docs/02`, không làm trong slice này).
- [ ] **Thứ tự deploy:** (1) chạy migration 0002 (service `migrate`, goose); (2) deploy `ticketing` server + `ticketing-worker` mới (worker
  cần `AMQP_URL`, thiếu thì thoát ngay). Code cũ vẫn chạy được trên schema 0002 trong khoảng giữa (mọi cột mới có DEFAULT/NULL; CI
  `AC8_InsertKieuCuChayTrenSchema0002VaNhanEventID`).
- [ ] **Rollback production = chỉ rollback ứng dụng**, giữ schema 0002 (forward-only, `docs/03:216`). Row do code cũ ghi trong lúc rollback
  vẫn được relay mới bọc phong bì đúng (`AC8_RowCuTruoc0002DuocBocPhongBi`).
- [ ] **Down chỉ dùng dev/test**; nếu buộc phải chạy: rút app mới + dừng `ticketing-worker` trước, rồi Down. Đã diễn tập Up→Down→Up trên DB có
  dữ liệu, schema sau Down khớp 0001 (cột, index, constraint) — CI `AC8_UpDownUpTraDungSchema0001`.
- [ ] **Rủi ro còn lại chấp nhận:** CI áp SQL trực tiếp, chưa chạy binary goose thật (r9) — chạy `make migrate` ở smoke compose trước production.
- [ ] **Ghi nhận duyệt:** tạo `docs/workflow/EVF-36-outbox-relay/approval.md` (người duyệt, thời điểm, SHA commit của migration — file
  được thêm ở `c2c347d`, không đổi tới HEAD `87969db`).

**Trạng thái:** `approval.md` **chưa tồn tại** (đã kiểm cả lịch sử git) ⇒ **merge bị chặn** cho tới khi người dùng duyệt. Ngoài ra #52 (EVF-7)
phải merge trước.
