# Runbook — deploy `apps/web` lên Kubernetes của Docker Desktop

Đóng **AC-6**. Phạm vi: cluster **Docker Desktop local**. Apply lên bất kỳ cluster nào khác là
điểm cần người duyệt (spec §6.2).

Mọi lệnh chạy từ **gốc repo** trên **Git Bash** (môi trường thật của dự án). Mục §6 ghi rõ phần
**chưa** verify được và vì sao — đọc mục đó trước khi báo cáo "đã deploy xong".

Ký hiệu dùng xuyên suốt:

```bash
IMG=ghcr.io/phanquocthang/eventflow-web
TAG=sha-60b4930        # = 7 ky tu dau cua commit tich hop; phai khop image: trong 10-deployment.yaml
NS=eventflow
```

---

## 1. Build image

```bash
docker build -f deploy/docker/web.Dockerfile -t "$IMG:$TAG" .
```

Context là **gốc repo** (`.`), không phải `apps/web/`: Dockerfile copy `apps/web/package.json` theo
đường dẫn từ gốc.

Hai điều phải kiểm ngay sau khi build, vì cả hai đều làm pod fail 100% mà log build vẫn xanh:

```bash
# (a) user cua image phai la DANG SO. Xem muc 6.1.
docker image inspect "$IMG:$TAG" --format 'Config.User={{.Config.User}}'
# ky vong: Config.User=1000

# (b) tag phai tro toi commit CO health endpoint, khong phai commit contract.
git show --stat --oneline 60b4930 | head -1
```

### Truyền API base (giới hạn đã biết — xem 6.4)

```bash
docker build -f deploy/docker/web.Dockerfile \
  --build-arg NEXT_PUBLIC_API_BASE=http://localhost:8080 \
  -t "$IMG:$TAG" .
```

Phải là `--build-arg`, **không** phải `-e` / `env:` / `environment:`. Next inline biến
`NEXT_PUBLIC_*` vào bundle lúc `next build`; đặt ở runtime hoàn toàn vô tác dụng và deploy sẽ
"thành công" với cả hai probe xanh trong khi frontend không gọi được backend.

## 2. Đưa image vào cluster

**Không cần bước load.** Kubernetes của Docker Desktop dùng **chung image store** với Docker
daemon, nên image vừa build đã nhìn thấy được và `imagePullPolicy: IfNotPresent` lấy đúng nó.
(Trên `kind` thì phải `kind load docker-image`, trên `minikube` phải `minikube image load` — nhưng
cả hai đều ngoài phạm vi runbook này.)

Điều kiện tiên quyết: **Kubernetes phải được bật** trong Docker Desktop → Settings → Kubernetes →
*Enable Kubernetes* → Apply & restart. Kiểm:

```bash
kubectl config current-context    # ky vong: docker-desktop
kubectl cluster-info
```

Nếu `kubectl config current-context` in `error: current-context is not set` thì Kubernetes **chưa
bật** — mọi bước dưới đây sẽ fail, và `gates.sh` sẽ **bỏ qua** check C4.4 thay vì fail (xem 6.3).

## 3. Apply

```bash
kubectl apply -f deploy/k8s/
```

Thứ tự alphabet của `kubectl apply -f <dir>` là lý do các file có tiền tố số: `00-namespace.yaml`
phải vào trước, nếu không bốn manifest còn lại fail vì namespace `eventflow` chưa tồn tại.

Kiểm cú pháp + schema **thật** trước khi apply (server-side, cần cluster):

```bash
kubectl apply -f deploy/k8s/00-namespace.yaml          # namespace phai ton tai truoc
kubectl apply -f deploy/k8s/ --dry-run=server
```

`--dry-run=client` **không** thay thế được: nó vẫn tải OpenAPI schema từ API server
(`localhost:8080`) nên offline là fail, không phải pass. Muốn kiểm offline thật thì cài
`kubeconform`:

```bash
go install github.com/yannh/kubeconform/cmd/kubeconform@latest
kubeconform -strict -summary deploy/k8s/
```

Chưa cài trong dự án, nên hiện **chưa có** cách kiểm manifest offline nào đáng tin.

## 4. Kiểm pod đã Ready

```bash
kubectl -n "$NS" rollout status deployment/eventflow-web --timeout=120s
kubectl -n "$NS" get pods -l app.kubernetes.io/name=eventflow-web -o wide
```

Lần apply đầu tiên Deployment tạo **1** pod (manifest cố ý không khai báo `replicas`, C2.3), rồi HPA
kéo lên `minReplicas: 2`. Việc áp biên dưới **không** cần metrics-server.

Nếu pod không lên Ready:

```bash
kubectl -n "$NS" describe pod -l app.kubernetes.io/name=eventflow-web
```

Ba chế độ lỗi đã biết và dấu hiệu nhận ra chúng:

| Dấu hiệu trong `describe` | Nguyên nhân |
|---|---|
| `CreateContainerConfigError: ... image has non-numeric user (node)` | Dockerfile dùng `USER node` thay vì `USER 1000`. Xem 6.1. |
| `ImagePullBackOff` | Tag trên cluster không khớp tag đã build, hoặc ai đó đổi sang `:latest` (k8s mặc định `Always` cho `latest` ⇒ bỏ qua image local ⇒ pull từ GHCR private ⇒ fail). |
| `Readiness probe failed: HTTP probe failed with statuscode: 503` kéo dài | `/api/readyz` trả 503 vĩnh viễn. Rollout sẽ **treo mãi** vì `maxUnavailable: 0`. |

## 5. Xem log, truy cập, rollback

```bash
# log
kubectl -n "$NS" logs -l app.kubernetes.io/name=eventflow-web --tail=100 -f

# truy cap: KHONG qua Ingress (xem 6.2), dung port-forward
kubectl -n "$NS" port-forward deployment/eventflow-web 3000:3000
curl -i http://localhost:3000/api/healthz    # 200
curl -i http://localhost:3000/api/readyz     # 200

# doi tag (vi du sau khi build commit moi)
kubectl -n "$NS" set image deployment/eventflow-web web="$IMG:sha-<short>"

# rollback
kubectl -n "$NS" rollout history deployment/eventflow-web
kubectl -n "$NS" rollout undo deployment/eventflow-web
kubectl -n "$NS" rollout undo deployment/eventflow-web --to-revision=<N>
```

`rollout undo` chỉ có nghĩa vì tag là `sha-<short>` bất biến. Nếu dùng `:latest` thì undo quay về
**đúng cùng một chuỗi tag**, tức không rollback gì cả.

### Giới hạn của graceful shutdown — phải biết trước khi rollout giữa giờ bán vé

Tổng thời gian tắt một pod là **~10 giây**, không phải 5: `preStop` và `DRAIN_MS` chạy **nối tiếp**,
không song song. kubelet chạy `preStop` trước, chỉ khi nó trả về mới gửi SIGTERM:

```
t=0    pod -> Terminating; endpoints controller rut endpoint; preStop exec (5s)
t=5s   preStop xong -> SIGTERM -> setReady(false), /api/readyz = 503
t=10s  DRAIN_MS het -> process.exit(0)
```

**Giới hạn phải chấp nhận (contracts.md C1.1b):** request đang bay **quá 5 giây** sẽ bị cắt giữa —
`process.exit(0)` không chờ request nào hoàn tất. Với trang web thì chấp nhận được; với một endpoint
ghi dữ liệu thì không, và đó là lý do ranh giới này ghi ở đây chứ không chỉ trong code.

**Không tăng `DRAIN_MS` mà không tăng `terminationGracePeriodSeconds`:** 30s hiện tại phải lớn hơn
`preStop + DRAIN_MS`. Hạ grace period xuống 10s thì SIGKILL rơi đúng lúc `exit(0)` và `ExitCode` sẽ
là 137 thay vì 0.

---

## 6. Những gì CHƯA verify được, và vì sao

### 6.1 Đã verify (bằng container thật, không phải suy luận)

| Hạng mục | Bằng chứng |
|---|---|
| `docker build` thành công | Build xanh; trước slice này image **chưa từng** build được lần nào (`COPY /app/public` trỏ vào thư mục không tồn tại). |
| Image chạy non-root dạng số | `Config.User=1000`; `docker exec … node -e 'process.getuid()'` ⇒ `1000`. |
| Hai probe phân biệt nhau | `/api/healthz` 200 và `/api/readyz` 200 khi bình thường. |
| Drain thật dưới SIGTERM | readyz ⇒ **503 sau 77 ms**, healthz **giữ 200** suốt cửa sổ drain, tiến trình thoát ở t=5129 ms với **ExitCode 0**. SIGTERM thứ hai ở t=2s (dưới `--init`) **không** cắt ngắn drain. |
| Drain thật dưới SIGINT | readyz ⇒ 503 trong ~1s, ExitCode 0. Trước khi đăng ký SIGINT thì Ctrl+C rơi vào default disposition: không drain, exit 130. |

### 6.2 Chưa verify — thiếu hạ tầng trên Docker Desktop

| Hạng mục | Vì sao |
|---|---|
| **HPA có scale theo CPU** | Cần metrics-server, Docker Desktop không cài sẵn. HPA sẽ ở `<unknown>/60%`. **Không được nói "HPA đã chạy"** — chỉ biên `minReplicas: 2` là áp được. Muốn kiểm thật: apply metrics-server release mới nhất kèm `--kubelet-insecure-tls`. |
| **Ingress** | Docker Desktop không có ingress controller. `30-ingress.yaml` apply được nhưng **không route gì**. Truy cập bằng `port-forward` (§5). |
| **Rolling update không cắt traffic (AC-4b)** | Cần ≥2 pod Ready và một client bắn liên tục qua Service trong lúc `rollout restart`. Chưa chạy. |
| **Toàn bộ §3, §4** | Tại thời điểm viết, `kubectl config current-context` in `current-context is not set` ⇒ Kubernetes **chưa bật** trong Docker Desktop. Mọi lệnh apply/kiểm pod trong runbook này **chưa ai chạy** — chúng được viết ra, không phải được xác nhận. |

### 6.3 Chưa verify — gate tự im lặng

`gates.sh` check C4.4 (validate manifest) **bỏ qua** khi không có cluster thay vì fail. Nên một
`gates.sh` xanh trên máy không bật Kubernetes **không** nói gì về tính đúng của `deploy/k8s/`. Dòng
`[gates] BO QUA validate k8s` trong output là dấu hiệu duy nhất.

Hệ quả nặng hơn: CI cũng không bù được. `ci.yml` paths-filter chỉ có `go`, `apps/web/**`, `python` —
`deploy/k8s/**`, `.github/scripts/**`, `deploy/compose/**` **không khớp filter nào**, nên PR chỉ sửa
ba vùng đó sẽ xanh mà **không chạy lệnh nào**. Thêm filter cho `deploy/**` và `.github/scripts/**` là
điểm cần người duyệt (spec §6.5); slice này không tự sửa `ci.yml`.

### 6.4 Chưa verify — frontend gọi được backend

Image dựng **không** có `--build-arg NEXT_PUBLIC_API_BASE` thì `API_BASE = ""` và `queue-client` gọi
URL tương đối trên chính origin của pod web — nơi chỉ có `/api/healthz` và `/api/readyz` — nên mọi
call queue/ticketing trả 404. Pod vẫn Ready, cả hai probe vẫn xanh. **Deploy "thành công" không
chứng minh frontend hoạt động.**

Dockerfile đã có `ARG`, compose đã truyền qua `build.args`. Phần **chưa** làm: `cd-web.yml` chưa
truyền `--build-arg`, vì sửa CD chạm môi trường ngoài repo ⇒ cần người duyệt (spec §6.4).

### 6.5 Rủi ro đã biết, chưa có biện pháp

`readinessProbe.failureThreshold: 1` + `periodSeconds: 2` + `cpu limit: 500m` + **không có
PodDisruptionBudget**: dưới tải đủ nặng để probe timeout 2s, cả hai pod có thể bị rút khỏi endpoints
gần như cùng lúc ⇒ Service còn **0 endpoint**. Tham số probe nằm trong contract đã đóng băng (C2.2)
nên không sửa trong slice này; đã ghi lại để xử lý ở slice sau.

### 6.6 Chưa có

Structured logging / correlation id. Next standalone ghi stdout dạng text thuần — món của S5.
