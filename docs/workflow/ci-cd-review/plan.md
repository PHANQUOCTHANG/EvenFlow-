# Plan — sửa CI/CD (vòng 3, sau Plan Review vòng 2: KHÔNG ĐẠT vì N1)

## 00. Thay đổi so với vòng 2

| Phát hiện vòng 2 | Mức | Xử lý |
|---|---|---|
| N1 PR này đỏ **3 job** (`lint-go` 10 lỗi, `test-go` coverage domain/app 0% ở ticketing + waitingroom, `lint-python` ruff 6 lỗi) + `build` theo K5 | major | **Leo thang cho người dùng** — đây là quyết định thứ tự merge / chính sách, không phải thiết kế workflow. Bản diff CI giống nhau cho mọi lựa chọn trừ (c) `only-new-issues`. Hiện thực trước, hỏi trước khi merge |
| N2 `toJSON(needs)` + regex | minor | Điều kiện dùng biểu thức: `GATE_BAD: ${{ contains(needs.*.result, 'failure') \|\| contains(needs.*.result, 'cancelled') }}`; `toJSON(needs)` chỉ để in |
| N3 Gate 2 nuốt lỗi | minor | `for t in …; do bash "$t" \|\| exit 1; done` |
| N4 `BASE_REF` không sang step sau | minor | Tính và gọi `gates.sh` trong **cùng một** step |
| N5 `always()` + `cancel-in-progress` ⇒ run cũ bị huỷ có `build` đỏ | minor | Chấp nhận (đúng AC-8: cancelled ⇒ gate fail); ghi ở §4 |
| N6 comment `# v3` | minor | Comment ghi "commit của tag v3 tại 2026-10-06" |

Base `c848afa`. Mọi phiên bản dưới đây đã kiểm **tồn tại thật** bằng `git ls-remote` / `docker
manifest inspect`, không đoán.

## 0. Thay đổi so với vòng 1

| Phát hiện vòng 1 | Mức | Xử lý trong plan này |
|---|---|---|
| #1 `trivy-gate`: điều kiện ở `if:` của job ⇒ job skip ⇒ Success | blocker | §2.4, §4: K5 — `if: always()` + step `exit 1` |
| #2 `build` cùng lỗi | major | §2.1: áp K5 cho `build`; ghi B3 vào spec |
| #3 SC2129 sửa sai step | major | §3: gộp ba lệnh ghi `$GITHUB_OUTPUT` ở step `src` (dòng 50-52) |
| #4 PR này không chạy `lint-go` | major | §2.1: `ci.yml` vào filter `go` và `python` |
| #5 `trivy-action` pin bằng tag trong job có `packages: write` | major | §3: pin SHA cả 4 action bên thứ ba của `cd-web.yml` |
| #6 đường dẫn script trong `working-directory` | minor | §2.3: `$GITHUB_WORKSPACE/...` |
| #7 `before` mất sau force-push | minor | §2.1: thử `git fetch` trước, hỏng thì `::error::` |
| #8 nội suy `vars` vào shell | minor | §3: qua `env:` |
| #9 mất provenance | minor | chấp nhận, ghi vào spec §3 |
| #10 cách chạy `tests/*` | minor | §2.1: vòng lặp tường minh |
| #11 `trivy.yml` không tự kích hoạt | minor | §2.4: thêm chính nó vào filter |

## 1. File thay đổi, theo thứ tự

| # | File | AC |
|---|---|---|
| 1 | `.github/scripts/run-and-report-tests.sh` (**mới**) — đúng K1 | AC-4 |
| 2 | `Makefile` — `test-oversell` thêm `$(TESTFLAGS)` (K2) | AC-4 |
| 3 | `.github/workflows/oversell-gate.yml` | AC-4, 7 |
| 4 | `.github/workflows/integration.yml` | AC-4, 7 |
| 5 | `.github/workflows/ci.yml` | AC-1, 5, 6, 7, 8, 9, 10 |
| 6 | `.github/workflows/cd-web.yml` | AC-2, 3, 7, 10, 11 |
| 7 | `.github/workflows/trivy.yml` | AC-5, 7, 8 |
| 8 | `.github/workflows/e2e-nightly.yml` | AC-7, 9, 10 |
| 9 | `.github/scripts/tests/*` — **Test Design viết, Implementer không sửa** | mọi AC |
| 10 | `deploy/docker/ticketing.Dockerfile`, `deploy/docker/waitingroom.Dockerfile` — đưa `go.work` + `go.mod` của **mọi** module trong `go.work` vào stage build: `libs` và service đích đầy đủ, 7 service còn lại **chỉ `go.mod`** (bản đầu copy cả `services/`, đã đổi sau Code Review #1 vì kéo `.env`/`*.pem` chưa track vào build stage) (**mở rộng phạm vi 2026-10-08**, B4; chưa qua Plan Review ⇒ Code Review phải soi) | AC-12 |

## 2. Chi tiết

### 2.1 `ci.yml`

- `permissions: { contents: read, pull-requests: read }` ở cấp workflow.
- `NODE_VERSION: "22"`.
- Filter:
  - `go` += `.golangci.yml`, `.github/scripts/check-domain-coverage.sh`, `.github/workflows/ci.yml`
  - `web` += `deploy/docker/web.Dockerfile`, `.github/scripts/gates.sh`, `.github/workflows/ci.yml`
  - `python` += `.github/workflows/ci.yml`
  - `deploy` (**mới**) = `deploy/k8s/**`, `deploy/compose/**`
  - `ci` (**mới**) = `.github/workflows/**`, `.github/scripts/**`
- `lint-go`: ba lần `golangci/golangci-lint-action@v6` → `@v9`, `version: v2.12.2`.
- Job **mới** `gates-web` (`if: web`): `fetch-depth: 0`; Node 22; `npm ci` trong `apps/web`. Tính
  `BASE_REF` trong một step qua `env:` (không nội suy vào script):
  - `pull_request` ⇒ `github.event.pull_request.base.sha`
  - `push` ⇒ `github.event.before`; là 40 số 0 ⇒ không đặt (dùng mặc định của script)
  - nếu đã đặt mà `git cat-file -e "$BASE_REF^{commit}"` hỏng ⇒ `git fetch --no-tags origin
    "$BASE_REF"`; vẫn hỏng ⇒ `::error::` nói rõ "commit base không còn trong lịch sử (force-push?)" và
    `exit 1`
  - rồi `bash .github/scripts/gates.sh`
- Job **mới** `validate-deploy` (`if: deploy`): `docker run ghcr.io/yannh/kubeconform:v0.8.0 -strict
  -summary -kubernetes-version 1.32.0`, rồi `docker compose -f deploy/compose/docker-compose.yml
  config -q`.
- Job **mới** `lint-ci` (`if: ci`): `docker run rhysd/actionlint:1.7.7`; rồi
  `for t in .github/scripts/tests/*.sh; do bash "$t" || exit 1; done`.
- `build`: `needs` += `gates-web`, `validate-deploy`, `lint-ci`. **`if: always()`** (K5). Step đầu tiên
  mới (cũng `if: always()`), qua `env:`: `GATE_BAD: ${{ contains(needs.*.result, 'failure') ||
  contains(needs.*.result, 'cancelled') }}` — đây là **điều kiện**; `NEEDS_JSON: ${{ toJSON(needs) }}`
  **chỉ để in** khi fail (N2 — bản vòng 2 grep `NEEDS_JSON`, đã bỏ). `"$GATE_BAD" = "true"` ⇒
  `::error::` + `exit 1`. Các step build hiện có **giữ nguyên** `if` theo filter. Tên job không đổi
  (K3).

### 2.2 `oversell-gate.yml`

`permissions: contents: read`. Bước chạy:
`bash .github/scripts/run-and-report-tests.sh TestNoOversell make test-oversell TESTFLAGS=-v`.

### 2.3 `integration.yml`

`permissions: contents: read`. Bước smoke (có `working-directory: services/ticketing`):
`bash "$GITHUB_WORKSPACE/.github/scripts/run-and-report-tests.sh" TestNoOversell go test -v
-tags=integration -run TestNoOversell ./test/concurrency/... -count=20 -timeout=15m`. Bước integration
đầy đủ phía trên **không đổi**.

### 2.4 `trivy.yml`

- `permissions: { contents: read, pull-requests: read }`.
- Filter `any` += `.github/workflows/trivy.yml`; và (sau Challenge vòng 1) `go.work`, `services/**`,
  `.dockerignore` — image Go đọc `go.work` + go.mod của cả 9 module.
- `trivy-gate`: **`if: always()`** (K5); step (cũng `if: always()`): `GATE_BAD` như 2.1 ⇒ `::error::` +
  `exit 1`; ngược lại in thông báo cũ. `needs` giữ `[changes, scan]`.

### 2.5 `e2e-nightly.yml`

`permissions: contents: read`; Node `22`; `for i` → `for _` (SC2034). Không sửa gì khác. E2E đỏ ở
`make up` (spec §1.4) — nhiều khả năng do B4, đã sửa ở mục 10; chưa tái hiện riêng.

## 3. `cd-web.yml` (K4)

- `permissions` giữ `contents: read`, `packages: write`. Ba điều kiện chống fork **giữ nguyên từng
  chữ**.
- Step `src`: gộp ba `echo … >> "$GITHUB_OUTPUT"` thành `{ …; } >> "$GITHUB_OUTPUT"` (SC2129 — đây là
  chỗ actionlint thật sự báo, dòng 48).
- Pin SHA (đã `git ls-remote`, lấy commit sau khi bóc annotated tag):
  - `docker/setup-buildx-action@8d2750c68a42422c14e847fe6c8ac0403b4cbd6f # v3`
  - `docker/login-action@c94ce9fb468520275223c153574b00df6fe4bcc9 # v3`
  - `docker/build-push-action@10e90e3645eae34f1e60eeb005ba3a3d33f178e8 # v6`
  - `aquasecurity/trivy-action@ed142fd0673e97e23eac54620cfb913e5ce36c25 # v0.36.0`
  Giữ nguyên major hiện có (v3/v6) — nâng major là ngoài phạm vi.
- Step mới trước build: `env: API_BASE: ${{ vars.NEXT_PUBLIC_API_BASE }}`; rỗng ⇒ `::warning` (AC-3).
- `build-push-action`: bỏ `push: true`, thêm `load: true`, `build-args: NEXT_PUBLIC_API_BASE=${{
  vars.NEXT_PUBLIC_API_BASE }}` (đây là input của action, không phải script shell).
- Step mới: `trivy-action` quét `<image>:sha-<short>`, `severity: CRITICAL`, `ignore-unfixed: true`,
  `exit-code: '1'`.
- Step mới: `docker push` hai tag — chính image đã quét.
- Step tóm tắt: thêm dòng API base (đã dùng `{ …; } >>` sẵn).

## 4. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| `lint-go` bật lên sẽ **đỏ ngay** vì 10 lỗi lint có sẵn (đo tại chỗ) — và vì `ci.yml` nằm trong filter `go`, **chính PR này** sẽ đỏ `lint-go` | Đúng hành vi của một gate vừa được sửa. Báo người dùng **trước khi merge**; sửa 10 lỗi là việc riêng của mã backend |
| `build` `if: always()` ⇒ chạy cả khi job trước fail | Step đầu `exit 1` ⇒ `build` **fail** chứ không skip: đó là mục đích (B3) |
| CD quét Trivy ⇒ có thể ngừng publish | Đúng chính sách EVF-6; lockfile web sạch CRITICAL (spec §1.3), `trivy (web)` xanh trên `main` (§1.4) |
| `gates-web` chậm thêm ~3–4 phút | Chấp nhận; gộp job là đổi tên job (K3) |
| ~~`toJSON(needs)` định dạng có khoảng trắng khác dự kiến~~ | Không còn áp dụng: điều kiện dùng `contains(needs.*.result, …)` (N2), `toJSON` chỉ để in |

## 5. Rollback

`git revert` từng commit. Không đổi trạng thái ngoài repo (không secret, không repo variable).

## 6. Gates (bước 6, không LLM)

1. `actionlint` 1.7.7 ⇒ 0 phát hiện (baseline: 2)
2. `for t in .github/scripts/tests/*.sh; do bash "$t"; done` ⇒ tất cả pass
3. kubeconform + `docker compose config` ⇒ sạch
4. `bash .github/scripts/gates.sh` ⇒ exit 0 (không hồi quy)
5. `git diff --name-only c848afa` chỉ chứa file ở §1 + `docs/workflow/ci-cd-review/`
6. Đã đo trước (không lặp): golangci-lint v2.12.2 `config verify` OK; `run` ⇒ 0 / 7 / 3 lỗi
