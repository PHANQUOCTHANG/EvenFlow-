/** Graceful shutdown o tang ung dung (AC-2b, contract C1.1b).
 *
 * Next goi `register()` dung mot lan khi server khoi dong (`next-server.js` tu chan bang co
 * `registeredInstrumentation`). Handler SIGTERM o day lam DU BA VIEC, DUNG THU TU:
 *
 *   1. `setReady(false)`  -> `/api/readyz` tra 503 ngay lap tuc
 *   2. cho `DRAIN_MS`     -> kubelet / endpoints controller kip rut endpoint cua pod
 *   3. `process.exit(0)`
 *
 * PHU THUOC BAT BUOC: image phai co `NEXT_MANUAL_SIG_HANDLE=1` (Integrator dat trong
 * `deploy/docker/web.Dockerfile`). Da kiem `next/dist/server/lib/start-server.js:255`: khong co
 * bien do thi Next TU dang ky SIGINT/SIGTERM, va handler cua no `server.close()` roi
 * `process.exit(0)` NGAY. Hau qua: sau SIGTERM server da dong nen `/api/readyz` khong tra 503
 * duoc nua — client nhan ECONNREFUSED, va nhanh 503 thanh code chet trong van hanh.
 * Neu P4.5 thay ECONNREFUSED thay vi 503 thi nguyen nhan gan nhu chac chan la thieu bien do.
 *
 * `setReady(false)` o day la call site DUY NHAT cua ham do trong ung dung. Do la cach tinh
 * "mot chieu" cua C1.1 duoc bao dam — bang mot call site, khong bang latch trong `readiness.ts`. */
import { setReady } from "./lib/readiness";

/** 5 giay, chot trong C1.1b. Day la CUA SO DRAIN, khong phai `terminationGracePeriodSeconds`
 * (30s) — cai sau chi la bien ngoai truoc khi kubelet gui SIGKILL.
 *
 * Gioi han phai ghi vao runbook: request dang bay qua 5 giay se bi cat. Khong tang bua: cua so
 * drain con phai nho hon grace period, va `readinessProbe` cua C2.2 chay moi 2 giay nen 5 giay
 * du cho it nhat hai nhip probe doc duoc 503. */
const DRAIN_MS = 5_000;

/** Chan goi hai lan. Kubelet co the gui them SIGTERM, va mot container runtime cung co the
 * gui lai khi grace period gan het. Khong co co nay thi tin hieu thu hai se hen mot
 * `process.exit(0)` thu hai va cat ngan cua so drain dang chay. */
let draining = false;

function handleSigterm(): void {
  if (draining) return;
  draining = true;

  // Buoc 1 phai dong bo va dung truoc moi thu khac: tu day tro di readyz tra 503.
  setReady(false);

  // Buoc 2 + 3. KHONG `unref()` timer nay: unref se cho phep event loop thoat som va bo qua
  // ca cua so drain, dung thu dang can doi.
  setTimeout(() => {
    process.exit(0);
  }, DRAIN_MS);
}

/** Next chay file nay ca o runtime `nodejs` va `edge`. Chi dang ky o `nodejs`: edge runtime
 * khong co `process.on` / `process.exit`, nen khong guard la crash luc khoi dong.
 *
 * Dung `process.on`, KHONG dung `process.once`. Hai ly do, ly do thu hai moi nghiem trong:
 *
 * 1. `once` thao listener ngay sau lan goi dau, nen nhanh `if (draining) return` o tren
 *    khong bao gio dat toi — tuc lop chan tin hieu lap ma comment cua no mo ta la code chet.
 *
 * 2. Khi listener JS cuoi cung cua mot signal bi thao, libuv tra signal ve SIG_DFL. Nghia la
 *    trong cua so drain 5 giay, SIGTERM thu hai KHONG con handler nao: tien trinh chet bang
 *    default disposition voi exit 143, drain bi cat giua, va `kubectl describe pod` ghi
 *    `Exit Code: 143` thay vi `0` nhu AC-2b doi. Chuyen nay chi lo ra khi node KHONG phai
 *    PID 1 (`docker run --init`, compose `init: true`, tini, hay bat ky entrypoint wrapper
 *    nao) — vi PID 1 duoc kernel bo qua SIG_DFL. Tuc la mot hoi quy an, chi hien ra khi doi
 *    cach chay container.
 *
 * `on` khong tich luy listener vi Next chi goi `register()` mot lan. */
export function register(): void {
  if (process.env.NEXT_RUNTIME !== "nodejs") return;

  process.on("SIGTERM", handleSigterm);

  // `NEXT_MANUAL_SIG_HANDLE=1` tat handler cua Next cho CA SIGINT lan SIGTERM
  // (start-server.js dang ky ca hai trong cung mot `if`). Neu chi dang ky lai
  // SIGTERM thi SIGINT khong con handler nao -> default disposition cua POSIX ->
  // tien trinh chet ngay, khong `setReady(false)`, khong drain, exit 130.
  // k8s khong anh huong (kubelet gui SIGTERM), nhung `docker compose up` o
  // foreground roi Ctrl+C thi mat graceful shutdown — hoi quy do chinh viec dat
  // co nay tao ra.
  process.on("SIGINT", handleSigterm);
}
