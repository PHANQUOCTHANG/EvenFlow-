# Evidence — Verification độc lập (bước 9)

Do một agent chỉ đọc, context sạch, tự chạy lại mọi thứ. Output thô rút gọn. Worktree
`fix/ci-cd-review`, base `c848afa`, chưa commit.

## Vòng 1 (2026-10-08) — KHÔNG ĐẠT

### actionlint (AC-10)

```
$ MSYS_NO_PATHCONV=1 docker run --rm -v "<worktree>:/repo:ro" -w /repo rhysd/actionlint:1.7.7 -color=false
ACTIONLINT_EXIT=0          (0 dong output; -verbose: "Collected 6 YAML files", "Found total 0 errors")
Doi chung tren ban export c848afa:
.github/workflows/cd-web.yml:48:9: shellcheck reported issue in this script: SC2129:style:5:1 ...
.github/workflows/e2e-nightly.yml:23:9: shellcheck reported issue in this script: SC2034:warning:1:1: i appears unused ...
BASE_ACTIONLINT_EXIT=1
```

### Hai script test

```
Git Bash Windows (khong co make):
  test-run-and-report-tests.sh: 35 PASS, 0 FAIL   exit 0
  test-workflows.sh:            128 PASS, 0 FAIL  exit 0
ubuntu:24.04, mawk, KHONG co make:   35/0, 128/0
ubuntu:24.04 + apt install make (GNU Make 4.3):
  WF_EXIT=1
  FAIL  [AC-4] Makefile: khong truyen TESTFLAGS => lenh y het hien tai (K2, khong hoi quy)
        'go test -tags=integration -run TestNoOversell \
        ./services/ticketing/test/concurrency/... -count=200 -timeout=30m'
  FAIL  [AC-4] Makefile: TESTFLAGS=-v => them dung token -v, phan con lai giu nguyen (K2)
  make -s -n test-oversell TESTFLAGS=-v | cat -A:
    go test -v -tags=integration -run TestNoOversell \$
    ^I./services/ticketing/test/concurrency/... -count=200 -timeout=30m$
  Base c848afa + make: 37 PASS, 91 FAIL
```

⇒ **Blocker**: bộ test chỉ xanh khi không có `make`; runner `ubuntu-latest` có `make` ⇒ `lint-ci` đỏ.

### Baseline `c848afa` (Git Bash)

```
test-run-and-report-tests.sh <tmp>: 0 PASS, 35 FAIL
test-workflows.sh <tmp>:            38 PASS, 90 FAIL
FAIL theo AC: AC-1 4 | AC-2 18 | AC-3 5 | AC-4 6 | AC-5 17 | AC-6 6 | AC-7 12 | AC-8 16 | AC-9 4 | AC-12 2
```

### AC-12 — build image Go (bản export sạch `c848afa`, `--no-cache`)

```
Dockerfile da sua:  FIXED_ticketing_EXIT=0     FIXED_waitingroom_EXIT=0
Dockerfile goc:     ORIG_ticketing_EXIT=1
  go: cannot load module ../gateway listed in go.work file: open ../gateway/go.mod: no such file or directory
                    ORIG_waitingroom_EXIT=1
```

### AC-4 — chạy thật trong `golang:1.23-alpine` (go1.23.12)

```
$ bash .github/scripts/run-and-report-tests.sh TestNoOversell make test-oversell TESTFLAGS=-v
=== RUN   TestNoOversell_UnderExtremeConcurrency
    oversell_test.go:37: TODO(EVF-39): can hien thuc testcontainers truoc khi bat test nay
--- SKIP: TestNoOversell_UnderExtremeConcurrency (0.00s)
...
::warning title=TestNoOversell KHONG kiem gi::0 lan PASS, 0 lan FAIL, 400 lan SKIP. ...
WRAPPER_EXIT=0   so dong ::warning = 1
summary: | 0 | 0 | 400 | 0 |  + "**Gate nay KHONG kiem gi:** 0 lan PASS (400 lan SKIP)."

Dang lenh cua integration.yml, them tam test TestNoOversell_VerifierFake (da xoa):
  fail: WRAPPER_EXIT=1, 0 dong ::warning
  pass: WRAPPER_EXIT=0, 0 dong ::warning
```

### `gates.sh`

```
$ BASE_REF=c848afa bash .github/scripts/gates.sh
OK buoc 1/4 lint | 2/4 typecheck | 3/4 test:coverage | 4/4 build
OK: khong co dong them nao chua .skip/.only/it.todo
OK: /api/healthz, /api/readyz khong nam trong prerender manifest
BO QUA validate k8s: khong co cluster
OK: co readiness co trong ca hai bundle, nhanh 503 con nguyen, healthz khong co 503
TAT CA GATE DAT.   GATES_RUN1_EXIT=0
kubeconform v0.8.0 -strict: Valid 5, Invalid 0 | compose config -q: exit 0 | golangci-lint v2.12.2 config verify: exit 0
```

### Pin SHA `cd-web.yml`

```
docker/setup-buildx-action  8d2750c68a42422c14e847fe6c8ac0403b4cbd6f = refs/tags/v3 = v3.12.0
docker/build-push-action    10e90e3645eae34f1e60eeb005ba3a3d33f178e8 = refs/tags/v6 = v6.19.2
docker/login-action         c94ce9fb468520275223c153574b00df6fe4bcc9 = refs/tags/v3 = v3.7.0
aquasecurity/trivy-action   ed142fd0673e97e23eac54620cfb913e5ce36c25 = refs/tags/v0.36.0^{}
Khoi `if:` chong fork cua CD: diff rong so voi c848afa. `packages: write` chi o cd-web.yml.
```

### Phạm vi

Chỉ file thuộc plan §1 mục 1–10 + `docs/workflow/ci-cd-review/`.

### Bảng AC

| AC | Trạng thái |
|---|---|
| AC-4, AC-10, AC-12 | **đạt, bằng chứng chạy thật** |
| AC-1, 2, 3, 5, 6, 8, 11 | kiểm tĩnh đạt; **hành vi chỉ kiểm được trên GitHub** — không tính là đạt hành vi |
| AC-7, AC-9 | kiểm tĩnh đạt |
| Bộ test (plan §1 mục 9) | **không đạt trên Linux có make** |

13 mâu thuẫn tài liệu — xử lý ghi ở `review-log.md` § Verification.

## Vòng 2 (2026-10-08) — ĐẠT

Phạm vi hẹp: bản sửa `norm_cmd` của Test Design cho nhánh `make -n`, và 13 mâu thuẫn tài liệu.

```
ubuntu:24.04, mawk, KHONG co make
  worktree (ban LF)        RRT 35 PASS 0 FAIL | WF 128 PASS 0 FAIL
  base c848afa             RRT 0 PASS 35 FAIL | WF 38 PASS 90 FAIL
  dot bien bo -count=200   WF 126 PASS 2 FAIL  (dung 2 ca K2)
ubuntu:24.04 + GNU Make 4.3
  worktree (ban LF)        RRT 35/0 | WF 128/0
  base c848afa             RRT 0/35 | WF 38/90
  dot bien bo -count=200   WF 126/2 (dung 2 ca K2; chuoi da noi dong, khong con '\' + xuong dong)
  dot bien bo -tags=integration  WF 126/2
Git Bash (worktree CRLF, khong make)
  worktree 35/0 + 128/0 | dot bien 126/2 | base 0/35 + 38/90
```

13/13 mâu thuẫn vòng 1: đã xử lý. 3 lỗi chữ mới (C2-1 spec B2 nói quá bằng chứng; C2-2 plan mục 10
còn tả cách copy cả `services/`; C2-3 baseline nói "ba" thay vì "hai" assertion K2 đi nhánh make/awk):
đã sửa sau vòng 2. Phạm vi file không đổi.

Integrator tự kiểm thêm, độc lập với hai agent: cùng harness Linux có make, đột biến bỏ `-count=200`
⇒ 126 PASS / 2 FAIL đúng hai ca K2.
