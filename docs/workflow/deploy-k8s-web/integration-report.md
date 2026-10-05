# P4 — Integration report

Nhánh tích hợp: `feature/deploy-k8s-web`. Base contract: `8b869bb`.

> **Trạng thái: P4 xong, P4.5 CHƯA CHẠY.** Người dùng hoãn phần Docker/Kubernetes sang hôm sau.
> Nghĩa là **AC-4 và AC-8 chưa được verify** — xem mục 5. Đừng đọc tài liệu này như "đã deploy được".

---

## 1. Kiểm trước khi merge

Đúng quy tắc §7.4 (file ownership độc quyền) và §P4 (không tự "chọn bên" khi conflict).

```
$ git merge-base --is-ancestor 8b869bb <nhanh>
WP-A  OK  (2 commit)
WP-B  OK  (1 commit)
WP-C  OK  (1 commit)
```

Diff của từng WP **nằm gọn trong ownership**, không chồng lấn, không WP nào chạm file của WP khác:

| WP | File |
|---|---|
| WP-A | `src/app/api/healthz/route.ts`, `src/app/api/readyz/route.ts`, `src/instrumentation.ts`, `src/lib/readiness.ts` |
| WP-B | `deploy/k8s/{00-namespace,10-deployment,20-service,30-ingress,40-hpa}.yaml` |
| WP-C | `.github/scripts/gates.sh`, `Makefile`, `deploy/compose/docker-compose.yml` |

Merge `--no-ff` theo thứ tự A → B → C. **Không conflict nào.**

### ⚠️ Lỗi hạ tầng: cả ba worktree bị tạo từ base SAI

Cả ba agent báo độc lập rằng worktree của chúng nằm ở `fa8ca69` (merge commit của `main`), **không**
phải `8b869bb`. Ở base đó thì `docs/workflow/deploy-k8s-web/` và **ba file test đã đóng băng** không
tồn tại — WP-A sẽ phải hiện thực mù.

Cả ba tự phát hiện và `git reset --hard 8b869bb` (cây sạch, không mất gì) trước khi làm. WP-B còn chủ
động cảnh báo Integrator kiểm hai worktree còn lại.

Bài học cho lần sau: **không tin tham số base của công cụ tạo worktree**; bước đầu tiên của mỗi agent
WP phải là tự xác minh `git merge-base --is-ancestor <base> HEAD`.

## 2. Việc của Integrator đã làm

| Việc | Lý do |
|---|---|
| `ENV HOSTNAME=0.0.0.0`, `ENV PORT=3000` | `server.js` của standalone đọc `process.env.HOSTNAME`; Docker đặt biến đó thành container id ⇒ Next bind IP eth0, **không** bind loopback ⇒ healthcheck `127.0.0.1` nhận ECONNREFUSED và service `web` unhealthy vĩnh viễn (C3) |
| `ENV NEXT_MANUAL_SIG_HANDLE=1` | Next tự đăng ký SIGTERM, `server.close()` rồi `exit(0)` ngay ⇒ `/api/readyz` trả ECONNREFUSED chứ không phải 503, và nhánh 503 thành code chết trong vận hành (C1.1b) |
| `ARG NEXT_PUBLIC_API_BASE` + `ENV` ở stage build | Next **inline** biến `NEXT_PUBLIC_*` lúc build; đặt ở runtime hoàn toàn vô tác dụng (C7). Giá trị thật phải truyền `--build-arg` trong CD — **cần người duyệt** |
| Sửa `spec.md` AC-5 | Vẫn còn `--dry-run=client` + điều kiện "nếu `kubectl` có" — hai giả định C4.4 đã chứng minh sai. **Lần thứ ba sót khi sửa tài liệu nhiều nơi**; WP-C phát hiện |
| `.gitattributes` | `*.sh text eol=lf` — repo có `core.autocrlf=true` và không có `.gitattributes`; CRLF trong shebang làm Linux báo `bad interpreter` |
| `.gitignore` thêm `.claude/worktrees/` | Worktree tạm của agent, không thuộc mã nguồn |

`git update-index --chmod=+x` là **no-op**: WP-C đã commit `gates.sh` với mode `100755`.

## 3. Gates trên KẾT QUẢ ĐÃ MERGE

Đúng §P4 — *"branch xanh chưa chắc merge xanh"*. Chạy bằng chính script của WP-C, không chạy tay:

```
$ bash .github/scripts/gates.sh
[gates]   OK buoc 1/4 lint
[gates]   OK buoc 2/4 typecheck
[gates]   OK buoc 3/4 test:coverage
[gates]   OK buoc 4/4 build
[gates]   OK: khong co dong them nao chua .skip/.only/it.todo   (BASE_REF=8b869bb)
[gates]   OK: /api/healthz, /api/readyz khong nam trong prerender manifest
[gates] BO QUA validate k8s: khong co cluster (kubectl cluster-info that bai).
[gates] KHONG kiem duoc apiVersion, ten field, selector, hay duong dan probe.
[gates] TAT CA GATE DAT.
=== EXIT=0 ===
```

```
Test Files  43 passed (43)
     Tests  1067 passed (1067)
All files   95.22 stmts | 96.47 branch | 94 funcs | 95.22 lines
```

**Không hồi quy**: 1016 → 1067 (+51 test của WP-A). Không test nào bị sửa, xoá hay skip.

> **Các con số trong mục này là ảnh chụp tại commit tích hợp `60b4930`, không phải trạng thái hiện
> tại.** Sau P5 chúng đã đổi (44 file / 1086 test) — xem **mục 7**. Để nguyên ở đây vì mục 3 là biên
> bản của *lần chạy gate khi merge*, sửa số vào đây sẽ xoá mất dấu vết đó.

Output `next build` xác nhận hai probe là **`ƒ (Dynamic)`**, không phải `○ (Static)` — tức AC-1/AC-2
phần "không bị prerender" đạt trên bản build thật, không chỉ qua assert `export const dynamic`.

## 4. Ba phát hiện của WP agent — đáng đọc

### 4.1 WP-A: nhánh 503 bị **xoá khỏi bản production** trong khi mọi test vẫn xanh

Vòng 1 cả 4 gate đều xanh. Nhưng khi đọc `.next/server/`, WP-A phát hiện Next đóng gói
`instrumentation.ts` và **từng** route handler thành **bundle webpack riêng**, nên `lib/readiness.ts`
bị **nhân bản** vào mỗi bundle và một `let` ở tầm module trở thành **hai biến không liên quan**:

- Bundle `/api/readyz`: không ai **ghi** cờ ⇒ terser chứng minh được nó luôn `true` và inline ⇒ chunk
  có `{status:"ready"}` hardcode, **nhánh 503 bị xoá** (`grep -c 503` → `0`).
- Bundle `instrumentation.ts`: không ai **đọc** cờ ⇒ `setReady(false)` thành dead code, **cũng bị xoá**.

Tức production `/api/readyz` **luôn trả 200** — đúng chế độ lỗi mà C1.1b sinh ra để diệt. Ba test đóng
băng vẫn xanh suốt vì vitest không bundle, nó dùng đúng một module instance. **Lỗi chỉ tồn tại trong
bản bundle.**

Sửa: giữ cờ trên `globalThis` (một tiến trình có một `globalThis`; truy cập property của nó là side
effect không phân tích tĩnh được nên optimizer không fold/xoá). Chữ ký C1.2 không đổi, không sửa test.

### 4.2 WP-C: chính check chống-xanh-giả **là** một cái xanh giả

Check C4.3 sinh ra để chặn probe bị prerender. Nhưng trên Git Bash, MSYS2 **dịch tham số trông giống
đường dẫn POSIX thành đường dẫn Windows** trước khi gọi `node.exe`, nên `/api/healthz` tới node thành
`C:/Program Files/Git/api/healthz` ⇒ so khớp không bao giờ đúng ⇒ gate báo `TAT CA GATE DAT, exit 0`
trong khi probe **đang bị prerender thật**.

Chỉ lộ ra vì WP-C dựng fixture cho **đường fail**. Sửa bằng cách truyền qua biến môi trường dạng JSON
thay vì argv.

### 4.3 WP-B: không đặt `runAsUser`, có chủ ý

Chỉ `runAsNonRoot: true` + `allowPrivilegeEscalation: false` + `capabilities.drop: [ALL]`. Lý do: mục
đích của `securityContext` là để một lần sửa Dockerfile làm mất `USER node` thì phải **lộ ra** —
`runAsNonRoot` một mình sẽ làm container fail (`CreateContainerConfigError`). Thêm `runAsUser: 1000`
sẽ **che** hồi quy đó bằng cách âm thầm chạy đúng uid.

## 5. CHƯA VERIFY — không được tính là đạt

| AC | Trạng thái | Cần gì để đóng |
|---|---|---|
| **AC-4** manifest k8s | ❌ **chưa verify bằng bất cứ cách nào** | `kubectl apply --dry-run=server -f deploy/k8s/` sau khi bật Kubernetes. Nhánh "bỏ qua" của gate **không** đóng được AC này — nó in rõ là không kiểm gì |
| **AC-8** chạy thật | ❌ chưa chạy | P4.5: build image, apply, port-forward, SIGTERM → 503, `rollout undo` |
| **AC-2b** hành vi SIGTERM | ✅ **đã đóng ở mục 7** | WP-A verify bằng `process.emit("SIGTERM")` in-process trên standalone server thật: readyz 503 suốt 5s, healthz giữ 200, exit code 0 sau 5005ms. Nhưng **Windows không có signal POSIX** (`child.kill('SIGTERM')` = `TerminateProcess`), nên giao hàng signal thật phải để P4.5 trên container — **đã làm**, xem mục 7.1 |
| `docker build` | ✅ **đã đóng ở mục 7** | Docker daemon chưa bật ở thời điểm merge |
| `NEXT_PUBLIC_API_BASE` | ⚠️ ARG đã có, **giá trị chưa truyền** | Cần người duyệt sửa CD |
| HPA scale theo tải | ❌ | Docker Desktop không có metrics-server ⇒ HPA sẽ hiện `<unknown>/60%`. Việc áp `minReplicas: 2` thì **không** cần metrics-server |
| Ingress định tuyến | ❌ | Không có ingress controller ⇒ dùng `kubectl port-forward` |
| `instrumentation.ts` coverage | ✅ **đã đóng ở mục 7** | Lúc merge là 0%: Test Design chỉ viết 3 file test, không file nào phủ nó. `877c4c7` đã thêm file test — nay **100%** |
| **AC-6** runbook | ✅ **đã đóng ở mục 7** | `runbook.md` **không tồn tại** ở thời điểm merge, và mục 5 bản đầu **không** liệt kê nó là thiếu — tức báo cáo tự bỏ qua một AC chưa đạt. Phát hiện ở P5 Challenge (F7) |

## 6. Việc còn lại

1. ~~**P4.5**~~ — một phần đã làm, xem mục 7. Phần còn lại cần bật Kubernetes Docker Desktop:
   `--dry-run=server` → apply → kiểm pod Ready → `rollout undo`.
2. ~~**P5** review song song theo vai + Challenge~~ — đã chạy, các phát hiện đã áp ở mục 7.
3. **P6** verification độc lập read-only.
4. Ba việc cần người duyệt: giá trị `NEXT_PUBLIC_API_BASE` trong CD; paths-filter `deploy/**` +
   `.github/scripts/**` trong `ci.yml`; bước deploy trong `cd-web.yml`.
5. `readinessProbe.failureThreshold: 1` + 2 replica + không có PodDisruptionBudget ⇒ có thể rơi về
   **0 endpoint**. Chạm contract đã đóng băng (C2.2) nên không sửa trong slice này; đã ghi vào
   `runbook.md` §6.5 và cần quay lại ở P1 của slice sau.

## 7. Cập nhật sau P4.5 (một phần) và P5

### 7.1 Đã verify bằng container thật — AC-2b đóng bằng signal POSIX

Docker đã chạy được. `docker build -f deploy/docker/web.Dockerfile .` **thành công lần đầu tiên**
trong lịch sử repo (trước đó `COPY --from=build /app/public ./public` trỏ vào thư mục không tồn tại).

Kiểm trong container Linux, bằng signal POSIX thật (`docker kill -s`), không phải `process.emit`:

| Quan sát | Kết quả |
|---|---|
| `/api/readyz` sau SIGTERM | **503 sau 77 ms** |
| `/api/healthz` suốt cửa sổ drain | **giữ 200** (liveness không được sập, nếu không kubelet restart pod giữa lúc drain) |
| Thoát tiến trình | **ExitCode 0** ở t=5129 ms |
| SIGTERM thứ hai ở t=2s, chạy dưới `--init` | **không** cắt ngắn drain |
| SIGINT (sau khi sửa F6) | readyz 503 trong ~1s, ExitCode 0 |
| `Config.User` của image | `1000` — dạng số; `docker exec … process.getuid()` ⇒ `1000` |

### 7.2 Phát hiện của P5 đã áp

| # | Vấn đề | Sửa |
|---|---|---|
| S1 | `USER node` ⇒ kubelet từ chối container (`runAsNonRoot` + user không phải dạng số) — pod fail **100%**, không phải chỉ khi hồi quy | `USER 1000` (`6264c60`) |
| F6 | `NEXT_MANUAL_SIG_HANDLE=1` tắt handler của Next cho **cả** SIGINT, mà chỉ SIGTERM được đăng ký lại ⇒ Ctrl+C mất graceful shutdown, exit 130 | đăng ký SIGINT (`bec9665`) |
| — | Ô `globalThis` giữ cờ ready ai cũng ghi được; chỉ kiểm tồn tại thì nhận cả chuỗi/object rỗng ⇒ `isReady()` trả non-boolean ⇒ readyz 503 vĩnh viễn ⇒ rollout treo mãi vì `maxUnavailable: 0` | kiểm **hình dạng** + `=== true` (`bec9665`) |
| F10 | Comment nói cửa sổ drain là 5s, thực tế ~10s (preStop và DRAIN_MS **nối tiếp**). Hạ grace period theo comment đó thì ExitCode thành 137 | sửa comment (`9e1f478`) |
| — | Tag image là `sha-8b869bb` = commit contract, **không có** route.ts/readiness.ts/instrumentation.ts ⇒ tag nói sai nguồn gốc image | `sha-60b4930` (`9e1f478`) |
| C7-B4 | `environment:` trong compose vô tác dụng với `NEXT_PUBLIC_*` | `build.args` (`b0ae290`) |
| F11 | `gates.sh` còn CRLF trong index dù `.gitattributes` khai `eol=lf` ⇒ `bad interpreter` khi chạy trên Linux | `git ls-files --eol` nay cho `i/lf` |
| — | Check C4.2 chỉ soi `**/*.test.*` và chỉ so với BASE_REF ⇒ `.spec.*`, file test ở cấp gốc, `.skipIf/.runIf`, và marker chưa commit đều **qua gate** | mở rộng glob + regex + thêm tầng diff với working tree (`bb4facb`) |
| F7 | `runbook.md` không tồn tại ⇒ **AC-6 chưa đạt**, mà mục 5 lại không liệt kê | đã viết `runbook.md` |
| F8 | Số trong mục 3/5 lạc hậu | mục này |

### 7.3 Số liệu hiện tại

```
Test Files  44 passed (44)
     Tests  1086 passed (1086)
All files   95.42 stmts | 96.49 branch | 94.58 funcs | 95.42 lines
```

Per-file, đọc từ `coverage/lcov.info`:

| File | lines | funcs | branch |
|---|---|---|---|
| `src/instrumentation.ts` | 16/16 | 2/2 | 7/7 |
| `src/lib/readiness.ts` | 22/22 | 4/4 | 10/10 |
| `src/app/api/healthz/route.ts` | 10/10 | 1/1 | 1/1 |
| `src/app/api/readyz/route.ts` | 16/16 | 1/1 | 5/5 |

**Con số `All files` bị thổi phồng** và không được dùng làm bằng chứng: `lcov.info` có cả
`src/instrumentation.test.ts`, `src/lib/readiness.test.ts`… tức `coverage.include` của
`vitest.config.ts` đếm **file test như file nguồn**. File test gần như luôn tự phủ 100% chính nó nên
kéo tổng lên. Đây là ticket riêng (sửa `vitest.config.ts` chạm file của WP-C ⇒ ngoài slice này).

### 7.4 Vẫn chưa verify

`gates.sh` vẫn in `BO QUA validate k8s: khong co cluster` — `kubectl config current-context` cho
`current-context is not set`, tức **Kubernetes chưa bật** trong Docker Desktop. Toàn bộ phần apply /
pod Ready / HPA / Ingress / rolling update vẫn **chưa ai chạy**; chi tiết trong `runbook.md` §6.2.

Một khoảng trống gate còn mở (P5 Challenge, F1): §4.1 là hồi quy ở **tầng bundle**, mà nó tái hiện
được với cả 4 gate xanh **và** C4.2, C4.3 xanh. Chưa có check nào đọc `.next/server/**`.
