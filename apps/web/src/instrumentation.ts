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
 * `process.once` thay vi `process.on` de dang ky tu no khong tich luy neu Next doi cach goi;
 * co `draining` van la lop chan chinh cho tin hieu lap. */
export function register(): void {
  if (process.env.NEXT_RUNTIME !== "nodejs") return;

  process.once("SIGTERM", handleSigterm);
}
