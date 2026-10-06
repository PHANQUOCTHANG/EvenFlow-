# Ma trận truy vết — slice deploy web lên Kubernetes

Nguồn: `spec.md` (AC-1 … AC-7), `contracts.md` (C1 … C7, **bản 2 sau P2 vòng 2**), `work-packages.md` (WP-A/B/C).

Bước 2 (Test Design) chỉ viết **test cho WP-A** (AC-1, AC-2). Các AC còn lại được kiểm bằng **lệnh**
(không phải unit test) nên vẫn có một dòng mỗi AC để ma trận phủ **toàn bộ** slice, không chỉ phần WP-A.

Trạng thái ở bước này là `chưa chạy` với **mọi** dòng: ba file test được viết trước khi WP-A hiện thực,
nên `apps/web/src/app/api/healthz/route.ts`, `apps/web/src/app/api/readyz/route.ts` và
`apps/web/src/lib/readiness.ts` chưa tồn tại. "Module not found" lúc này là trạng thái **đúng**.

| AC | Nội dung ngắn | Test file | Tên test | Trạng thái |
|---|---|---|---|---|
| AC-1 | `GET /api/healthz` trả 200 khi tiến trình còn chạy | `apps/web/src/app/api/healthz/route.test.ts` | `tra 200 khi tien trinh con chay` | chưa chạy |
| AC-1 | Handler trả `Response` chuẩn (assert được status/headers/json) | `apps/web/src/app/api/healthz/route.test.ts` | `tra ve Response chuan (assert duoc bang status/headers/json)` | chưa chạy |
| AC-1 | Body đúng nguyên văn `{"status":"ok"}` (C1) | `apps/web/src/app/api/healthz/route.test.ts` | `body dung nguyen van { status: "ok" } theo C1` | chưa chạy |
| AC-1 | Probe bị gọi liên tục: nhiều lần vẫn 200, body không đổi | `apps/web/src/app/api/healthz/route.test.ts` | `goi nhieu lan van 200 va body khong doi (probe bi goi lien tuc)` | chưa chạy |
| AC-1 | Không bao giờ trả mã khác 200, kể cả khi readiness đã not-ready (liveness khác readiness) | `apps/web/src/app/api/healthz/route.test.ts` | `khong bao gio tra ma khac 200 (ke ca khi readiness da not-ready)` | chưa chạy |
| AC-1 | Header `cache-control` chứa `no-store` | `apps/web/src/app/api/healthz/route.test.ts` | `header cache-control chua no-store` | chưa chạy |
| AC-1 | `no-store` trên **mọi** response, không chỉ lần đầu | `apps/web/src/app/api/healthz/route.test.ts` | `cache-control chua no-store tren MOI response, khong chi lan dau` | chưa chạy |
| AC-1 | Không prerender tĩnh: module export `dynamic = "force-dynamic"` | `apps/web/src/app/api/healthz/route.test.ts` | `module route export dynamic = "force-dynamic" (neu prerender thi probe dong bang tu luc build)` | chưa chạy |
| AC-1 | Không gọi dependency ngoài: `globalThis.fetch` bị stub và không được gọi | `apps/web/src/app/api/healthz/route.test.ts` | `khong goi globalThis.fetch khi xu ly request` | chưa chạy |
| AC-1 | Không gọi `XMLHttpRequest` khi xử lý request | `apps/web/src/app/api/healthz/route.test.ts` | `khong goi XMLHttpRequest khi xu ly request` | chưa chạy |
| AC-1 | Dependency ngoài chết (`fetch` reject) vẫn không làm liveness fail | `apps/web/src/app/api/healthz/route.test.ts` | `fetch bi loi (dependency ngoai chet) van KHONG lam liveness fail` | chưa chạy |
| AC-1 | Nạp module không gây request nào (không side effect lúc import) | `apps/web/src/app/api/healthz/route.test.ts` | `nap module route cung khong gay request nao (khong co side effect luc import)` | chưa chạy |
| AC-1 | Module nạp lại + gọi handler cũng không fetch | `apps/web/src/app/api/healthz/route.test.ts` | `nap module route va goi handler moi nap cung khong fetch` | chưa chạy |
| AC-1 | `/api/healthz` xuất hiện là **dynamic** trong output `next build` | — (kiểm bằng lệnh, unit test chỉ kiểm được export `dynamic`) | `npm run build` trong `apps/web` — gate `build` của WP-C | chưa chạy |
| AC-2 | `GET /api/readyz` trả 200 khi đã sẵn sàng | `apps/web/src/app/api/readyz/route.test.ts` | `tra 200` | chưa chạy |
| AC-2 | Body có `status: "ready"` khi sẵn sàng | `apps/web/src/app/api/readyz/route.test.ts` | `body co status = "ready"` | chưa chạy |
| AC-2 | Body đúng hình dạng C1: chỉ gồm `status` + `uptimeMs` | `apps/web/src/app/api/readyz/route.test.ts` | `body dung hinh dang cua C1: chi gom status va uptimeMs` | chưa chạy |
| AC-2 | `cache-control` chứa `no-store` khi sẵn sàng | `apps/web/src/app/api/readyz/route.test.ts` | `header cache-control chua no-store` | chưa chạy |
| AC-2 | Handler trả `Response` chuẩn | `apps/web/src/app/api/readyz/route.test.ts` | `tra ve Response chuan` | chưa chạy |
| AC-2 | Chưa sẵn sàng thì trả **503**, không phải 200 kèm cờ trong body | `apps/web/src/app/api/readyz/route.test.ts` | `tra 503, khong phai 200 kem co trong body` | chưa chạy |
| AC-2 | Chưa sẵn sàng thì body `status: "not-ready"` | `apps/web/src/app/api/readyz/route.test.ts` | `body co status = "not-ready"` | chưa chạy |
| AC-2 | Chưa sẵn sàng vẫn có `uptimeMs` là số nguyên không âm | `apps/web/src/app/api/readyz/route.test.ts` | `van co uptimeMs la so nguyen khong am` | chưa chạy |
| AC-2 | Chưa sẵn sàng vẫn `cache-control: no-store` | `apps/web/src/app/api/readyz/route.test.ts` | `header cache-control van chua no-store` | chưa chạy |
| AC-2 | Chưa sẵn sàng vẫn đúng hình dạng body của C1 | `apps/web/src/app/api/readyz/route.test.ts` | `body van dung hinh dang cua C1: chi gom status va uptimeMs` | chưa chạy |
| AC-2 | Mã HTTP và chuỗi `status` luôn khớp ở **cả hai** trạng thái | `apps/web/src/app/api/readyz/route.test.ts` | `setReady(%s) -> ma %i kem status "%s"` (it.each, 2 ca: ready / not-ready) | chưa chạy |
| AC-2 | Không dùng field boolean `ready` trong body để biểu diễn trạng thái (anti-pattern nêu trong C1) | `apps/web/src/app/api/readyz/route.test.ts` | ``KHONG dung field boolean `ready` trong body de bieu dien trang thai`` | chưa chạy |
| AC-2 | Hạ xuống not-ready rồi đưa lên lại thì quay về 200 (không dính trạng thái) | `apps/web/src/app/api/readyz/route.test.ts` | `ha xuong not-ready roi dua len lai -> quay ve 200 (khong dinh trang thai)` | chưa chạy |
| AC-2 | `uptimeMs` trong response là số nguyên không âm, hữu hiện | `apps/web/src/app/api/readyz/route.test.ts` | `la so nguyen khong am` | chưa chạy |
| AC-2 | `uptimeMs` không đi lùi giữa hai request liên tiếp | `apps/web/src/app/api/readyz/route.test.ts` | `khong di lui giua hai request lien tiep` | chưa chạy |
| AC-2 | `uptimeMs` tăng đúng theo đồng hồ (fake timers, nạp lại module dưới đồng hồ giả) | `apps/web/src/app/api/readyz/route.test.ts` | `tang dung theo dong ho khi thoi gian chay (fake timers)` | chưa chạy |
| AC-2 | Không prerender tĩnh: export `dynamic = "force-dynamic"` | `apps/web/src/app/api/readyz/route.test.ts` | `module route export dynamic = "force-dynamic"` | chưa chạy |
| AC-2 | `dynamic` không phải giá trị tĩnh nào khác (`force-static` / `auto` / `error`) | `apps/web/src/app/api/readyz/route.test.ts` | `dynamic khong phai gia tri tinh nao khac` | chưa chạy |
| AC-2 | `uptimeMs()` trả number đồng bộ, không Promise | `apps/web/src/lib/readiness.test.ts` | `tra ve number, khong phai Promise (API dong bo, khong an giau I/O)` | chưa chạy |
| AC-2 | `uptimeMs()` là số nguyên | `apps/web/src/lib/readiness.test.ts` | `la so nguyen` | chưa chạy |
| AC-2 | `uptimeMs()` không âm | `apps/web/src/lib/readiness.test.ts` | `khong am` | chưa chạy |
| AC-2 | `uptimeMs()` hữu hiện, không bao giờ NaN | `apps/web/src/lib/readiness.test.ts` | `huu hien, khong bao gio NaN` | chưa chạy |
| AC-2 | `uptimeMs()` = 0 ngay sau khi module khởi tạo (mốc là **module init**, không phải process start) | `apps/web/src/lib/readiness.test.ts` | `bang 0 ngay sau khi module khoi tao` | chưa chạy |
| AC-2 | `uptimeMs()` tăng đúng bằng lượng thời gian đã trôi (fake timers +5000ms) | `apps/web/src/lib/readiness.test.ts` | `tang dung bang luong thoi gian da troi` | chưa chạy |
| AC-2 | `uptimeMs()` không đi lùi khi đồng hồ tiến tiếp (ba nhịp) | `apps/web/src/lib/readiness.test.ts` | `khong di lui khi dong ho tien tiep (hai nhip lien tiep)` | chưa chạy |
| AC-2 | `uptimeMs()` không đi lùi giữa hai lần gọi (đồng hồ thật) | `apps/web/src/lib/readiness.test.ts` | `khong di lui giua hai lan goi lien tiep (dong ho that)` | chưa chạy |
| AC-2 | `uptimeMs()` vẫn đếm tiếp khi app đã bị hạ xuống not-ready (đang drain vẫn là đang chạy) | `apps/web/src/lib/readiness.test.ts` | `van dem tiep khi app da bi ha xuong not-ready (dang drain van la dang chay)` | chưa chạy |
| AC-2 | Mặc định là **sẵn sàng** ngay sau khi module khởi tạo (nếu không, pod không bao giờ vào Service endpoints) | `apps/web/src/lib/readiness.test.ts` | `mac dinh la san sang ngay sau khi module khoi tao` | chưa chạy |
| AC-2 | `isReady()` trả boolean thật, không phải giá trị truthy/falsy mơ hồ | `apps/web/src/lib/readiness.test.ts` | `tra ve boolean that, khong phai gia tri truthy/falsy mo ho` | chưa chạy |
| AC-2 | `isReady()` không trả Promise (readyz phải đọc được đồng bộ) | `apps/web/src/lib/readiness.test.ts` | `khong tra ve Promise (readyz phai doc duoc dong bo)` | chưa chạy |
| AC-2 | `setReady(false)` thì `isReady()` là false | `apps/web/src/lib/readiness.test.ts` | `setReady(false) lam isReady() thanh false` | chưa chạy |
| AC-2 | `setReady(true)` thì `isReady()` là true | `apps/web/src/lib/readiness.test.ts` | `setReady(true) lam isReady() thanh true` | chưa chạy |
| AC-2 | Đổi trạng thái qua lại nhiều lần đều phản ánh đúng | `apps/web/src/lib/readiness.test.ts` | `doi trang thai qua lai nhieu lan deu phan anh dung` | chưa chạy |
| AC-2 | `setReady` cùng giá trị hai lần là idempotent | `apps/web/src/lib/readiness.test.ts` | `goi setReady cung gia tri hai lan khong doi ket qua (idempotent)` | chưa chạy |
| AC-2 | Đọc lại nhiều lần không tự đổi trạng thái | `apps/web/src/lib/readiness.test.ts` | `doc lai nhieu lan khong tu dong doi trang thai` | chưa chạy |
| AC-1, AC-2 | `readiness` không chạm dependency ngoài (`fetch` / `XMLHttpRequest` không được gọi) | `apps/web/src/lib/readiness.test.ts` | `isReady / setReady / uptimeMs khong goi fetch hay XMLHttpRequest` | chưa chạy |
| AC-1, AC-2 | Nạp `readiness` không gây request nào, và export đủ 3 hàm | `apps/web/src/lib/readiness.test.ts` | `nap module khong gay request nao (khong co side effect luc import)` | chưa chạy |
| AC-3 | Compose: service `web` có `healthcheck` gọi `/api/healthz`, đặt tường minh `interval`/`timeout`/`retries`/`start_period`, parse được | **WP-C** sở hữu (`deploy/compose/docker-compose.yml`) — kiểm bằng **lệnh**, không có unit test | `docker compose -f deploy/compose/docker-compose.yml config` | chưa chạy |
| AC-4 | Manifest k8s: Namespace/Deployment/Service/Ingress/HPA, probe trỏ đúng C1, `resources.requests` + `limits`, `securityContext` non-root drop ALL, image `ghcr.io/phanquocthang/eventflow-web` tag `sha-<short>`, `minReplicas` >= 2 | **WP-B** sở hữu (`deploy/k8s/**`) — kiểm bằng **lệnh**, không có unit test | `kubectl apply --dry-run=server -f deploy/k8s/` ở bước **P4.5** (client dry-run KHÔNG chạy offline) | chưa chạy |
| AC-5 | Script gate chạy đủ `lint`, `typecheck`, `test:coverage`, `build`; fail thì exit khác 0 và nêu rõ bước; kiểm diff test `.skip`/`.only`/`it.todo`; validate `deploy/k8s/**`; chạy được trên Git Bash Windows | **WP-C** sở hữu (`.github/scripts/gates.sh`, `Makefile`) — kiểm bằng **lệnh**, không có unit test | `bash .github/scripts/gates.sh` từ gốc repo (`make gates` chỉ là tuỳ chọn — máy dev không có `make`) | chưa chạy |
| AC-6 | Runbook: lệnh cụ thể để build image, load vào cluster, apply, kiểm pod ready, xem log, rollback; ghi rõ phần **chưa** verify được và vì sao | **Integrator** sở hữu (`docs/workflow/deploy-k8s-web/runbook.md`) — kiểm bằng **đọc + chạy lệnh**, không có unit test | Đối chiếu từng lệnh trong runbook khi Docker Desktop chạy | chưa chạy |
| AC-7 | Không hồi quy: toàn bộ test hiện có vẫn pass; không WP nào sửa file ngoài ownership (C5) | **Integrator** kiểm sau merge — kiểm bằng **lệnh**, không có unit test | `npm run test:coverage` (apps/web) + `git diff --name-only` đối chiếu bảng C5 | chưa chạy |

## Bổ sung sau P2 vòng 2

| AC | Nội dung ngắn | Chủ | Bằng chứng | Trạng thái |
|---|---|---|---|---|
| AC-2b | `instrumentation.ts` bắt `SIGTERM` → `setReady(false)` → chờ 5s → `exit(0)`; readyz trả `503` trong cửa sổ drain | **WP-A** | P4.5 bước 5: gửi SIGTERM, curl `/api/readyz` phải ra `503` chứ không `ECONNREFUSED` | chưa chạy |
| AC-4b | `maxUnavailable: 0`, `maxSurge: 1`, `terminationGracePeriodSeconds: 30`, `preStop` | **WP-B** | đọc manifest + `--dry-run=server` ở P4.5 | chưa chạy |
| AC-4c | đủ `startupProbe` / `livenessProbe` / `readinessProbe` đúng C2.2; Deployment **không** khai `replicas` | **WP-B** | đọc manifest; `kubectl get deploy -o yaml` ở P4.5 | chưa chạy |
| AC-5b | gate chặn `.skip`/`.only`/`it.todo` **mới thêm**; gate chặn probe nằm trong `prerender-manifest.json` | **WP-C** | chạy `gates.sh` với một test cố tình `.skip` → phải fail | chưa chạy |
| AC-8 | xác minh chạy thật: pod Ready, 2 probe, SIGTERM→503, `rollout undo` | **Integrator** | log thô trong `integration-report.md` | chưa chạy |

### Giới hạn đã biết, không được tính là đã kiểm

- **AC-4 không đóng được trong P3.** Không có cluster thì `--dry-run=client` không chạy, và nhánh bỏ qua của gate **không kiểm gì** (không bắt được sai `apiVersion`, sai cấp field, `selector` lệch, probe sai path — tất cả đều là YAML hợp lệ). Bằng chứng duy nhất là `--dry-run=server` ở P4.5.
- **`NEXT_PUBLIC_API_BASE`**: deploy có thể Ready với cả hai probe xanh mà frontend **không gọi được backend**. Cần người duyệt (spec mục 6, điểm 4).
- **CI không chạy gì** cho `deploy/**` và `.github/scripts/**` (paths-filter không khớp) ⇒ PR của WP-B/WP-C xanh là **dương tính giả**. Bằng chứng là thủ công.

## Cập nhật trạng thái sau P4 / P4.5 (một phần) / P5

Trạng thái `chưa chạy` ở các bảng trên là ảnh chụp tại **P2**, khi ba file test đã được viết nhưng
WP-A chưa hiện thực gì. Để nguyên để giữ dấu vết. Bảng dưới là trạng thái **hiện tại**; chỉ liệt kê
dòng đã đổi.

| AC | Trạng thái | Bằng chứng |
|---|---|---|
| AC-1, AC-2 (toàn bộ dòng unit test) | ✅ đạt | `44 file / 1086 test` pass; `gates.sh` exit 0. Per-file coverage: `route.ts` healthz 10/10, readyz 16/16, `readiness.ts` 22/22 dòng |
| AC-1, AC-2 "xuất hiện là dynamic trong output build" | ✅ đạt | `next build` in `ƒ (Dynamic)` cho cả hai probe; thêm check C4.3 của `gates.sh` xác nhận hai đường dẫn **không** nằm trong `prerender-manifest.json` |
| AC-2b | ✅ **đạt bằng signal POSIX thật** | `docker kill -s SIGTERM` trong container Linux: readyz ⇒ 503 **dưới nửa giây** (77 ms lần đo đầu, ~316 ms khi P6 đo lại — xem `integration-report.md` §8), healthz **giữ 200**, ExitCode **0** ở t=5129 ms; SIGTERM thứ hai ở t=2s dưới `--init` không cắt ngắn drain. Thêm SIGINT: 503 trong ~1s, ExitCode 0. Chi tiết `integration-report.md` §7.1 |
| AC-3 | ✅ đạt | `docker compose -f deploy/compose/docker-compose.yml config` parse được; `healthcheck` gọi `/api/healthz` với bốn tham số tường minh |
| AC-5 | ✅ đạt | `bash .github/scripts/gates.sh` chạy đủ 4 lệnh, exit 0, chạy được trên Git Bash Windows |
| AC-5b | ✅ đạt, đã mở rộng (2 đợt) | **`bb4facb`:** thêm `.spec.*`, `.skipIf`/`.runIf`/`.concurrent.skip`, và marker **chưa commit** (tầng diff với working tree). Bản đầu của dòng này còn ghi "file test ở cấp gốc" — **sai**: `**` của `:(glob)` khớp cả 0 thư mục nên vùng đó chưa từng mù (P6 thử thực nghiệm). **Đợt 2 (sau P6):** P6 tìm ra 3 lỗ còn sót — `.skip.each`/`.only.each`/`.todo.each`, toàn bộ `tests/e2e/**` (Playwright, có cả `test.fixme`), và file test **mới chưa `git add`** (`git diff HEAD` không thấy file untracked). Đã vá cả ba; regex kiểm trên 18 mẫu (bắt 12/12, không bắt nhầm 6/6); tiêm `test.fixme` vào file e2e untracked + `.skip.each` vào file đã track → gate exit 1 nêu đúng 2 dòng, trong khi bộ test vẫn xanh; hoàn nguyên → exit 0 |
| AC-6 | ✅ đạt | `docs/workflow/deploy-k8s-web/runbook.md`: build → đưa image vào cluster → apply → kiểm pod Ready → log/port-forward → rollback, kèm §6 liệt kê từng thứ **chưa** verify được và vì sao |
| AC-7 | ✅ đạt | `1016 → 1086` test, không test nào bị sửa/xoá/skip; `git diff --name-only` khớp bảng ownership C5 |
| AC-4, AC-4b, AC-4c | ❌ **vẫn chưa chạy** | `kubectl config current-context` ⇒ `current-context is not set`. Kubernetes chưa bật trong Docker Desktop, nên `--dry-run=server` chưa chạy được. `gates.sh` **bỏ qua** C4.4 chứ không fail — một gate xanh ở đây **không** nói gì về `deploy/k8s/` |
| AC-8 | ⚠️ một phần | Đã chạy thật ở tầng **container** (bảng AC-2b). Phần **cluster** — pod Ready, 2 probe qua kubelet, `rollout undo` — vẫn chưa chạy, cùng lý do trên |

### Khoảng trống gate ở tầng bundle (P5 Challenge, F1) — đã đóng

Hồi quy ở §4.1 của `integration-report.md` — nhánh 503 bị optimizer **xoá khỏi bản production** —
tái hiện được với **cả 4 gate xanh** và C4.2, C4.3 cũng xanh, nên trước đó lớp bảo vệ duy nhất chỉ là
comment trong `readiness.ts`. Đã thêm **C4.5** vào `gates.sh`: grep `.next/server/**` tìm marker mà
minifier không đổi tên được.

| AC | Trạng thái | Bằng chứng |
|---|---|---|
| AC-5b (mở rộng: tầng bundle) | ✅ đạt, **kiểm cả hai chiều** | Tiêm lại hồi quy (cờ ready về `let` ở tầng module) → lint, typecheck, **44 file / 1086 test**, `next build`, C4.2, C4.3 **đều xanh** trong khi bundle readyz **không còn `not-ready` và không còn `503`**; chỉ C4.5 fail, exit 1, nêu đúng 4 marker mất. Hoàn nguyên → exit 0 |

Marker được chọn vì **tồn tại được qua minify**: `__eventflowWebReadiness__` là khoá **chuỗi** trên
`globalThis` (terser không rename chuỗi), cộng `not-ready` / `503` / `SIGTERM` / `SIGINT` là literal.
Thêm một bất biến ngược: `503` **không** được có trong bundle `healthz` — liveness trả 503 nghĩa là
logic readiness đã lọt vào nó, và k8s sẽ **restart** pod đang drain thay vì chỉ rút endpoint.

`grep -c setReady` **không** dùng được: tên hàm bị minify nên nó ra 0 ngay cả khi code hoàn toàn
đúng — đúng loại check sinh ra báo động giả rồi bị tắt đi.
