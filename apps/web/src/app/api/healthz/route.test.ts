// @vitest-environment node
/**
 * Test cho `GET /api/healthz` — AC-1 (spec muc 4), contract C1.
 *
 * Route handler duoc goi TRUC TIEP (import `GET` tu module route), khong dung
 * server, khong dung network. Handler tra ve `Response` chuan nen assert duoc
 * tren `status`, `headers` va `json()`.
 *
 * Ba rang buoc quan trong, theo dung ly do nghiep vu o spec muc 3.1 va C1:
 *
 *   1. LIVENESS KHONG CHAM DEPENDENCY NGOAI. Liveness fail => kubelet GIET va
 *      KHOI DONG LAI pod. Neu probe nay phu thuoc Redis/gateway/db thi Redis
 *      chet se keo theo restart lien tuc pod web: khong sua duoc Redis, ma con
 *      mat luon kha nang hien thi trang loi tu te. Test chan bang cach stub
 *      `globalThis.fetch` + `XMLHttpRequest` roi assert KHONG ai goi, va bang
 *      cach cho `fetch` nem loi ma handler van phai tra 200.
 *   2. KHONG BAO GIO TRA MA KHAC 200 KHI TIEN TRINH CON CHAY (C1). Ke ca khi
 *      readiness da bi ha xuong not-ready thi liveness van 200 — hai probe nay
 *      la hai khai niem khac nhau.
 *   3. KHONG DUOC PRERENDER TINH. Next prerender tinh moi route hien co; probe
 *      bi prerender se tra ve ket qua DONG BANG TU LUC BUILD, tuc luon xanh ke
 *      ca khi tien trinh da hong. Bat buoc `export const dynamic =
 *      "force-dynamic"` + `cache-control: no-store`.
 *
 * Trang thai dung o buoc Test Design: module `./route` CHUA TON TAI, test se
 * bao "module not found" cho den khi WP-A hien thuc.
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { setReady } from "@/lib/readiness";

import { GET, dynamic } from "./route";

const HEALTHZ_URL = "http://127.0.0.1:3000/api/healthz";

/** Route handler cua Next co the khai bao `GET()` hoac `GET(request)`. Cast mot
 * lan o day de test khong phu thuoc vao chu ky ham cua ban hien thuc. */
type RouteHandler = (request?: Request) => Promise<Response> | Response;

async function callHealthz(): Promise<Response> {
  const handler = GET as unknown as RouteHandler;
  return await handler(new Request(HEALTHZ_URL));
}

afterEach(() => {
  // Liveness khong duoc phu thuoc readiness, nhung test co doi state nen phai
  // tra lai mac dinh de khong ro ri sang test khac.
  setReady(true);
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("GET /api/healthz — ma trang thai va body (AC-1, C1)", () => {
  it("tra 200 khi tien trinh con chay", async () => {
    const response = await callHealthz();

    expect(response.status).toBe(200);
  });

  it("tra ve Response chuan (assert duoc bang status/headers/json)", async () => {
    const response = await callHealthz();

    expect(response).toBeInstanceOf(Response);
    expect(typeof response.status).toBe("number");
  });

  it("body dung nguyen van { status: \"ok\" } theo C1", async () => {
    const response = await callHealthz();

    await expect(response.json()).resolves.toEqual({ status: "ok" });
  });

  it("goi nhieu lan van 200 va body khong doi (probe bi goi lien tuc)", async () => {
    for (let i = 0; i < 3; i += 1) {
      const response = await callHealthz();

      expect(response.status).toBe(200);
      await expect(response.json()).resolves.toEqual({ status: "ok" });
    }
  });

  it("khong bao gio tra ma khac 200 (ke ca khi readiness da not-ready)", async () => {
    setReady(false);

    const response = await callHealthz();

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ status: "ok" });
  });
});

describe("GET /api/healthz — khong duoc cache / khong duoc prerender (AC-1, C1)", () => {
  it("header cache-control chua no-store", async () => {
    const response = await callHealthz();

    expect(response.headers.get("cache-control")).toContain("no-store");
  });

  it("cache-control chua no-store tren MOI response, khong chi lan dau", async () => {
    const first = await callHealthz();
    const second = await callHealthz();

    expect(first.headers.get("cache-control")).toContain("no-store");
    expect(second.headers.get("cache-control")).toContain("no-store");
  });

  it("module route export dynamic = \"force-dynamic\" (neu prerender thi probe dong bang tu luc build)", () => {
    expect(dynamic).toBe("force-dynamic");
  });
});

describe("GET /api/healthz — KHONG goi dependency ngoai (AC-1, spec 3.1)", () => {
  it("khong goi globalThis.fetch khi xu ly request", async () => {
    const fetchSpy = vi.fn(() => {
      throw new Error("liveness khong duoc goi fetch");
    });
    vi.stubGlobal("fetch", fetchSpy);

    const response = await callHealthz();

    expect(fetchSpy).not.toHaveBeenCalled();
    expect(response.status).toBe(200);
  });

  it("khong goi XMLHttpRequest khi xu ly request", async () => {
    const xhrOpen = vi.fn();
    const xhrSend = vi.fn();
    vi.stubGlobal(
      "XMLHttpRequest",
      class {
        open = xhrOpen;
        send = xhrSend;
        setRequestHeader = vi.fn();
      },
    );

    await callHealthz();

    expect(xhrOpen).not.toHaveBeenCalled();
    expect(xhrSend).not.toHaveBeenCalled();
  });

  it("fetch bi loi (dependency ngoai chet) van KHONG lam liveness fail", async () => {
    vi.stubGlobal("fetch", () => Promise.reject(new Error("redis down")));

    const response = await callHealthz();

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ status: "ok" });
  });

  it("nap module route cung khong gay request nao (khong co side effect luc import)", async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    vi.resetModules();

    const mod = await import("./route");

    expect(typeof mod.GET).toBe("function");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("nap module route va goi handler moi nap cung khong fetch", async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    vi.resetModules();

    const mod = await import("./route");
    const handler = mod.GET as unknown as RouteHandler;
    const response = await handler(new Request(HEALTHZ_URL));

    expect(response.status).toBe(200);
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
