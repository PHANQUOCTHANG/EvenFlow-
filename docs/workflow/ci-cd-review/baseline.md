# Baseline — kết quả test trên trạng thái CHƯA SỬA (`c848afa`)

Lúc chạy baseline, worktree **đã có** bản hiện thực (workflow đã sửa, `run-and-report-tests.sh`,
Makefile có `TESTFLAGS`). Để đo trên bản gốc mà không đụng worktree (không stash/checkout/reset), base
được export ra thư mục tạm rồi trỏ hai script vào đó bằng tham số `repo-root`:

```
B=$(mktemp -d)
git -c core.autocrlf=false archive c848afa | tar -x -C "$B"
bash .github/scripts/tests/test-run-and-report-tests.sh "$B"
bash .github/scripts/tests/test-workflows.sh          "$B"
```

Máy chạy chính: Windows 11, Git Bash 5.2.37, GNU awk 5.3.2, **không có** `make` ⇒ hai assertion K2 (`ac4_mk_unchanged`, `ac4_mk_with_v`) chạy
nhánh mô phỏng bằng awk; ca thứ ba (`recipe … dung $(TESTFLAGS)`) luôn dùng awk/grep, có make hay không
cũng vậy. *(Sửa chữ bởi Integrator sau Verification vòng 2, C2-3.)* Có `make` (vd ubuntu-latest, job `lint-ci`) thì chạy nhánh `make -n`.

**Sửa sau Independent Verification:** nhánh `make -n` trước đây **chưa ai chạy** và **hỏng** — `make -n`
in recipe nguyên dạng nhiều dòng (`\` + xuống dòng + tab), nên so chuỗi luôn lệch ⇒ 2 assertion K2 FAIL
giả trên Linux có make (worktree 126/128, base 37/91). Câu "trên ubuntu-latest thì dùng `make -n`" ở bản
trước vì vậy là sai. Đã sửa: cả hai nhánh đi qua cùng một hàm `norm_cmd` (bỏ CR, nối dòng `\`, gom khoảng
trắng — kể cả hai dấu cách mà `make` để lại khi `TESTFLAGS` rỗng). Kiểm lại trong `ubuntu:24.04` (mawk),
trên bản sao worktree đã đổi sang LF và trên export `c848afa`:

| | K1 | Workflow |
|---|---|---|
| worktree, **không** make | 35 / 0 | 128 / 0 |
| worktree, GNU Make 4.3 | 35 / 0 | 128 / 0 |
| base `c848afa`, **không** make | 0 / 35 | 38 / 90 |
| base `c848afa`, GNU Make 4.3 | 0 / 35 | 38 / 90 |

(PASS / FAIL.) Nhánh make vẫn bắt được thay đổi thật: Makefile bỏ `-count=200` hoặc bỏ
`-tags=integration` ⇒ `[AC-4] Makefile: khong truyen TESTFLAGS => lenh y het hien tai` và
`[AC-4] Makefile: TESTFLAGS=-v => ...` đều FAIL (GNU Make 4.3, ubuntu:24.04). Cách tái hiện:

```
# <dir>/wt = ban sao worktree da doi LF, <dir>/base = export c848afa
docker run --rm -v "<dir>:/w" ubuntu:24.04 bash -c '
  cd /w/wt
  bash .github/scripts/tests/test-workflows.sh /w/wt;  bash .github/scripts/tests/test-workflows.sh /w/base
  apt-get update -qq && apt-get install -y -qq make
  bash .github/scripts/tests/test-workflows.sh /w/wt;  bash .github/scripts/tests/test-workflows.sh /w/base'
```

> Cập nhật sau Challenge vòng 1: K1 có 35 ca (thêm T31–T35), workflow có 128 assertion (thêm 23).
> Số liệu dưới đây là của bản mới; §6 ghi vòng tăng cứng.

## 1. `test-run-and-report-tests.sh` (K1) — 35 ca

**Base: 0 PASS, 35 FAIL** (exit 1). Script `.github/scripts/run-and-report-tests.sh` không tồn tại ở
`c848afa`.

Ca fail: T01..T35 (tất cả).

**Một ca từng pass trên base và đã sửa lại.** Bản đầu tiên cho 4 PASS / 26 FAIL. Bốn ca pass là
T08 (không có `::warning`), T13 (exit 127), T26, T27 (không tạo file). Cả bốn pass **vô nghĩa**: không
có script thì đương nhiên không có cảnh báo, không có file; còn `bash <file-không-tồn-tại>` trả 127 đúng
bằng mã mà T13 chờ. Đã thêm điều kiện: SUT không tồn tại ⇒ mọi ca FAIL. Kết quả trên là sau khi sửa.

## 2. `test-workflows.sh` (AC-1..AC-12, K2..K5) — 128 assertion

**Base: 38 PASS, 90 FAIL** (exit 1). (Bản 105 assertion trước đó: 34 PASS, 71 FAIL.)

### 2.1 Fail (90) — đúng như mong đợi

| AC | Số fail | Ca |
|---|---|---|
| AC-1 | 4 | còn `@v6`; còn `version: latest`; action không hỗ trợ v2; không pin `v2.X.Y` |
| AC-2 | 18 | tất cả assertion của AC-2 (13 cũ + Trivy không `if:`, image-ref ∈ ref được push, ref push ∈ tag đã build, login sau Trivy/trước push, `persist-credentials: false`) |
| AC-3 | 5 | build-arg; nằm dưới `build-args:`; `::warning`; đọc qua `env:`; summary |
| AC-4 | 6 | oversell-gate và integration không gọi wrapper; Makefile không có `$(TESTFLAGS)`; `TESTFLAGS=-v` không thêm `-v`; 2 × "exit code wrapper không bị nuốt" |
| AC-5 | 17 | 13 cũ + trivy.yml không phủ `go.work`, `services/gateway/**`, `services/notification/**`, `.dockerignore` |
| AC-6 | 6 | tất cả |
| AC-7 | 12 | `permissions:` + `contents: read` ở ci, e2e-nightly, integration, oversell-gate, trivy (10); `pull-requests: read` ở ci, trivy (2) |
| AC-8 | 16 | 8 assertion × 2 gate (`trivy-gate`, `build`): 5 cũ + continue-on-error, `if:` của step, literal so sánh |
| AC-9 | 4 | `NODE_VERSION`; Node 20 còn; setup-node ci.yml; setup-node e2e-nightly.yml |
| AC-12 | 2 | ticketing.Dockerfile, waitingroom.Dockerfile — thiếu 7 `go.mod` (đúng B4) |

Lý do in ra khớp với spec §1.2: B1 (`@v6` + `latest`), B2/M4 (CD không quét, `push: true`, không
build-arg), M1 (không wrapper), M2/M3 (filter, không gates), m1 (không permissions), B3/m2
(`if: always() && !cancelled() && !contains(...)` ở cả `trivy-gate` lẫn `build`), m3 (Node 20).

### 2.2 Pass trên base (38) — xét từng ca xem có kiểm gì không

Bốn ca mới pass ở base, đều có lý do:
- `[AC-5] trivy.yml: doi services/ai-worker/app/main.py => job trivy chay` — base đã có `services/ai-worker/**`.
- `[AC-8] {trivy-gate, build}: needs: chua MOI job khac trong file` — base đã `needs` đủ. Đây là ca không
  hồi quy; nó fail khi bớt một job (C-M3, C-M4) hoặc khi thêm job mới mà quên đưa vào `build.needs`.
- `[AC-12] go.work co khoi use (...) doc duoc` — điều kiện tiên quyết: nếu không đọc được danh sách
  module thì hai ca AC-12 kia sẽ pass vô nghĩa.

34 ca còn lại như bảng dưới (không đổi so với bản trước):

Không ca nào pass vì "không kiểm gì". Tất cả thuộc một trong ba loại dưới đây, và mỗi ca đều fail được
(xem §3):

| Loại | Ca | Vì sao pass ở base là đúng |
|---|---|---|
| **Không hồi quy** (AC-11, K2, K5) | 15 × `[AC-11] K3: ... con job '...'`; 4 × điều kiện `if:` của CD; `cd-web: van chi kich hoat sau workflow 'CI' tren main`; `khong ... kubectl apply ...`; `chi cd-web.yml duoc push image`; `[AC-4] Makefile: khong truyen TESTFLAGS => lenh y het hien tai`; `[AC-8] ci.yml/build: cac step build giu dieu kien theo filter`; `[AC-1] job lint-go van goi golangci-lint` | Mô tả hành vi **đang có** mà bản sửa không được làm mất. Ở base, chúng phải pass |
| **cd-web.yml đã đúng một phần** | `[AC-7] cd-web.yml: khai permissions:` / `co contents: read` / `co packages: write va KHONG co quyen ghi nao khac` | `cd-web.yml` ở base đã có `permissions: contents: read, packages: write` (spec m1 chỉ liệt kê 5 file kia) |
| **Ràng buộc phủ định, đi cặp với ca dương** | `[AC-7] <5 file>: khong co quyen ghi nao`; `[AC-3] khong workflow nao noi suy vars.NEXT_PUBLIC_API_BASE thang vao run:` | Ở base chúng pass vì **chưa có gì** (chưa khai permissions, chưa dùng biến). Riêng lẻ thì vô nghĩa; chúng có ý nghĩa vì ca dương tương ứng (`khai permissions:` / `step canh bao doc bien qua env:`) fail ở base và bắt bản sửa phải thêm thứ mà ràng buộc phủ định sẽ soi |

Ca pass vô nghĩa đã tìm được và sửa: `[AC-2] step docker push khong co if: always()...` bản đầu pass
khi **không có** step `docker push` nào. Đã sửa: không có step push ⇒ FAIL.

### 2.3 AC-10 — actionlint (lệnh, không nằm trong script)

```
MSYS_NO_PATHCONV=1 docker run --rm -v "<base>:/repo" -w /repo rhysd/actionlint:1.7.7 -no-color .github/workflows/*.yml
```

Base: **exit 1, 2 phát hiện** — `cd-web.yml:48` SC2129, `e2e-nightly.yml:23` SC2034. Khớp spec §1.2 m4.
(Bản `git archive` không có `.git`, nên phải truyền danh sách file; trong worktree thật thì chạy không
tham số.)

## 3. Mỗi assertion fail được — cách đã thử

*Phần này ghi **vòng đầu**, khi bộ test còn 30 ca K1 / 105 assertion. Vòng tăng cứng sau Challenge
(35 / 128) ở §6.*

Hai bản "đúng" tự viết trong thư mục tạm (không nằm trong repo, không phải bản hiện thực):

- **K1:** một `run-and-report-tests.sh` mẫu ~20 dòng (`"$@" 2>&1 | tee`, `PIPESTATUS`, `grep -Ec`). Kết
  quả vòng đầu: 30/30 PASS ⇒ không ca nào fail vô điều kiện.
- **Workflow:** bản sao của base với các sửa theo spec (Trivy + `load: true` + `docker push`, SHA pin,
  filter, một job validate manifest — trong bản mẫu tôi đặt tên `validate-manifests`; bản hiện thực dùng
  job id `validate-deploy`, và test không phụ thuộc tên job này —, job `gates-web`, `if: always()` + step
  `exit 1`, permissions, Node 22, Makefile `$(TESTFLAGS)`). Kết quả vòng đầu: 105/105 PASS.

Rồi **đột biến** từng bản đúng, mỗi đột biến chỉ đổi một chỗ, và kiểm rằng đúng assertion mong đợi
chuyển sang FAIL:

**K1 — 20 đột biến, đột biến nào cũng bị bắt** (số ca fail trong ngoặc):
`rc=$?` thay `PIPESTATUS` (6: T10 T11 T13 T14 T21 T22) · `>` thay `>>` (1: T24) · bỏ neo
`^[[:space:]]*` (10) · bỏ `2>&1` (1: T20) · in cảnh báo hai lần (6) · chỉ cảnh báo khi SKIP>0 (2: T05
T06) · thiếu tham số exit 1 (2: T01 T02) · neo `^---` không cho thụt lề (3: T14 T15 T22) · ghi
`summary.md` khi biến không đặt (2: T26 T27) · không dọn file tạm (2: T26 T27) · `$*` thay `"$@"` (1:
T30) · FAIL>0 + exit 0 vẫn exit 0 (1: T09) · nuốt output (3: T20 T28 T30) · `.*` trước regex (1: T18)
· cảnh báo cả khi có PASS (3) · summary thiếu `KHONG kiem gi` (1: T25) · FAIL đếm cả test khác (2) ·
stderr bỏ đi (2) · summary không phải bảng (2) · mọi exit ≠ 0 thành 1 (4).

**Workflow — 63 đột biến, cả 63 cho đúng kết quả mong đợi**: 61 đột biến dương, mỗi cái làm đúng
assertion nhắm tới chuyển sang FAIL; 2 đột biến *âm* phải **không** gây FAIL nào (và đúng là không):
- thêm dòng comment `# version: latest` (comment không được tính);
- `kubectl apply --dry-run=client` (dry-run được phép).

Một đột biến dương đáng chú ý: thay khối permissions thật bằng comment `# permissions:` ⇒ **phải**
FAIL, và có FAIL (comment không được tính là đã khai báo).

Danh sách đột biến dương (rút gọn): `@v8→@v6`; `version→latest`; `version→v1.64.8`; xoá
golangci-lint; `severity` thêm HIGH; `ignore-unfixed: false`; `exit-code '0'`; `scan-type: fs`;
`continue-on-error` ở Trivy; `load→push: true`; `docker push` trước Trivy; `if: always()` ở push; build
lại sau Trivy; Trivy/login-action pin tag thay vì SHA; bỏ build-arg; nội suy `vars.` vào `run:`; bỏ
`::warning`; bỏ `env:` của step cảnh báo; bỏ dòng summary; `TESTFLAGS=` rỗng; bỏ wrapper; bỏ `-v`;
`-count=2`; Makefile bỏ `$(TESTFLAGS)`; `-count=100`; `TESTFLAGS ?= -v`; bỏ từng path filter (web
Dockerfile, ci.yml khỏi go, ci.yml khỏi python, trivy.yml khỏi trivy); bỏ `-strict`; `config→ps`; gate
job manifest theo filter sai; `fetch-depth: 1`; bỏ `BASE_REF`; `gates.sh || true`; không gọi gates.sh;
thêm `issues: write`; thêm `id-token: write` vào CD; bỏ `packages: write`; bỏ `pull-requests: read`;
`if:` của gate có `!contains(...)` / `!cancelled()`; `exit 0` thay `exit 1`; bỏ `::error`; bỏ nhánh
`cancelled`; thêm nhánh `skipped`; bỏ `if:` filter của step build; Node 20 (env và literal); đổi tên job
CD / `lint-go`; bỏ từng điều kiện chống fork; `refs/heads/develop`; bỏ `conclusion == 'success'`;
`workflows: ["CI2"]`; thêm `kubectl apply` thật; thêm `docker push` ngoài CD.

## 4. Lỗi của chính bộ test, lộ ra khi chạy trên bản hiện thực

Lần chạy đầu trên worktree báo FAIL `[AC-4] integration: smoke ...` với lý do "tham số `<test-regex>`
không phải TestNoOversell". Đó là **lỗi của test**, không phải của bản hiện thực: bản hiện thực viết
`bash "$GITHUB_WORKSPACE/.github/scripts/run-and-report-tests.sh" TestNoOversell`, dấu `"` đóng ngay sau
`.sh` mà regex không cho phép. Hợp lệ theo K1. Đã nới regex (cho phép dấu nháy sau `.sh`), và nới
tương tự cho cách gọi `gates.sh` qua đường dẫn tuyệt đối có nháy (AC-6). Sau khi sửa, đã chạy lại bản
"đúng" (lúc đó 105/105 — bộ 105 assertion của vòng đầu) và các đột biến liên quan (M20 M21 M22 M22b M32
M32b) — vẫn bị bắt.

Lỗi thứ hai, do Independent Verification tìm ra: nhánh `make -n` của K2 không nối dòng `\` ⇒ FAIL giả
trên Linux có make. Đã sửa; bảng bốn cấu hình (có/không make × worktree/base) ở đầu file.

## 5. Kết quả trên worktree hiện tại (bản hiện thực, sau Challenge vòng 1)

| Script | PASS | FAIL |
|---|---|---|
| `test-run-and-report-tests.sh` | 35 | 0 |
| `test-workflows.sh` | 128 | 0 |
| actionlint 1.7.7 (AC-10) | exit 0, 0 phát hiện (đo ở vòng trước) | — |

## 6. Tăng cứng sau Challenge vòng 1

Challenger chỉ ra những đột biến workflow vẫn để bộ 105 assertion xanh. Tôi thêm assertion, rồi áp từng
đột biến lên một **bản sao của worktree hiện tại** (`.github/workflows`, `deploy/docker`, `Makefile`,
`go.work`) trong thư mục tạm và chạy `bash .github/scripts/tests/test-workflows.sh <tmp-root>`. Bản sao
chưa đột biến: 128/128 PASS (đối chứng).

| Đột biến | Assertion bắt được (FAIL) |
|---|---|
| C-M1a `continue-on-error: true` ở step gate của `trivy-gate` | `[AC-8] trivy.yml/trivy-gate: step gate / job khong co continue-on-error` |
| C-M1b như trên, `build` (ci.yml) | `[AC-8] ci.yml/build: step gate / job khong co continue-on-error` |
| C-M2a step gate `if: github.event_name == 'never'` (trivy-gate) | `[AC-8] trivy.yml/trivy-gate: if: cua step gate bo trong hoac DUNG BANG always()` |
| C-M2b như trên, `build` | `[AC-8] ci.yml/build: if: cua step gate ...` |
| C-M2n *(âm)* bỏ hẳn `if:` của step gate | không FAIL nào (bỏ trống được chấp nhận, `always()` cũng được) |
| C-M3 `trivy-gate` `needs: [scan]` | `[AC-8] trivy.yml/trivy-gate: needs: chua MOI job khac trong file` |
| C-M4 bỏ `lint-ci` khỏi `build.needs` | `[AC-8] ci.yml/build: needs: chua MOI job khac trong file` |
| C-M4b bỏ `lint-go` khỏi `build.needs` | như trên |
| C-M5a `"$GATE_BAD" = "True"` (trivy-gate) | `[AC-8] trivy.yml/trivy-gate: step gate so ket qua bieu thuc voi dung literal 'true'` |
| C-M5b `"$GATE_BAD" = "1"` (build) | `[AC-8] ci.yml/build: step gate so ket qua ...` |
| C-M6 `if: github.ref == 'refs/heads/main'` ở step Trivy của CD | `[AC-2] cd-web: step Trivy KHONG co if:` |
| C-M7 `image-ref: ...:scan` (không phải tag được push) | `[AC-2] cd-web: image-ref cua Trivy la mot trong cac ref duoc docker push` |
| C-M7b thêm `docker push "$IMAGE:other"` | `[AC-2] cd-web: moi ref duoc docker push la tag cua image da build (load) roi quet` |
| C-M10a `make test-oversell TESTFLAGS=-v \|\| true` | `[AC-4] oversell-gate: exit code cua run-and-report-tests.sh khong bi nuot` |
| C-M10b `... -timeout=15m \|\| true` (integration) | `[AC-4] integration: exit code ... khong bi nuot` |
| C-M10c `... -timeout=15m; true` (integration) | như trên |
| C-M10d `continue-on-error: true` ở step wrapper (oversell-gate) | `[AC-4] oversell-gate: exit code ... khong bi nuot` |
| C-M9 `export BASE_REF="$base"` → `BASE_REF="$base"` | `[AC-6] ci.yml: job gates dat BASE_REF` |
| C-N1 bỏ `go.work` khỏi filter `any` (trivy.yml) | `[AC-5] trivy.yml: doi go.work => job trivy (scan) chay` |
| C-N2 bỏ `services/**` | `[AC-5] trivy.yml: doi services/{gateway/go.mod, notification/cmd/main.go, ai-worker/app/main.py} => ...` (3) |
| C-N3 bỏ `.dockerignore` | `[AC-5] trivy.yml: doi .dockerignore => job trivy (scan) chay` |
| C-N4 ticketing.Dockerfile bỏ `COPY services/gateway/go.mod` | `[AC-12] deploy/docker/ticketing.Dockerfile: COPY go.work + go.mod ...` |
| C-N5 waitingroom.Dockerfile bỏ `COPY libs ./libs` | `[AC-12] deploy/docker/waitingroom.Dockerfile: ...` |
| C-N6 thêm `./services/newmod` vào `go.work` (danh sách lấy từ go.work, không viết cứng) | `[AC-12]` cả hai Dockerfile |
| C-N7 `COPY services/identity/go.mod ./` (sai đích) | `[AC-12] deploy/docker/ticketing.Dockerfile: ...` |
| C-N7b waitingroom.Dockerfile bỏ `COPY go.work ./` | `[AC-12] deploy/docker/waitingroom.Dockerfile: ...` |
| C-N8 chuyển `docker/login-action` lên trước Trivy | `[AC-2] cd-web: docker/login-action nam SAU Trivy va TRUOC docker push` |
| C-N9 `persist-credentials: true` | `[AC-2] cd-web: actions/checkout co persist-credentials: false` |
| C-NEG2 *(âm)* thay `COPY services/gateway/go.mod ...` bằng `COPY services ./services` | không FAIL nào (copy cả thư mục cha được chấp nhận) |

Mỗi đột biến dương làm **đúng một** assertion nhắm tới FAIL (C-N2: 3, C-N6: 2, đúng như thiết kế); các
assertion khác vẫn PASS. C-N9 bản đầu tiên không có hiệu lực (sed thay nhầm dòng comment chứa cùng
chuỗi) — sửa đột biến cho trúng dòng YAML thật thì bị bắt.

**K1 (T31–T35)** — kiểm trên một bản cài đặt mẫu mới (nhóm regex, bỏ `^`/`$`, fail-closed): 35/35
PASS; và 4 đột biến của bản mẫu:

| Đột biến bản mẫu | Ca bắt được |
|---|---|
| G1 không nhóm regex (`--- PASS: ${re}` thay vì `(${re})`) | T31, T32 |
| G2 không bỏ `^`/`$` | T33 |
| G3 regex không hợp lệ rơi về đếm 0, exit 0 | T34 |
| G4 `mktemp` hỏng mà vẫn chạy tiếp | T35 |

Phần AC đánh dấu **manual** trong `traceability.md` vẫn chưa kiểm được: chỉ biết được sau khi chạy
thật trên GitHub.
