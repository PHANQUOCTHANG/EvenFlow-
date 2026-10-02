# Ma trận truy vết — slice deploy web lên Kubernetes

Nguồn: `spec.md` (AC-1 … AC-7), `contracts.md` (C1 … C6), `work-packages.md` (WP-A/B/C).

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
| AC-4 | Manifest k8s: Namespace/Deployment/Service/Ingress/HPA, probe trỏ đúng C1, `resources.requests` + `limits`, `securityContext` non-root drop ALL, image `ghcr.io/<owner>/eventflow-web`, `minReplicas` >= 2 | **WP-B** sở hữu (`deploy/k8s/**`) — kiểm bằng **lệnh**, không có unit test | `kubectl apply --dry-run=client -f deploy/k8s/` cho **mọi** file | chưa chạy |
| AC-5 | Script gate chạy đủ `lint`, `typecheck`, `test:coverage`, `build`; fail thì exit khác 0 và nêu rõ bước; kiểm diff test `.skip`/`.only`/`it.todo`; validate `deploy/k8s/**`; chạy được trên Git Bash Windows | **WP-C** sở hữu (`.github/scripts/gates.sh`, `Makefile`) — kiểm bằng **lệnh**, không có unit test | `make gates` / `bash .github/scripts/gates.sh` (chạy từ gốc repo) | chưa chạy |
| AC-6 | Runbook: lệnh cụ thể để build image, load vào cluster, apply, kiểm pod ready, xem log, rollback; ghi rõ phần **chưa** verify được và vì sao | **Integrator** sở hữu (`docs/workflow/deploy-k8s-web/runbook.md`) — kiểm bằng **đọc + chạy lệnh**, không có unit test | Đối chiếu từng lệnh trong runbook khi Docker Desktop chạy | chưa chạy |
| AC-7 | Không hồi quy: toàn bộ test hiện có vẫn pass; không WP nào sửa file ngoài ownership (C5) | **Integrator** kiểm sau merge — kiểm bằng **lệnh**, không có unit test | `npm run test:coverage` (apps/web) + `git diff --name-only` đối chiếu bảng C5 | chưa chạy |
