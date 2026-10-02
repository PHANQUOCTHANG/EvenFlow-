# Contract đóng băng — slice deploy (bản 2, sau P2)

> **Bản 1 bị P2 kết luận KHÔNG ĐẠT** với 5 blocker + 9 major. Bản này sửa toàn bộ. Những chỗ sửa
> đều ghi rõ lý do, vì phần lớn là **giả định sai đã được kiểm chứng bằng lệnh thật**, không phải
> tranh luận phong cách.

**Base commit cho mọi worktree: commit chứa chính file này trên `feature/deploy-k8s-web`.**
Không dùng SHA của `develop`. Lý do (B5 + m5): bản 1 ghi hai SHA khác nhau ở hai tài liệu, và
contract khi đó còn chưa được commit nên worktree sẽ **không nhìn thấy nó**.

Mọi WP phải tuân thủ nguyên văn. **Không WP nào được sửa file này.** Muốn đổi thì quay lại P1, tạm
dừng WP phụ thuộc, duyệt lại ở P2.

---

## C1 — Health endpoint

### `GET /api/healthz` — liveness

| | |
|---|---|
| Luôn trả | `200` khi tiến trình còn chạy |
| Body | **đúng** `{"status":"ok"}`, không thêm field |
| Header | `cache-control` **chứa** `no-store` |
| Render | `export const dynamic = "force-dynamic"` |

Không gọi Redis, database, gateway, hay bất kỳ dependency ngoài nào. Liveness fail ⇒ kubelet **giết
và khởi động lại pod**; nếu probe này phụ thuộc Redis thì Redis chết sẽ kéo theo restart liên tục
toàn bộ pod web — không sửa được Redis mà còn mất luôn khả năng hiển thị trang lỗi tử tế.

### `GET /api/readyz` — readiness

| | |
|---|---|
| Sẵn sàng | `200`, body **đúng** `{"status":"ready","uptimeMs":<integer>}` |
| Chưa sẵn sàng | `503`, body **đúng** `{"status":"not-ready","uptimeMs":<integer>}` |
| Header | `cache-control` chứa `no-store` |
| Render | `export const dynamic = "force-dynamic"` |

Trạng thái nằm ở **mã HTTP**, không ở field trong body: Kubernetes chỉ đọc mã, nên `200` kèm
`{"ready":false}` sẽ bị hiểu là sẵn sàng và traffic đẩy vào pod chưa sẵn sàng.

### C1.1 — Predicate readiness (B2, bản 1 thiếu hẳn)

Bản 1 chốt rất kỹ *hình dạng* nhưng **không nói khi nào là chưa sẵn sàng**, nên nhánh 503 là code
chết không test được. Chốt:

> **`isReady()` trả `true` ngay từ đầu, và trở thành `false` vĩnh viễn khi tiến trình nhận `SIGTERM`.**

Mặc định `true`: frontend Next không có bước khởi tạo bất đồng bộ nào; mặc định `false` thì pod không
bao giờ vào Service endpoints và Deployment treo ở `0/N ready`.

Chuyển sang `false` khi `SIGTERM`: đây mới là lý do readiness tồn tại cho mục tiêu G4. Khi pod bắt đầu
drain, endpoint phải bị rút khỏi Service **trước khi** server đóng, nếu không request đang bay bị reset
— đúng vào lúc khách đang ở phòng chờ.

### C1.2 — Ký hiệu `apps/web/src/lib/readiness.ts` (đóng băng)

```ts
export function uptimeMs(): number;            // nguyen, khong am, ms ke tu khi module khoi tao
export function isReady(): boolean;            // dong bo
export function setReady(ready: boolean): void; // duong DUY NHAT doi trang thai
```

File test đã được viết theo đúng ba chữ ký này. WP-A **không được sửa test** (C5); muốn tên khác thì
escalate để đổi contract.

### C1.3 — Môi trường test cho route handler (m2)

`vitest.config.ts` đặt `environment: "jsdom"` toàn cục và C6 cấm sửa file đó. jsdom không implement
`Request`/`Response`. Mọi file test của route handler phải có docblock ở dòng đầu:

```ts
// @vitest-environment node
```

## C2 — Image, tên, và tham số vận hành

| Hạng mục | Giá trị đóng băng |
|---|---|
| Cổng container | `3000` |
| Tên image | `ghcr.io/phanquocthang/eventflow-web` |
| Tag dùng trong manifest | `sha-<short>` — **không** dùng `:latest` |
| `imagePullPolicy` | `IfNotPresent` |
| Namespace | `eventflow` |
| Tên app | `eventflow-web` |
| Nhãn | `app.kubernetes.io/name: eventflow-web`, `app.kubernetes.io/part-of: eventflow` |

**Owner là `phanquocthang`**, không phải `trandinhvo`: repo là `PHANQUOCTHANG/EvenFlow-` nên
`github.repository_owner` của `cd-web.yml` cho ra chuỗi đó sau khi hạ chữ thường.

**Vì sao không `:latest` (B3):** k8s mặc định `imagePullPolicy: Always` cho tag `latest` ⇒ pod luôn
cố pull từ GHCR, **bỏ qua image build local**; package GHCR mặc định private và chưa có
`imagePullSecrets` ⇒ `ImagePullBackOff`. Và `:latest` phá rollback: `kubectl rollout undo` quay về
đúng cùng một tag, tức không rollback gì cả. Cập nhật tag bằng `kubectl set image` (ghi trong runbook).

### C2.1 — Rolling update và graceful shutdown (M3)

Mục tiêu G4 là "không sập ở T0", nên một `kubectl rollout` giữa lúc đang bán vé không được cắt request:

| | |
|---|---|
| `strategy.rollingUpdate.maxUnavailable` | `0` |
| `strategy.rollingUpdate.maxSurge` | `1` |
| `terminationGracePeriodSeconds` | `30` |
| `preStop` | `node -e "setTimeout(()=>{},5000)"` |

`preStop` dùng `node` chứ không `sleep`, cùng lý do với C3: không phụ thuộc busybox của base image.

### C2.2 — Tham số probe (M4)

| Probe | Tham số |
|---|---|
| `startupProbe` → `/api/healthz` | `periodSeconds: 2`, `failureThreshold: 30` |
| `livenessProbe` → `/api/healthz` | `periodSeconds: 10`, `timeoutSeconds: 2`, `failureThreshold: 3` |
| `readinessProbe` → `/api/readyz` | `periodSeconds: 5`, `timeoutSeconds: 2`, `failureThreshold: 3`, `successThreshold: 1` |

Có `startupProbe` để tách "đang khởi động" khỏi "đã treo"; thiếu nó thì lần đầu khởi động chậm sẽ bị
liveness giết và người đọc kết luận sai là health endpoint hỏng.

### C2.3 — HPA và replicas (m3)

Deployment **không** khai báo `replicas` — để HPA làm chủ. Khai cả hai thì mỗi lần `apply` sẽ giật số
pod về giá trị tĩnh sau khi HPA đã scale. HPA: `minReplicas: 2`, `maxReplicas: 6`, mục tiêu CPU 60%.

### C2.4 — Ingress (M7)

| | |
|---|---|
| `ingressClassName` | `nginx` |
| `host` | `eventflow.localtest.me` |
| `path` | `/` · `pathType: Prefix` |
| Rewrite | **không** |

Không dùng path-prefix: `next.config.mjs` không có `basePath`/`assetPrefix` và C6 cấm sửa nó, nên
`/web` + rewrite sẽ làm mọi asset `/_next/static/*` trả 404 mà **không có đường sửa**.

## C3 — Lệnh healthcheck trong container

```
node -e "fetch('http://127.0.0.1:3000/api/healthz').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))"
```

Không `curl` (không có trong `node:22-alpine`), không `wget` (có nhưng là chi tiết của base image).

Tham số compose: `interval: 10s`, `timeout: 3s`, `retries: 5`, `start_period: 30s`.

**Điều kiện bắt buộc để lệnh này hoạt động (M5):** Next standalone bind theo `process.env.HOSTNAME`
(`server.js` do `output: "standalone"` sinh ra: `const hostname = process.env.HOSTNAME || '0.0.0.0'`).
Docker đặt `HOSTNAME` = container id ⇒ Next bind vào IP eth0, **không** bind loopback ⇒
`fetch('http://127.0.0.1:...')` nhận ECONNREFUSED và service `web` **unhealthy vĩnh viễn**.

Nên `deploy/docker/web.Dockerfile` phải có `ENV HOSTNAME=0.0.0.0` và `ENV PORT=3000`. File đó thuộc
**Integrator** (xem C5).

## C4 — Script gate

| | |
|---|---|
| Đường dẫn | `.github/scripts/gates.sh` |
| Gọi từ | thư mục gốc repo, bằng `bash .github/scripts/gates.sh` |
| `make gates` | **tuỳ chọn** — máy dev không có `make`, nên đây không phải đường chạy chính thức |
| Base ref | biến môi trường `BASE_REF`, mặc định là base commit ở đầu file này |

### C4.1 — Bốn lệnh, ghi literal (M1)

```bash
(cd apps/web && npm run lint)
(cd apps/web && npm run typecheck)
(cd apps/web && npm run test:coverage)
(cd apps/web && npm run build)
```

Không gate Go/Python: máy dev **không có `make`, không có `go`, không có `golangci-lint`** (đã kiểm).

> **Sửa lý do của bản 1 (m1).** Bản 1 viết script này tồn tại vì "CI quên typecheck". **Sai.**
> `ci.yml` đã có đủ bốn: dòng 104 `npm run lint`, 105 `npm run typecheck`, 170 `test:coverage`,
> 227 `npm run build`. Lỗi thật đã xảy ra là ở vòng chạy **cục bộ** trước khi commit. Script này để
> chạy đúng bốn lệnh đó **cục bộ**, cộng hai check mà CI không có (C4.2, C4.3).

### C4.2 — Check diff của test (M2)

```bash
git diff --unified=0 "$BASE_REF"...HEAD -- 'apps/web/**/*.test.*' | grep -E '^\+' | grep -E '\.(skip|only)\(|it\.todo\('
```

Có kết quả ⇒ fail. Chỉ xét **dòng thêm**, nên test cũ vốn đã có thì không bị tính.

### C4.3 — Check chống prerender (M9)

Sau `npm run build`, khẳng định `/api/healthz` và `/api/readyz` **không** nằm trong
`.next/prerender-manifest.json`. Có là fail.

Đây là rủi ro hạng nhất của chính spec và **đã xảy ra thật** trong repo: commit `9607fa6` sửa đúng
lỗi này cho đồng hồ đếm ngược. Probe xanh trong khi tiến trình đã hỏng là chế độ lỗi tệ nhất.

### C4.4 — Validate manifest k8s (B1 — giả định của bản 1 đã được chứng minh là SAI)

Bản 1 viết "`kubectl` có → validate manifest được **kể cả khi chưa có cluster**". Đã kiểm trên máy thật:

```
$ kubectl config current-context
error: current-context is not set

$ kubectl apply --dry-run=client -f ns.yaml
error: failed to download openapi: Get "http://localhost:8080/openapi/v2?timeout=32s": ... refused
```

`--dry-run=client` **không** phải chế độ offline. Chốt lại:

1. Điều kiện phát hiện là **`kubectl cluster-info` thành công**, không phải "có `kubectl`".
2. Có cluster ⇒ `kubectl apply --dry-run=server -f deploy/k8s/` (đây mới là validate thật).
3. Không có cluster ⇒ chỉ **parse YAML** bằng `js-yaml` (đã có trong `node_modules`) để bắt lỗi cú
   pháp, **và in rõ**: `[gates] BO QUA validate k8s: khong co cluster. YAML chi duoc parse cu phap.`
   Tuyệt đối không âm thầm pass.

Script duyệt `deploy/k8s/` theo **glob**, không giả định tên file — nhờ vậy WP-B tự do đặt tên.

## C5 — Ranh giới sở hữu file

| WP | Sở hữu độc quyền |
|---|---|
| **WP-A** | `apps/web/src/app/api/healthz/route.ts`, `apps/web/src/app/api/readyz/route.ts`, `apps/web/src/lib/readiness.ts`, `apps/web/src/instrumentation.ts` |
| **WP-B** | `deploy/k8s/**` |
| **WP-C** | `.github/scripts/gates.sh`, `deploy/compose/docker-compose.yml`, `Makefile` |
| **Integrator** | `deploy/docker/web.Dockerfile`, `docs/workflow/deploy-k8s-web/**` |

**File test — KHÔNG WP nào được sửa** (M6, bản 1 có glob `**` nuốt luôn file test):

```
apps/web/src/app/api/healthz/route.test.ts
apps/web/src/app/api/readyz/route.test.ts
apps/web/src/lib/readiness.test.ts
```

`instrumentation.ts` thuộc WP-A vì SIGTERM phải gọi `setReady(false)` (C1.1). Nếu Next yêu cầu bật cờ
trong `next.config.mjs` thì **escalate**, không tự sửa (C6).

`gates.sh` cần bit thực thi: Integrator chạy `git update-index --chmod=+x` sau khi merge.

## C6 — Những gì KHÔNG WP nào được làm

- Sửa `apps/web/next.config.mjs`
- Sửa `.github/workflows/**`
- Sửa `apps/web/vitest.config.ts`, `vitest.setup.ts`
- Sửa bất kỳ file test nào
- Thêm dependency vào `apps/web/package.json`
- Chạy `docker build` hoặc `kubectl apply` thật
- Merge — chỉ Integrator được merge

## C7 — Giới hạn đã biết, phải ghi vào runbook chứ không được im lặng

| Hạng mục | Vì sao chưa chứng minh được |
|---|---|
| **`NEXT_PUBLIC_API_BASE` (B4)** | Next inline biến `NEXT_PUBLIC_*` vào bundle lúc `next build`. Image hiện tại build **không** có biến này ⇒ `API_BASE = ""` ⇒ `queue-client` gọi URL tương đối trên chính origin của pod web. Đặt `environment:` trong compose **hoàn toàn vô tác dụng** — đây là giả định sai **đã có sẵn trong repo**. Deploy sẽ "thành công" (pod Ready, cả hai probe xanh) mà frontend không gọi được backend. Sửa đúng là thêm `ARG NEXT_PUBLIC_API_BASE` vào Dockerfile + truyền `--build-arg` trong CD — **cần người duyệt** vì chạm CD. |
| **HPA (m4)** | HPA theo CPU **cũng** cần metrics-server, mà Docker Desktop không cài sẵn. HPA sẽ ở `<unknown>/60%`. Runbook không được nói "HPA đã chạy". |
| **Ingress** | Docker Desktop không có ingress controller. Truy cập bằng `kubectl port-forward`. |
| **Bằng chứng CI (M8)** | `ci.yml` paths-filter chỉ có `go`, `apps/web/**`, `python`. `deploy/k8s/**`, `.github/scripts/**`, `Makefile`, `deploy/compose/**` **không khớp filter nào** ⇒ PR của WP-B và WP-C qua CI "xanh" mà không chạy lệnh nào. Bằng chứng cho AC-4 là **thủ công**, dán log vào `integration-report.md`. |
| **Logging** | Next standalone ghi stdout dạng text. Chưa có structured logging / correlation id — món của S5. |
