/** `GET /api/readyz` — readiness probe (AC-2, contract C1).
 *
 * Dich trang thai cua `@/lib/readiness` thanh ma HTTP. Khong tu quyet dinh gi, khong goi
 * dependency ngoai.
 *
 * TRANG THAI NAM O MA HTTP, khong o field trong body. Kubernetes chi doc ma trang thai: tra
 * `200` kem `{"ready":false}` thi no coi la san sang va DAY TRAFFIC vao mot pod chua san sang.
 * Vi vay body co `status: "ready" | "not-ready"` de nguoi doc log hieu duoc, nhung thu that su
 * dieu khien Service endpoints la 200 vs 503. */
import { isReady, uptimeMs } from "@/lib/readiness";

/** Cung ly do nhu `/api/healthz`: probe bi prerender se dong bang tu luc build va luon xanh.
 * Voi readyz thi con tai hon — nhanh `503` se khong bao gio chay duoc. */
export const dynamic = "force-dynamic";

/** 200 + `{"status":"ready",...}` khi san sang; 503 + `{"status":"not-ready",...}` khi chua.
 *
 * `uptimeMs` di kem o CA HAI nhanh: khi dang drain, "bao lau roi" la thong tin can nhat de doc
 * log — no cho biet pod vua khoi dong hay da chay lau roi moi nhan SIGTERM. */
export function GET(): Response {
  const ready = isReady();

  const body = {
    status: ready ? "ready" : "not-ready",
    uptimeMs: uptimeMs(),
  };

  return new Response(JSON.stringify(body), {
    status: ready ? 200 : 503,
    headers: {
      "content-type": "application/json",
      "cache-control": "no-store",
    },
  });
}
