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
| **AC-2b** hành vi SIGTERM | ⚠️ một phần | WP-A verify bằng `process.emit("SIGTERM")` in-process trên standalone server thật: readyz 503 suốt 5s, healthz giữ 200, exit code 0 sau 5005ms. Nhưng **Windows không có signal POSIX** (`child.kill('SIGTERM')` = `TerminateProcess`), nên giao hàng signal thật phải để P4.5 trên container |
| `docker build` | ❌ chưa từng chạy thành công lần nào | Docker daemon chưa bật |
| `NEXT_PUBLIC_API_BASE` | ⚠️ ARG đã có, **giá trị chưa truyền** | Cần người duyệt sửa CD |
| HPA scale theo tải | ❌ | Docker Desktop không có metrics-server ⇒ HPA sẽ hiện `<unknown>/60%`. Việc áp `minReplicas: 2` thì **không** cần metrics-server |
| Ingress định tuyến | ❌ | Không có ingress controller ⇒ dùng `kubectl port-forward` |
| `instrumentation.ts` coverage | **0%** | Test Design chỉ viết 3 file test, không file nào phủ nó. Muốn có test tự động cho AC-2b thì cần file test mới — quyết định của P1/Test Design, không phải của WP-A |

## 6. Việc còn lại

1. **P4.5** (hoãn sang hôm sau theo yêu cầu người dùng): bật Kubernetes Docker Desktop → `docker build`
   → `--dry-run=server` → apply → port-forward → SIGTERM → `rollout undo` → dán log vào mục 5 này.
2. **P5** review song song theo vai + Challenge.
3. **P6** verification độc lập read-only.
4. Ba việc cần người duyệt: giá trị `NEXT_PUBLIC_API_BASE` trong CD; paths-filter `deploy/**` +
   `.github/scripts/**` trong `ci.yml`; bước deploy trong `cd-web.yml`.
