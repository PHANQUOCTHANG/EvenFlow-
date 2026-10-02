/** `GET /api/healthz` — liveness probe (AC-1, contract C1).
 *
 * Tra loi DUY NHAT mot cau hoi: "tien trinh nay con chay khong?". Con chay thi 200.
 *
 * KHONG goi Redis, database, gateway, hay bat ky dependency ngoai nao — ke ca readiness.
 * Liveness fail => kubelet GIET va KHOI DONG LAI pod. Neu probe nay phu thuoc Redis thi Redis
 * chet se keo theo restart lien tuc toan bo pod web: khong sua duoc Redis, ma con mat luon kha
 * nang hien thi trang loi tu te. Day la loi kinh dien cua probe, khong phai gia thiet.
 *
 * Vi the module nay co y KHONG import `@/lib/readiness`: hai probe la hai khai niem khac nhau,
 * va mot import "cho tien" o day la buoc dau de liveness bi gan vao trang thai drain. */

/** Bat buoc, khong phai toi uu. Next prerender tinh moi route hien co; probe bi prerender se
 * tra ve ket qua DONG BANG TU LUC BUILD, tuc luon xanh ke ca khi tien trinh da hong — che do
 * loi te nhat co the. Bay nay da gap that trong repo (commit 9607fa6, dong ho dem nguoc). */
export const dynamic = "force-dynamic";

/** Luon 200 voi body dung nguyen van `{"status":"ok"}` (C1: khong them field).
 *
 * `cache-control: no-store` di cung `force-dynamic`: mot cai chan cache o tang build, cai kia
 * chan cache o tang HTTP (proxy, ingress, CDN). Thieu cai thu hai thi probe van co the doc
 * duoc mot ban luu. */
export function GET(): Response {
  return new Response(JSON.stringify({ status: "ok" }), {
    status: 200,
    headers: {
      "content-type": "application/json",
      "cache-control": "no-store",
    },
  });
}
