// @vitest-environment node
/**
 * Test cho `GET /api/readyz` — AC-2 (spec muc 4), contract C1.
 *
 * Route handler duoc goi TRUC TIEP (import `GET` tu module route), khong dung
 * server, khong dung network.
 *
 * Diem song con cua readiness, theo C1 va spec muc 3.2:
 *
 *   1. TRANG THAI PHAI NAM O MA HTTP, khong phai o mot field trong body.
 *      Kubernetes chi doc ma trang thai: tra `200` kem `{"ready":false}` thi no
 *      coi la san sang va DAY TRAFFIC vao mot pod chua san sang. Vi vay test
 *      assert `200` khi ready va `503` khi chua ready, va assert chuoi `status`
 *      trong body khop voi ma o CA HAI truong hop.
 *   2. `uptimeMs` la so NGUYEN KHONG AM, mili-giay ke tu khi module khoi tao,
 *      va khong duoc di lui giua hai lan goi.
 *   3. KHONG DUOC PRERENDER TINH: `export const dynamic = "force-dynamic"` +
 *      `cache-control: no-store`. Probe bi prerender se tra ve ket qua dong
 *      bang tu luc build nen luon xanh ke ca khi tien trinh da hong.
 *
 * Gia dinh ve `@/lib/readiness` (xem bao cao buoc Test Design): module do export
 * `isReady()`, `setReady(ready)` va `uptimeMs()`. Test lai trang thai 503 bang
 * `setReady(false)` thay vi mock module, de khong rang buoc ban hien thuc phai
 * goi ham nao theo thu tu nao.
 *
 * Trang thai dung o buoc Test Design: `./route` va `@/lib/readiness` CHUA TON
 * TAI, test se bao "module not found" cho den khi WP-A hien thuc.
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { setReady } from "@/lib/readiness";

import { GET, dynamic } from "./route";

const READYZ_URL = "http://127.0.0.1:3000/api/readyz";
const FIXED_NOW = Date.UTC(2026, 0, 15, 3, 0, 0);

/** Route handler cua Next co the khai bao `GET()` hoac `GET(request)`. Cast mot
 * lan o day de test khong phu thuoc vao chu ky ham cua ban hien thuc. */
type RouteHandler = (request?: Request) => Promise<Response> | Response;

type ReadyzBody = { status: string; uptimeMs: number };

async function callReadyz(handler: unknown = GET): Promise<Response> {
  return await (handler as RouteHandler)(new Request(READYZ_URL));
}

async function readBody(response: Response): Promise<ReadyzBody> {
  return (await response.json()) as ReadyzBody;
}

afterEach(() => {
  setReady(true);
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("GET /api/readyz — khi da san sang (AC-2, C1)", () => {
  it("tra 200", async () => {
    const response = await callReadyz();

    expect(response.status).toBe(200);
  });

  it("body co status = \"ready\"", async () => {
    const body = await readBody(await callReadyz());

    expect(body.status).toBe("ready");
  });

  it("body dung hinh dang cua C1: chi gom status va uptimeMs", async () => {
    const body = await readBody(await callReadyz());

    expect(Object.keys(body).sort()).toEqual(["status", "uptimeMs"]);
    expect(typeof body.status).toBe("string");
    expect(typeof body.uptimeMs).toBe("number");
  });

  it("header cache-control chua no-store", async () => {
    const response = await callReadyz();

    expect(response.headers.get("cache-control")).toContain("no-store");
  });

  it("tra ve Response chuan", async () => {
    const response = await callReadyz();

    expect(response).toBeInstanceOf(Response);
  });
});

describe("GET /api/readyz — khi CHUA san sang (AC-2, C1)", () => {
  it("tra 503, khong phai 200 kem co trong body", async () => {
    setReady(false);

    const response = await callReadyz();

    expect(response.status).toBe(503);
    expect(response.status).not.toBe(200);
  });

  it("body co status = \"not-ready\"", async () => {
    setReady(false);

    const body = await readBody(await callReadyz());

    expect(body.status).toBe("not-ready");
  });

  it("van co uptimeMs la so nguyen khong am", async () => {
    setReady(false);

    const body = await readBody(await callReadyz());

    expect(Number.isInteger(body.uptimeMs)).toBe(true);
    expect(body.uptimeMs).toBeGreaterThanOrEqual(0);
  });

  it("header cache-control van chua no-store", async () => {
    setReady(false);

    const response = await callReadyz();

    expect(response.headers.get("cache-control")).toContain("no-store");
  });

  it("body van dung hinh dang cua C1: chi gom status va uptimeMs", async () => {
    setReady(false);

    const body = await readBody(await callReadyz());

    expect(Object.keys(body).sort()).toEqual(["status", "uptimeMs"]);
  });
});

describe("GET /api/readyz — ma HTTP va chuoi status luon khop (AC-2, C1)", () => {
  it.each([
    [true, 200, "ready"],
    [false, 503, "not-ready"],
  ])(
    "setReady(%s) -> ma %i kem status \"%s\"",
    async (ready, expectedStatus, expectedBodyStatus) => {
      setReady(ready as boolean);

      const response = await callReadyz();
      const body = await readBody(response);

      expect(response.status).toBe(expectedStatus);
      expect(body.status).toBe(expectedBodyStatus);
    },
  );

  it("KHONG dung field boolean `ready` trong body de bieu dien trang thai", async () => {
    setReady(false);

    const body = (await readBody(await callReadyz())) as ReadyzBody & { ready?: boolean };

    expect(body.ready).toBeUndefined();
    expect("ready" in body).toBe(false);
  });

  it("ha xuong not-ready roi dua len lai -> quay ve 200 (khong dinh trang thai)", async () => {
    setReady(false);
    expect((await callReadyz()).status).toBe(503);

    setReady(true);
    const response = await callReadyz();
    const body = await readBody(response);

    expect(response.status).toBe(200);
    expect(body.status).toBe("ready");
  });
});

describe("GET /api/readyz — uptimeMs (AC-2, C1)", () => {
  it("la so nguyen khong am", async () => {
    const body = await readBody(await callReadyz());

    expect(Number.isInteger(body.uptimeMs)).toBe(true);
    expect(body.uptimeMs).toBeGreaterThanOrEqual(0);
    expect(Number.isFinite(body.uptimeMs)).toBe(true);
  });

  it("khong di lui giua hai request lien tiep", async () => {
    const first = await readBody(await callReadyz());
    const second = await readBody(await callReadyz());

    expect(second.uptimeMs).toBeGreaterThanOrEqual(first.uptimeMs);
  });

  it("tang dung theo dong ho khi thoi gian chay (fake timers)", async () => {
    // Nap lai ca route va readiness DUOI dong ho gia, de moc khoi tao module va
    // moc doc deu cung mot nguon thoi gian.
    vi.useFakeTimers();
    vi.setSystemTime(FIXED_NOW);
    vi.resetModules();

    const mod = await import("./route");

    const before = await readBody(await callReadyz(mod.GET));
    vi.advanceTimersByTime(3_000);
    const after = await readBody(await callReadyz(mod.GET));

    expect(after.uptimeMs - before.uptimeMs).toBe(3_000);
    expect(after.uptimeMs).toBeGreaterThan(before.uptimeMs);
  });
});

describe("GET /api/readyz — khong duoc prerender tinh (AC-2, spec 3.3)", () => {
  it("module route export dynamic = \"force-dynamic\"", () => {
    expect(dynamic).toBe("force-dynamic");
  });

  it("dynamic khong phai gia tri tinh nao khac", () => {
    expect(dynamic).not.toBe("force-static");
    expect(dynamic).not.toBe("auto");
    expect(dynamic).not.toBe("error");
  });
});
