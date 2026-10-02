# Deploy web lên Kubernetes — spec (bước 1)

| | |
|---|---|
| Mục tiêu | Đưa frontend `apps/web` chạy được trên Kubernetes của Docker Desktop, có probe thật và cổng chất lượng tự động |
| Tier đề xuất | **Standard**, nhưng có **2 điểm cần người duyệt** (xem mục 6) vì chạm CI/CD và có thể dịch tag `:latest` |
| Chế độ | **Parallel** theo §7 của `multi_agent_coding_workflow.pdf` — xem `work-packages.md` |
| Base | commit cuối của `feature/EVF-1802-event-cards` |
| Nguồn | `docs/00-tong-quan.md` mục tiêu G4 + milestone S5; `deploy/compose/docker-compose.yml`; `deploy/docker/web.Dockerfile`; `.github/workflows/cd-web.yml` |

---

## 1. Bối cảnh

`docs/00-tong-quan.md` chốt đích hạ tầng: **G4 "Không sập ở T0" — pre-warm pod trước 30', HPA custom
metric, load shedding có ưu tiên, circuit breaker**; milestone **S5** yêu cầu "K8s, HPA, load test,
chaos drill, runbook".

Hiện trạng đã kiểm:

| Thứ | Trạng thái |
|---|---|
| `deploy/compose/docker-compose.yml` | Có service `web` build từ `web.Dockerfile`, **không có healthcheck** (postgres/redis/rabbitmq thì có) |
| Health endpoint trong `apps/web` | **Không có cái nào** |
| `deploy/k8s/` | **Không tồn tại** |
| `.github/workflows/cd-web.yml` | Publish image lên GHCR sau khi CI xanh trên `main`. **Không có bước deploy** — đúng như đã ghi |
| `web.Dockerfile` | Đã sửa `npm ci` + `apps/web/public/.gitkeep`, nhưng **chưa từng được build thành công lần nào** (Docker daemon chưa chạy ở mọi lần thử) |
| `kubectl` | Có (đi kèm Docker Desktop) → validate manifest được kể cả khi chưa có cluster |
| `helm` / `minikube` / `kind` | Không có |

## 2. Phạm vi

### Trong phạm vi

1. **Health endpoint tách đôi**: `/api/healthz` (liveness) và `/api/readyz` (readiness)
2. **Healthcheck cho service `web`** trong compose
3. **Manifest k8s** cho web: Namespace, Deployment, Service, Ingress, HPA
4. **Script cổng chất lượng** chạy đủ 4 gate + kiểm diff test + validate manifest
5. **Runbook ngắn**: cách chạy, cách kiểm, cách rollback

### Ngoài phạm vi

| Việc | Lý do |
|---|---|
| Manifest cho 8 service Go và ai-worker | 6/8 vẫn là stub health-check 31 dòng; chưa có gì để deploy |
| HPA theo **custom metric** (độ sâu hàng đợi) như G4 đòi | Cần metrics-server + prometheus-adapter; Docker Desktop không có sẵn. Làm HPA theo CPU và **ghi rõ đây chưa phải cái G4 yêu cầu** |
| Bước deploy trong `cd-web.yml` | Cần cluster thật + secret. Người duyệt mới quyết (mục 6) |
| Load test, chaos drill (S5) | Cần cluster và thời gian riêng |
| TLS / cert-manager | Không có domain |

## 3. Quyết định phải tuân thủ

**3.1 Liveness KHÔNG kiểm dependency.** `/api/healthz` chỉ trả lời "tiến trình còn sống". Nếu nó fail
khi Redis hay gateway chết thì k8s sẽ **restart pod web** — việc đó không sửa được Redis, chỉ làm mất
luôn frontend và xoá cả khả năng hiển thị trang lỗi tử tế. Đây là lỗi kinh điển của probe.

**3.2 Readiness mới là thứ G4 cần.** Pre-warm pod trước 30 phút chỉ có nghĩa khi biết **đúng lúc nào**
pod sẵn sàng nhận traffic. Pod chưa ready thì Service không route vào.

**3.3 Hai endpoint phải động.** Next prerender tĩnh mọi route hiện có. Nếu probe bị prerender thì nó
trả về một kết quả **đóng băng từ lúc build** — probe luôn xanh kể cả khi tiến trình đã hỏng. Bắt buộc
`dynamic = "force-dynamic"` và `cache-control: no-store`. Đây đúng là bẫy đã gặp ở đồng hồ đếm ngược.

**3.4 Không có `resources` thì HPA vô nghĩa.** HPA theo CPU tính theo phần trăm của `requests`. Thiếu
`requests` thì HPA không có mẫu số và không scale.

**3.5 Container chạy non-root.** `web.Dockerfile` đã có `USER node`. Manifest phải khẳng định lại bằng
`securityContext` (`runAsNonRoot`, `allowPrivilegeEscalation: false`, drop ALL capabilities), nếu không
một lần sửa Dockerfile là mất tính chất đó mà không ai biết.

**3.6 Script gate phải chạy ĐỦ bốn lệnh, không cho chọn lọc.** Lý do cụ thể: ở slice trước mình chạy
`test` + `lint` + `build` nhưng **quên `typecheck`**, và một type error đã lọt vào commit `9607fa6`.
PDF bước 6 ghi rõ gate là "script, không dùng LLM" chính để chặn đúng kiểu lỗi đó.

## 4. Tiêu chí nghiệm thu (AC)

### AC-1 — `/api/healthz`
- **Then** `GET` trả `200` và JSON có `status: "ok"`
- **Then** header `cache-control` chứa `no-store`
- **Then** route **không** bị prerender tĩnh (xuất hiện là dynamic trong output `next build`)
- **Then** handler **không** gọi tới bất kỳ dependency ngoài nào (không fetch, không redis, không db)

### AC-2 — `/api/readyz`
- **Then** `GET` trả `200` + `status: "ready"` khi app sẵn sàng
- **Then** body có `uptimeMs` là số không âm
- **Then** `cache-control: no-store`, không prerender tĩnh
- **Then** khi chưa sẵn sàng trả **`503`** (không phải 200 kèm cờ) — k8s chỉ đọc mã trạng thái

### AC-3 — Compose
- **Then** service `web` có `healthcheck` gọi `/api/healthz`
- **Then** `interval`, `timeout`, `retries`, `start_period` đều được đặt tường minh
- **Then** `docker compose config` parse thành công

### AC-4 — Manifest k8s
- **Then** `deploy/k8s/` có Namespace, Deployment, Service, Ingress, HPA
- **Then** Deployment có `livenessProbe` → `/api/healthz`, `readinessProbe` → `/api/readyz`
- **Then** có `resources.requests` **và** `limits` cho cpu + memory
- **Then** `securityContext`: `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`
- **Then** image tham chiếu đúng tên mà `cd-web.yml` publish (`ghcr.io/<owner>/eventflow-web`)
- **Then** HPA có `minReplicas` ≥ 2 (G4: không được có điểm chết đơn lẻ ở T0)
- **Then** `kubectl apply --dry-run=client -f deploy/k8s/` thành công cho **mọi** file

### AC-5 — Script gate
- **Then** chạy **đủ** `lint`, `typecheck`, `test:coverage`, `build`; thiếu bất kỳ cái nào là lỗi script
- **Then** fail ở bất kỳ bước nào → exit code khác 0 và nêu rõ bước nào fail
- **Then** kiểm diff test: có `.skip` / `.only` / `it.todo` mới thêm → fail
- **Then** validate `deploy/k8s/**` bằng `kubectl --dry-run=client` nếu `kubectl` có; không có thì **bỏ qua có thông báo**, không âm thầm pass
- **Then** chạy được trên Git Bash của Windows (môi trường thật của dự án)

### AC-6 — Runbook
- **Then** có lệnh cụ thể để build image, load vào cluster, apply, kiểm pod ready, xem log, rollback
- **Then** ghi rõ những gì **chưa** verify được và vì sao

### AC-7 — Không hồi quy
- **Then** toàn bộ test hiện có vẫn pass; không sửa file ngoài ownership của WP (xem `work-packages.md`)

## 5. Rủi ro

| Rủi ro | Giảm thiểu |
|---|---|
| Liveness kiểm dependency → k8s restart pod web vô ích | AC-1 cấm gọi dependency ngoài |
| Probe bị prerender → luôn xanh kể cả khi hỏng | AC-1/AC-2 bắt dynamic + kiểm trong output build |
| Thiếu `resources` → HPA không scale | AC-4 bắt có cả requests và limits |
| Script gate chạy thiếu lệnh (đã xảy ra thật) | AC-5 bắt đủ 4 lệnh, thiếu là lỗi script |
| Ingress không chạy vì chưa có ingress controller | Ghi rõ là **chưa verify**; truy cập qua port-forward |
| Image chưa từng build thành công lần nào | Là việc đầu tiên khi Docker chạy; nếu fail thì dừng và báo, không sửa bừa base image |

## 6. Điểm cần người duyệt

1. **Trước khi thêm bước deploy vào `cd-web.yml`** — nó sẽ tác động tới môi trường ngoài repo.
2. **Trước khi apply lên bất kỳ cluster nào không phải Docker Desktop local.**
3. Nếu `docker build` fail vì lý do ngoài Dockerfile (mạng, base image) → dừng và báo.
