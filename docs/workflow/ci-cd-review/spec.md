# Spec — rà soát và sửa CI/CD

**Yêu cầu (người dùng, 2026-10-06):** "có thể sửa lại code CI/CD nhé vì trưởng nhóm có thể làm sai."
Người dùng **cho phép sửa CI** — nới quy tắc "agent không sửa CI" của workflow cho riêng task này.

**Tier: Standard**, có thêm review bảo mật ở bước Code Review vì `cd-web.yml` giữ `packages: write`.
**Không** thêm bước deploy lên môi trường thật nào (spec deploy §6.1 — vẫn cần người duyệt).

**Base:** `origin/develop` @ `c848afa`. Nhánh `fix/ci-cd-review`, worktree riêng.

## 1. Bằng chứng đã thu (trước khi viết AC)

### 1.1 Lịch sử chạy thật trên GitHub (public API, 40 run gần nhất)

| Workflow | Nhánh | Commit | Kết quả |
|---|---|---|---|
| Trivy scan | main | `779b73b`, `b4696f8`, `c848afa` | **failure** cả 3 |
| CD web | main | `779b73b`, `b4696f8`, `c848afa` | **success** cả 3 — đã publish lên GHCR |
| Trivy scan | PR `feature/deploy-k8s-web` | `96091aa` | **failure** — PR vẫn được merge (PR #26) |
| E2E nightly | main | `b4696f8` ×3, `c848afa` | **failure** mỗi đêm |
| Oversell Gate (EVF-39) | main + PR | mọi commit | **success** |

Chi tiết từng job/step **chưa lấy được ở lần đầu (2026-10-06)**: API không xác thực bị giới hạn 60
lần/giờ và đã hết. Không dùng credential của người dùng để lách. **Đã lấy được ngày 2026-10-08 — xem
§1.4.**

*(Thứ tự mục: §1.2 "Đọc mã" đứng sau §1.5 vì §1.3–§1.5 được chèn thêm khi có bằng chứng mới; giữ số
mục cũ để các chỗ trích dẫn khác không lệch.)*

### 1.3 Trivy đỏ vì đâu — quét lockfile tại chỗ

Không lấy được log job, nên quét thẳng lockfile của `c848afa` bằng `aquasec/trivy:0.66.0 fs
--severity CRITICAL,HIGH --ignore-unfixed` (cùng chính sách `ignore-unfixed` với `trivy.yml`):

| Target | CRITICAL | HIGH |
|---|---|---|
| `services/ticketing/go.mod` | **2** — `github.com/jackc/pgx/v5` v5.7.1: CVE-2026-33815, CVE-2026-33816 (sửa ở 5.9.0) | 0 |
| `apps/web/package-lock.json` | **0** | 3 — `postcss` 8.4.31 (2 CVE), `source-map-js` 1.2.1 |
| 9 module Go còn lại, `package-lock.json` gốc | 0 | 0 |

~~Kết luận có giới hạn: matrix `ticketing` của `trivy.yml` gần như chắc chắn là job đỏ vì pgx.~~
**Kết luận đó SAI — xem §1.4.** Bảng trên vẫn đúng (pgx có CRITICAL thật), nhưng pgx **chưa bao giờ**
là lý do Trivy đỏ. Điều vẫn đúng: lockfile web sạch CRITICAL, nên Trivy đỏ trên `main` không chứng minh
CD đã publish một image *web* có CRITICAL; lỗ hổng thật của B2 là CD **không bao giờ quét chính image
nó publish**.

### 1.4 Trivy đỏ vì đâu — log job thật (2026-10-08, sau khi API reset)

`GET /actions/runs/37262131238/jobs` (Trivy, `main` @ `c848afa`):

| Job | Kết luận | Step đỏ |
|---|---|---|
| `trivy (web, …)` | success | — |
| `trivy (ai-worker, …)` | success | — |
| `trivy (waitingroom, …)` | **failure** | **`build image waitingroom`** |
| `trivy (ticketing, …)` | **failure** | **`build image ticketing`** |
| `trivy-gate` | **skipped** | — ⇒ báo Success (B3, có bằng chứng thật) |

Tức hai image Go **không build được**, Trivy chưa từng được quét tới. Log chi tiết cần quyền admin
(`403 Must have admin rights`), nên tái hiện tại chỗ từ bản export LF sạch của `origin/main`
(`4050da1`). Kết quả áp được cho base `c848afa` của nhánh: `git diff --stat c848afa 4050da1` không chạm
`deploy/docker/`, `go.work` hay `.github/workflows/` (Verification độc lập đã tái hiện lại trên
`c848afa`, cùng output):

```
go: cannot load module ../gateway listed in go.work file: open ../gateway/go.mod: no such file or directory
go: cannot load module ../identity listed in go.work file: ...
```

Thêm: `E2E nightly` @ `4050da1` đỏ ở step **`make up`** — nhất quán với việc compose phải build hai
image này. Chưa tái hiện riêng E2E.

Cũng từ API: `CI` @ `4050da1` (commit **sửa `.github/scripts/gates.sh`**) — `detect-changes` success,
**mọi job khác skipped**, `build` success. Bằng chứng thật cho M2 (CI không chạy gì) + B3 (vẫn xanh).

### 1.5 Khi build được thì sao — quét image đã sửa tại chỗ

Dockerfile đã sửa build xanh cả hai. Quét bằng đúng chính sách `trivy.yml` (`aquasec/trivy:0.66.0
image --severity CRITICAL --ignore-unfixed`):

| Image | CRITICAL | Sửa ở |
|---|---|---|
| ticketing | **3** — `pgx/v5` v5.7.1 (CVE-2026-33815, CVE-2026-33816) + stdlib | pgx ≥ 5.9.0 + Go mới hơn |
| waitingroom | **1** — `stdlib` Go **1.23.12**, CVE-2025-68121 (`crypto/tls`) | Go ≥ 1.24.13 / 1.25.7 |

Lớp OS (`distroless/static-debian12`): 0. ⇒ Sau khi sửa build, Trivy sẽ đỏ **vì lỗ hổng thật** — đúng
việc của nó. Sửa cần **nâng Go khỏi 1.23** (đang pin ở mọi `go.mod`, `go.work`, Dockerfile, `ci.yml`)
và nâng pgx: quyết định của nhóm, ngoài task CI.

### 1.2 Đọc mã

| # | Phát hiện | Bằng chứng |
|---|---|---|
| B1 | `lint-go` không thể xanh | `.golangci.yml:1` là `version: "2"` (định dạng golangci-lint v2), nhưng `ci.yml:72-86` dùng `golangci/golangci-lint-action@v6` + `version: latest`. Action v6 từ chối golangci-lint v2; golangci-lint v1 không đọc được config v2. Không có tổ hợp nào chạy được. Chưa thấy nó fail trên GitHub chỉ vì `lint-go` chỉ chạy khi mã Go đổi |
| B2 | CD không quét image nó publish | `cd-web.yml` chỉ chờ workflow `CI`; Trivy là workflow riêng và không chặn CD. Bảng 1.1: CD publish 3 lần trong khi Trivy cùng commit đỏ. Job đỏ là `waitingroom` và `ticketing`, ở bước **build** chứ không phải bước quét (§1.4, B4); `trivy (web)` xanh **ở `c848afa`** — chỉ commit đó có log job; với `779b73b` và `b4696f8` (CD cũng publish) chưa xem job `trivy (web)`, nên **không** khẳng định được image web nào cũng sạch. Dù vậy nếu có thì cũng không có gì chặn |
| M1 | Oversell gate pass mà không kiểm gì | `services/ticketing/test/concurrency/helpers.go:33` `t.Skip("TODO(EVF-39)…")` trong `setupEnv` (output `go test -v` in vị trí `oversell_test.go:37` vì `setupEnv` gọi `t.Helper()`). Chạy thật: **400 SKIP, 0 PASS** cho `-count=200`. `go test` coi skip là thành công ⇒ "TestNoOversell ×200" và smoke ×20 xanh với **0** lần chạy thật. Đây là bất biến số một (BR-O1) |
| M2 | CI không chạy gì cho Dockerfile / script gate / manifest / chính nó | `ci.yml:47-48` filter `web` chỉ có `apps/web/**`. Sửa `deploy/docker/web.Dockerfile`, `.github/scripts/gates.sh`, `deploy/k8s/**`, `deploy/compose/**`, hay `ci.yml` ⇒ không job nào chạy |
| M3 | Gate C4.2/C4.3/C4.5 không bao giờ chạy trên CI | `gates.sh` chỉ chạy tay. C4.5 là gate duy nhất bắt được nhánh 503 bị xoá khỏi bundle (integration-report §7.5) |
| M4 | Image publish thiếu `NEXT_PUBLIC_API_BASE` | `cd-web.yml:71-82` không có `build-args` ⇒ `API_BASE = ""` (contracts.md C7-B4) |
| B4 | Image Go **chưa từng build được** | *(Phát hiện 2026-10-08 từ log job thật, §1.4.)* `deploy/docker/{ticketing,waitingroom}.Dockerfile` copy `go.work` (9 module) nhưng chỉ copy 2 module ⇒ `go: cannot load module ../gateway listed in go.work file`. Là nguyên nhân Trivy đỏ trên `main`; nhiều khả năng cũng là nguyên nhân E2E đỏ ở `make up` |
| B3 | Job "gate" bị **skip** được GitHub tính là **Success** | *(Phát hiện ở Plan Review vòng 1.)* Tài liệu GitHub: *"A job that is skipped will report its status as 'Success'. It will not prevent a pull request from merging, even if it is a required check."* `trivy.yml:70` (`trivy-gate`) và `ci.yml:197` (`build`) đặt điều kiện "không có job nào fail" vào `if:` của job ⇒ khi có job fail, gate bị **skip** ⇒ báo Success. Là cách giải thích hợp lý nhất cho việc PR #26 merge được khi Trivy đỏ (nếu `trivy-gate` là required check — chưa kiểm được branch protection) |
| m1 | Không khai `permissions:` | `ci.yml`, `trivy.yml`, `integration.yml`, `oversell-gate.yml`, `e2e-nightly.yml` dùng quyền mặc định của `GITHUB_TOKEN` theo cấu hình repo — có thể là write-all |
| m2 | `trivy-gate` xanh khi `detect-changes` hỏng | `trivy.yml:70` chỉ xét `needs.scan.result`. `changes` fail ⇒ `scan` skip ⇒ gate in "khong co CRITICAL" |
| m3 | CI test trên Node khác runtime | `ci.yml:20` `NODE_VERSION: "20"`, `e2e-nightly.yml:31` `"20"`, còn image chạy `node:22-alpine` |
| m4 | 2 cảnh báo shellcheck | actionlint baseline: SC2129 `cd-web.yml:48`, SC2034 `e2e-nightly.yml:23` |

## 2. Acceptance criteria

**AC-1 — `lint-go` chạy được.** *Given* `.golangci.yml` định dạng v2, *when* `lint-go` chạy, *then*
action và golangci-lint là cặp tương thích v2, và golangci-lint **pin phiên bản cụ thể** (không `latest`
— `latest` chính là thứ đã đổi ngầm từ v1 sang v2 và làm hỏng job).

**AC-2 — CD không publish image có CRITICAL.** *Given* push lên `main`, *when* CD chạy, *then* image
được build, **quét Trivy với đúng chính sách của `trivy.yml`** (`CRITICAL`, `ignore-unfixed`), và chỉ
**push nếu quét sạch**. Quét fail ⇒ không tag nào được push, kể cả `:latest`. Image được push phải là
**chính image đã quét** (`docker push`), không build lại. Mọi action bên thứ ba trong `cd-web.yml` —
job duy nhất giữ `packages: write` — pin bằng **SHA commit đầy đủ**, không bằng tag (tag ghi đè được;
Plan Review dẫn sự cố tag của `trivy-action` bị force-push tháng 3/2026, CVE-2026-33634 — nguồn do
reviewer đưa, chưa kiểm độc lập; pin SHA là đúng bất kể sự cố đó).

**AC-3 — CD truyền API base, và không im lặng khi thiếu.** *Given* repo variable
`NEXT_PUBLIC_API_BASE`, *when* CD build, *then* giá trị được truyền qua `build-args`. Biến chưa đặt ⇒
**annotation `::warning::`** + một dòng trong job summary nói rõ image gọi URL tương đối.

**AC-4 — Oversell gate không được xanh câm.** *Given* `TestNoOversell*` bị skip toàn bộ, *when*
oversell gate hoặc smoke trong integration chạy, *then* workflow phát **`::warning::`** + summary ghi
"0 lần PASS, N lần SKIP — gate này KHÔNG kiểm gì". *Given* có ≥1 lần FAIL, *then* job vẫn **fail**
(exit code của `go test` không bị nuốt). *Given* có PASS, *then* summary ghi số PASS.

**AC-5 — CI chạy khi thứ nó bảo vệ thay đổi.** Đổi `deploy/docker/web.Dockerfile`,
`.github/scripts/gates.sh`, hay `.github/workflows/ci.yml` ⇒ các job web chạy. Đổi `deploy/k8s/**` hay
`deploy/compose/**` ⇒ một job validate manifest chạy: `kubeconform -strict` + `docker compose config`.
Đổi `ci.yml` ⇒ **cả** job Go và Python cũng chạy (nếu không, chính PR sửa `lint-go` sẽ không chạy
`lint-go` lần nào trước khi merge — đúng kiểu lỗi đã che B1). Đổi `trivy.yml` ⇒ Trivy quét.

**AC-6 — Gate C4.2/C4.3/C4.5 chạy trên CI.** Job chạy `bash .github/scripts/gates.sh` với lịch sử git
đầy đủ và `BASE_REF` = base của PR (pull_request) hoặc commit trước (push).

**AC-7 — Quyền tối thiểu.** Mọi workflow khai `permissions:`. Workflow chỉ đọc ⇒ `contents: read` (+
`pull-requests: read` nơi có `dorny/paths-filter`, vì nó gọi API liệt kê file của PR). Chỉ `cd-web.yml`
có `packages: write`.

**AC-8 — Job gate không được xanh nhờ bị skip (B3).** Áp dụng cho `trivy-gate` (`trivy.yml`) và
`build` (`ci.yml`). *Given* bất kỳ job nào trong `needs` là `failure` hoặc `cancelled`, *then* job gate
**chạy** và **fail** (kết luận `failure`, không phải `skipped`). Cách duy nhất làm được điều đó: `if:`
của job là `always()`, và **một step** `exit 1` khi phát hiện `failure`/`cancelled`. Điều kiện đó
**không** được nằm trong `if:` của job. *Given* mọi job trong `needs` là `success` hoặc `skipped`,
*then* gate xanh.

**AC-9 — CI chạy cùng Node với runtime.** `NODE_VERSION` = `22` ở mọi workflow dùng Node.

**AC-10 — actionlint sạch.** `actionlint` 1.7.7 trên toàn bộ `.github/workflows/` ⇒ 0 phát hiện.

**AC-11 — Không đổi hành vi ngoài phạm vi.** Không thêm bước deploy; CD vẫn chỉ chạy từ `main` với
đủ ba điều kiện chống fork hiện có; không đổi tên job nào đang có (có thể đang là required check
trong branch protection).

**AC-12 — Image Go build được (B4).** *(Thêm 2026-10-08, sau Plan Review vòng 2 — mở rộng phạm vi,
phải đi qua Code Review.)* *Given* checkout sạch, *when* `docker build -f
deploy/docker/{ticketing,waitingroom}.Dockerfile .`, *then* exit 0. Không né bằng `GOWORK=off` (repo
không track `go.sum` ⇒ `missing go.sum entry`, đã thử).

## 3. Ngoài phạm vi — cần người quyết

- **Nâng Go khỏi 1.23 và nâng pgx.** Sau AC-12, Trivy quét được và sẽ **đỏ thật**: ticketing 3
  CRITICAL, waitingroom 1 CRITICAL (§1.5). CVE stdlib chỉ sửa ở Go ≥ 1.24.13. Đổi phiên bản Go chạm mọi
  `go.mod`, `go.work`, Dockerfile Go và `ci.yml` ⇒ quyết định của nhóm.
- **Repo không track `go.sum`.** Build vẫn chạy (workspace mode đối chiếu sum.golang.org lúc build)
  nhưng checksum không được pin trong repo.
- **`develop` chậm hơn `main` 6 commit**: PR #27 và #28 merge thẳng vào `main`, bỏ qua `develop`.

- **Biến oversell-skip thành fail cứng.** Đúng tinh thần "fail ⇒ không được merge vào main", nhưng sẽ
  chặn mọi PR vào `main` cho tới khi EVF-39 được hiện thực. Quyết định của trưởng nhóm; spec này chỉ
  làm cho nó **không còn câm** (AC-4).
- **Branch protection** (Trivy có phải required check không — PR #26 merge khi Trivy đỏ): cấu hình
  repo, không nằm trong file.
- **E2E nightly đỏ mỗi đêm:** đỏ ở step **`make up`** (§1.4). Nhiều khả năng do compose build hai
  image Go hỏng (B4), mà AC-12 đã sửa — nhưng **chưa tái hiện riêng E2E**, nên không khẳng định nó sẽ
  xanh. Không sửa thêm gì cho E2E.
- **Giá trị thật của `NEXT_PUBLIC_API_BASE`:** phụ thuộc gateway được đặt ở đâu — người đặt repo
  variable quyết.
- **Nợ có sẵn mà các job Go/Python sẽ lộ ra** khi được chạy (và vì `ci.yml` nằm trong filter của
  chúng, **chính PR này** sẽ chạy chúng):
  - `lint-go` — 10 lỗi (golangci-lint v2.12.2, bản export LF của `c848afa`): `libs/go` 0;
    `services/ticketing` 7 (5 gofmt, 2 errcheck); `services/waitingroom` 3 (1 gofmt, 2 errcheck)
  - `test-go` — unit test xanh cả 9 module, nhưng cổng coverage domain/app ≥ 70% **fail**:
    `ticketing` và `waitingroom` có `internal/domain` + `internal/app` mà **không có file `_test.go`
    nào** ⇒ 0.0% (Plan Review vòng 2 đo trong `golang:1.23`)
  - `lint-python` — ruff mới nhất 6 lỗi; `pyproject.toml` ghi `ruff>=0.7` và **không có `uv.lock`** ⇒
    CI cài bản ruff mới nhất, số lỗi trôi theo thời gian
  - ⇒ `build` fail theo K5 (trước đây nó bị skip nên báo Success — chính lỗi B3)

  Là mã/cấu hình backend ⇒ không sửa trong task CI. **Thứ tự merge là quyết định của người dùng /
  trưởng nhóm** (leo thang ở Plan Review vòng 2, N1). Không dùng `continue-on-error` để né: đó đúng là
  kiểu "xanh câm" task này đang dẹp.
- **Mất provenance attestation** khi chuyển từ `push: true` sang `load` → Trivy → `docker push`. Đánh
  đổi có chủ ý: bảo đảm image được push đúng là image đã quét quan trọng hơn; không AC nào đòi
  provenance.
- **Force-push lên `main`/`develop`** làm `github.event.before` không còn trong lịch sử ⇒ `gates-web`
  fail ⇒ CI đỏ ⇒ CD không publish. Job thử `git fetch` commit đó trước; vẫn hỏng thì `::error::` nêu
  rõ nguyên nhân. Chấp nhận: force-push lên nhánh chung vốn là sự cố cần người xử lý.
