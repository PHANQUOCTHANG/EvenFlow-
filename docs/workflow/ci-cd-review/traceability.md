# Traceability — CI/CD review

Test viết **chỉ** từ `spec.md` + `contracts.md` (bản sau Plan Review vòng 1: AC-8/K5, AC-2/K4 mới,
AC-5 mở rộng; và sau Challenge vòng 1: K1 regex/fail-closed, AC-12, các lỗ do challenger chỉ ra).
Hai script, chạy từ gốc repo:

```
bash .github/scripts/tests/test-run-and-report-tests.sh [repo-root]   # 35 ca, hành vi K1
bash .github/scripts/tests/test-workflows.sh          [repo-root]   # 128 assertion, tĩnh
```

Tên ca dưới đây là tiền tố in ra trong output (`T07`, hoặc `[AC-x] <mô tả>`). `WF` = `test-workflows.sh`,
`RRT` = `test-run-and-report-tests.sh`.

Loại: **behaviour** = chạy script thật với lệnh giả; **static** = grep/awk trên YAML/Makefile;
**command** = chạy công cụ ngoài; **manual** = chỉ kiểm được bằng một lần chạy thật trên GitHub.

Trạng thái: cột *Base* = kết quả trên `c848afa` (xem `baseline.md`); cột *Hiện tại* ghi ở phần cuối
`baseline.md` cho worktree tại thời điểm chạy.

| AC | Test (file — ca) | Loại | Base |
|---|---|---|---|
| AC-1 | WF — `[AC-1] khong workflow nao con golangci/golangci-lint-action@v6` | static | FAIL |
| AC-1 | WF — `[AC-1] khong workflow nao con 'version: latest'` (bỏ qua comment) | static | FAIL |
| AC-1 | WF — `[AC-1] job lint-go van goi golangci-lint` (chống "sửa" bằng cách xoá lint) | static | PASS (không hồi quy) |
| AC-1 | WF — `[AC-1] golangci-lint-action ... ban ho tro v2 (@v7+ hoac SHA)` | static | FAIL |
| AC-1 | WF — `[AC-1] golangci-lint duoc pin dang vX.Y.Z voi X=2` | static | FAIL |
| AC-1 | `lint-go` thật sự xanh trên GitHub (action + binary + config v2 tương thích **khi chạy**) | **manual** — xem PR chạy `lint-go`. Lưu ý spec §3: 10 lỗi lint có sẵn ⇒ job sẽ *chạy được* nhưng đỏ | — |
| AC-2 | WF — `[AC-2] cd-web: job CD co step aquasecurity/trivy-action` | static | FAIL |
| AC-2 | WF — `[AC-2] ... severity = CRITICAL` / `ignore-unfixed = true` / `exit-code = '1'` / `quet IMAGE` | static | FAIL |
| AC-2 | WF — `[AC-2] ... khong co continue-on-error: true` | static | FAIL (vì chưa có step Trivy) |
| AC-2 | WF — `[AC-2] ... khong con 'push: true'` / `moi docker/build-push-action dung load: true` (K4.1) | static | FAIL |
| AC-2 | WF — `[AC-2] ... build dung TRUOC Trivy` / `co 'docker push' va moi docker push dung SAU Trivy` | static | FAIL |
| AC-2 | WF — `[AC-2] ... step docker push khong co if: always()/failure()/cancelled()` | static | FAIL (chưa có docker push) |
| AC-2 | WF — `[AC-2] ... khong build lai sau Trivy` (push đúng image đã quét) | static | FAIL |
| AC-2 | WF — `[AC-2] ... moi uses: ben thu ba ... pin SHA 40 hex` | static | FAIL |
| AC-2 | WF — `[AC-2] cd-web: step Trivy KHONG co if:` *(Challenge)* | static | FAIL (chưa có step Trivy) |
| AC-2 | WF — `[AC-2] cd-web: image-ref cua Trivy la mot trong cac ref duoc docker push` (K4.3, đã thay biến `env:` của step push) *(Challenge)* | static | FAIL |
| AC-2 | WF — `[AC-2] cd-web: moi ref duoc docker push la tag cua image da build (load) roi quet` *(Challenge)* | static | FAIL |
| AC-2 | WF — `[AC-2] cd-web: docker/login-action nam SAU Trivy va TRUOC docker push` *(Challenge)* | static | FAIL |
| AC-2 | WF — `[AC-2] cd-web: actions/checkout co persist-credentials: false` *(Challenge)* | static | FAIL |
| AC-2 | Quét fail ⇒ thật sự không tag nào lên GHCR | **manual** — cần một lần CD với image có CRITICAL (hoặc dispatch thử trên fork); static chỉ chứng minh *thứ tự* step và không có đường vòng | — |
| AC-3 | WF — `[AC-3] ... truyen NEXT_PUBLIC_API_BASE=${{ vars.NEXT_PUBLIC_API_BASE }}` / `... nam duoi build-args:` | static | FAIL |
| AC-3 | WF — `[AC-3] ... co step in ::warning khi NEXT_PUBLIC_API_BASE rong` | static | FAIL |
| AC-3 | WF — `[AC-3] khong workflow nao noi suy vars.NEXT_PUBLIC_API_BASE thang vao run:` (K4) | static | PASS (vô nghĩa ở base: chưa có biến) |
| AC-3 | WF — `[AC-3] ... step canh bao doc bien qua env:` (K4) | static | FAIL |
| AC-3 | WF — `[AC-3] ... co dong job summary nhac API base` | static | FAIL |
| AC-3 | Nội dung câu summary nói rõ "URL tương đối"; annotation hiện trên UI | **manual** — xem summary của một run CD khi biến chưa đặt | — |
| AC-4 | RRT — T01, T02 (thiếu tham số ⇒ exit 2) | behaviour | FAIL |
| AC-4 | RRT — T03, T04 (toàn SKIP ⇒ exit 0 + đúng một `::warning` `KHONG kiem gi`) | behaviour | FAIL |
| AC-4 | RRT — T05, T06 (0/0/0 và output rỗng ⇒ vẫn cảnh báo) | behaviour | FAIL |
| AC-4 | RRT — T07, T08 (có PASS ⇒ exit 0, không `::warning`) | behaviour | FAIL |
| AC-4 | RRT — T09 (FAIL + exit 0 ⇒ exit 1), T10/T11/T12/T13/T21 (exit 3/42/1/127/6 giữ nguyên) | behaviour | FAIL |
| AC-4 | RRT — T14 (ví dụ K1: SKIP=1 PASS=2 FAIL=1), T15 (subtest thụt lề space + tab) | behaviour | FAIL |
| AC-4 | RRT — T16..T19 (chỉ đếm test khớp regex, đúng vị trí, đúng định dạng dòng) | behaviour | FAIL |
| AC-4 | RRT — T20 (dòng in ra stderr cũng được ghi lại và đếm) | behaviour | FAIL |
| AC-4 | RRT — T22, T23 (bảng markdown có PASS/FAIL/SKIP/exit đúng số), T24 (append, giữ nội dung cũ) | behaviour | FAIL |
| AC-4 | RRT — T25 (summary toàn SKIP có `KHONG kiem gi` — lấy từ chữ của AC-4, K1 không nói) | behaviour | FAIL |
| AC-4 | RRT — T26, T27 (không đặt summary ⇒ không file nào; không rác trong TMPDIR/cwd) | behaviour | FAIL |
| AC-4 | RRT — T28, T29 (stdout/stderr của lệnh vẫn hiện ra), T30 (`[args...]` nguyên vẹn) | behaviour | FAIL |
| AC-4 | RRT — T31 (`TestA\|TestB`, cả hai PASS ⇒ exit 0, không cảnh báo), T32 (regex được **nhóm**: chỉ `--- SKIP: TestB` ⇒ PASS=0 SKIP=1, vẫn cảnh báo) — K1 "Regex" | behaviour | FAIL |
| AC-4 | RRT — T33 (`^TestX$` + `--- PASS: TestX` ⇒ bỏ `^`/`$`, không cảnh báo) — K1 "Regex" | behaviour | FAIL |
| AC-4 | RRT — T34 (regex `Test[` ⇒ exit 2), T35 (`TMPDIR=/nonexistent/x` ⇒ exit 2) — K1 "Fail-closed" | behaviour | FAIL |
| AC-4 | WF — `[AC-4] oversell-gate: goi run-and-report-tests.sh TestNoOversell boc 'make test-oversell TESTFLAGS=-v'` | static | FAIL |
| AC-4 | WF — `[AC-4] integration: smoke goi run-and-report-tests.sh ... -v ... -count=20` | static | FAIL |
| AC-4 | WF — `[AC-4] {oversell-gate, integration}: exit code cua run-and-report-tests.sh khong bi nuot (\|\| true, ; true, continue-on-error)` *(Challenge)* | static | FAIL (chưa có wrapper) |
| AC-4 | WF — `[AC-4] Makefile: recipe test-oversell dung $(TESTFLAGS)` (K2) | static | FAIL |
| AC-4 | WF — `[AC-4] Makefile: khong truyen TESTFLAGS => lenh y het hien tai` (K2) | static (`make -n` nếu có make, không thì mô phỏng bằng awk; cả hai qua cùng `norm_cmd`) | PASS (không hồi quy). Đã kiểm **cả hai nhánh** trong ubuntu:24.04: không make và GNU Make 4.3 đều PASS trên base và worktree. Trước bản sửa sau Independent Verification, nhánh make FAIL giả (không nối dòng `\`) — xem baseline.md |
| AC-4 | WF — `[AC-4] Makefile: TESTFLAGS=-v => them dung token -v` (K2) | static | FAIL |
| AC-4 | Trên GitHub, oversell gate hiện annotation + summary "0 PASS, N SKIP" | **manual** — xem run `oversell-gate` / `integration` đầu tiên sau merge | — |
| AC-5 | WF — `[AC-5] ci.yml: doi {deploy/docker/web.Dockerfile, .github/scripts/gates.sh, .github/workflows/ci.yml} => job {lint-web, test-web} chay` (6 assertion) | static | FAIL |
| AC-5 | WF — `[AC-5] ci.yml: doi .github/workflows/ci.yml => job {lint-go, test-go, lint-python, test-python} chay` (4) | static | FAIL |
| AC-5 | WF — `[AC-5] ci.yml: doi deploy/k8s/** => co job chay 'kubeconform -strict'` | static | FAIL |
| AC-5 | WF — `[AC-5] ci.yml: doi deploy/compose/** => co job chay 'docker compose ... config'` | static | FAIL |
| AC-5 | WF — `[AC-5] trivy.yml: doi .github/workflows/trivy.yml => job trivy (scan) chay` | static | FAIL |
| AC-5 | WF — `[AC-5] trivy.yml: doi {go.work, services/gateway/go.mod, services/notification/cmd/main.go, services/ai-worker/app/main.py, .dockerignore} => job trivy (scan) chay` (filter `any` phủ `go.work`, `services/**`, `.dockerignore`) *(Challenge)* | static | go.work, gateway, notification, .dockerignore FAIL; ai-worker PASS (base đã có `services/ai-worker/**`) |
| AC-5 | dorny/paths-filter thật sự đánh giá pattern như mô phỏng (glob `**`, dotfile) | **manual** — một PR chỉ sửa `deploy/k8s/*` phải kích hoạt job validate | — |
| AC-6 | WF — `[AC-6] ci.yml: co job chay bash .github/scripts/gates.sh` | static | FAIL |
| AC-6 | WF — `[AC-6] ... fetch-depth: 0` / `dat BASE_REF` / `BASE_REF = base cua PR` / `= github.event.before` | static | FAIL |
| AC-6 | (trong `[AC-6] ci.yml: job gates dat BASE_REF`) *(Challenge)*: BASE_REF phải **tới được** tiến trình `gates.sh` — `env:` của job/step, `export`/`declare -x`, hoặc tiền tố `BASE_REF=... bash gates.sh`. Gán trần `BASE_REF="$x"` ⇒ FAIL | static | FAIL |
| AC-6 | WF — `[AC-6] ... loi cua gates.sh khong bi nuot` | static | FAIL (chưa có job) |
| AC-6 | gates.sh xanh trên runner (có node/npm, BASE_REF giải được) | **manual** — run CI trên PR | — |
| AC-7 | WF — `[AC-7] <file>: khai permissions:` (6 file) | static | 1 PASS (cd-web), 5 FAIL |
| AC-7 | WF — `[AC-7] <file>: co contents: read` (6 file) | static | 1 PASS (cd-web), 5 FAIL |
| AC-7 | WF — `[AC-7] cd-web.yml: co packages: write va KHONG co quyen ghi nao khac` | static | PASS (không hồi quy) |
| AC-7 | WF — `[AC-7] <file>: khong co quyen ghi nao` (5 file ≠ cd-web) | static | PASS (vô nghĩa ở base: chưa khai gì — đi cặp với "khai permissions") |
| AC-7 | WF — `[AC-7] <file>: dung dorny/paths-filter => co pull-requests: read` (ci, trivy) | static | FAIL |
| AC-8 | WF — `[AC-8] {trivy.yml/trivy-gate, ci.yml/build}: if: cua job DUNG BANG always()` (K5) | static | FAIL |
| AC-8 | WF — `[AC-8] ...: if: cua job KHONG chua failure/cancelled/contains` (K5) | static | FAIL |
| AC-8 | WF — `[AC-8] ...: co step doc needs.*.result, in ::error va exit 1` | static | FAIL |
| AC-8 | WF — `[AC-8] ...: step gate xet ca 'failure' lan 'cancelled'` | static | FAIL |
| AC-8 | WF — `[AC-8] ...: step gate khong fail khi needs chi la success/skipped` | static | FAIL (chưa có step) |
| AC-8 | WF — `[AC-8] ci.yml/build: cac step build giu dieu kien theo filter go/web/python` (K5) | static | PASS (không hồi quy) |
| AC-8 | WF — `[AC-8] {trivy-gate, build}: needs: chua MOI job khac trong file` *(Challenge)* | static | PASS cả hai (không hồi quy: base đã `needs` đủ mọi job; ca này chặn việc **bớt** job khỏi `needs`, và bắt job **mới** như `gates-web` phải được thêm vào `build.needs`) |
| AC-8 | WF — `[AC-8] ...: step gate / job khong co continue-on-error` *(Challenge)* | static | FAIL (chưa có step gate) |
| AC-8 | WF — `[AC-8] ...: if: cua step gate bo trong hoac DUNG BANG always()` *(Challenge)* | static | FAIL (chưa có step gate) |
| AC-8 | WF — `[AC-8] ...: step gate so ket qua bieu thuc voi dung literal 'true' (hoac != 'false')` *(Challenge)* | static | FAIL (chưa có step gate) |
| AC-8 | Kết luận job là `failure` chứ không `skipped` khi một need fail | **manual** — cần một run có job trước fail. Ở `c848afa`, `trivy (ticketing)` và `trivy (waitingroom)` đỏ ở bước **build image** (B4, spec §1.4), không phải vì pgx — kết luận "đỏ vì pgx" của §1.3 đã bị rút lại. Sau AC-12 image build được và spec §3 dự báo Trivy sẽ đỏ thật (ticketing 3 CRITICAL, waitingroom 1) ⇒ run Trivy đầu tiên sau merge phải cho `trivy-gate` = **failure**, không phải `skipped` | — |
| AC-9 | WF — `[AC-9] moi NODE_VERSION khai trong workflow = 22` | static | FAIL |
| AC-9 | WF — `[AC-9] khong con node-version 20 / node:20` | static | FAIL |
| AC-9 | WF — `[AC-9] {ci.yml, e2e-nightly.yml}: moi actions/setup-node co node-version = 22` | static | FAIL |
| AC-10 | `actionlint` 1.7.7 trên toàn bộ workflow — lệnh (Git Bash, từ gốc repo): `MSYS_NO_PATHCONV=1 docker run --rm -v "$(cygpath -w "$PWD"):/repo" -w /repo rhysd/actionlint:1.7.7 -no-color` (trên Linux bỏ `MSYS_NO_PATHCONV=1` và `cygpath`) ⇒ **0 dòng output, exit 0**. Trên bản `git archive` không có `.git` phải truyền danh sách file `.github/workflows/*.yml` | command | FAIL — exit 1, 2 phát hiện: SC2129 `cd-web.yml:48`, SC2034 `e2e-nightly.yml:23` (khớp spec §1.2 m4) |
| AC-12 | WF — `[AC-12] go.work co khoi use (...) doc duoc` | static | PASS (go.work có sẵn — điều kiện để hai ca dưới có nghĩa) |
| AC-12 | WF — `[AC-12] deploy/docker/{ticketing,waitingroom}.Dockerfile: COPY go.work + go.mod (hoac ca thu muc) cua MOI module trong go.work` — danh sách module **đọc từ** `use (...)` của `go.work`, mỗi module phải tới đúng `<module>/go.mod` trong stage build (theo nguồn/đích của COPY) | static | FAIL (B4: thiếu 7 module) |
| AC-12 | `docker build -f deploy/docker/{ticketing,waitingroom}.Dockerfile .` exit 0 trên checkout sạch | **command / manual** — chạy được tại chỗ bằng Docker; trên GitHub là job `trivy` | — |
| AC-11 | WF — `[AC-11] K3: <file> con job '<name>'` (15 assertion, so theo `name:` của job) | static | PASS (không hồi quy) |
| AC-11 | WF — `[AC-11] cd-web: if: con ...event == 'push'` / `head_repository.full_name == github.repository` / `github.ref == 'refs/heads/main'` / `conclusion == 'success'` | static | PASS (không hồi quy) |
| AC-11 | WF — `[AC-11] cd-web: van chi kich hoat sau workflow 'CI' tren main` | static | PASS (không hồi quy) |
| AC-11 | WF — `[AC-11] khong workflow nao chay kubectl apply/... hay helm install/upgrade that` | static | PASS (không hồi quy) |
| AC-11 | WF — `[AC-11] chi cd-web.yml duoc push image` | static | PASS (không hồi quy) |
| AC-11 | Tên job vẫn khớp required check trong branch protection | **manual** — cấu hình repo, không nằm trong file (spec §3) | — |

## Giới hạn của kiểm tĩnh (cố ý ghi ra)

- Bộ đọc YAML là awk theo thụt lề, giả định block style. Một workflow hợp lệ viết job/step dạng flow
  `{...}` sẽ bị đọc sai ⇒ test fail giả (không pass giả).
- Comment cuối dòng chỉ được bỏ khi có khoảng trắng trước `#`. Một chuỗi shell chứa ` #` sẽ bị cắt —
  chỉ có thể làm *mất* bằng chứng (fail giả), không tạo ra bằng chứng.
- AC-5 mô phỏng glob của dorny bằng pattern bash (`**` ≈ `*`). Pattern phủ định `!…` bị bỏ qua.
- AC-8 chỉ nhận dạng `contains(needs.*.result, '<x>')`. Một cách viết khác đúng ngữ nghĩa (vd jq trên
  `toJSON(needs)`) sẽ bị báo FAIL — chọn chặt hơn là lỏng hơn.
- AC-8 "so literal": chỉ nhận so sánh `[ "$VAR" = "true" ]` / `==` / `!= "false"` với `VAR` là biến `env:`
  (của step hoặc job) mang biểu thức `needs.*.result`, hoặc biểu thức `${{ ... }}` nội suy thẳng. Không
  kiểm được `exit 1` có nằm đúng nhánh `then` hay không — đó là việc của lần chạy thật (manual).
- AC-8 `needs` đủ: quy tắc là `needs:` của gate chứa **mọi** job id khác trong cùng file.
- AC-2 "ref được push": thay `$VAR`/`${VAR}` bằng giá trị trong `env:` của step push (rồi của job), bỏ
  nháy và khoảng trắng trong `${{ }}`, rồi so chuỗi với `image-ref` và `tags:` của build-push-action. Biến
  được dựng trong shell (vd `ref="$IMAGE:$X"; docker push "$ref"`) sẽ không thay được ⇒ FAIL giả.
- AC-12: đọc COPY của **stage đầu** (stage build); bỏ `COPY --from=`; giả định mọi đích tương đối cùng
  một `WORKDIR`. Không xét `.dockerignore` có loại `go.mod` hay không — điều đó chỉ lộ khi `docker build`.
