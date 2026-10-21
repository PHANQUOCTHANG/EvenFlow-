/**
 * Contract test: lib/api-client.ts (AC-3, AC-4, AC-5; spec muc 5 edge cases).
 * fetch duoc mock o bien; kiem URL, method, header, body dung contracts.md.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError, apiFetch, readProblem } from "@/lib/api-client";
import { getDeviceFingerprint } from "@/lib/device";
import {
  BASE,
  expectNoIdentityOrCredentials,
  installFetch,
  json,
  problem,
  resetEnv,
  seedSession,
  setupEnv,
} from "@/test-utils/fetch-mock";

beforeEach(setupEnv);
afterEach(resetEnv);

describe("apiFetch co ban (AC-3)", () => {
  it("them apiBase() vao truoc path", async () => {
    const { calls } = installFetch(() => json({}));
    await apiFetch("/v1/payments/methods");
    expect(calls[0].url).toBe(`${BASE}/v1/payments/methods`);
  });

  it("co phien -> gui Authorization: Bearer <access>", async () => {
    seedSession("tok-abc");
    const { calls } = installFetch(() => json({}));
    await apiFetch("/v1/orders/o1");
    expect(calls[0].headers.get("Authorization")).toBe("Bearer tok-abc");
  });

  it("khong phien -> khong co header Authorization", async () => {
    const { calls } = installFetch(() => json({}));
    await apiFetch("/v1/orders/o1");
    expect(calls[0].headers.has("Authorization")).toBe(false);
  });

  it("auth:false -> khong gui Bearer du co phien", async () => {
    seedSession("tok-abc");
    const { calls } = installFetch(() => json({}));
    await apiFetch("/api/v1/auth/login", { method: "POST", auth: false });
    expect(calls[0].headers.has("Authorization")).toBe(false);
  });

  it("khong bao gio dung credentials include va khong gui X-Identity-*", async () => {
    seedSession();
    const { calls } = installFetch(() => json({}));
    await apiFetch("/v1/events/e1/holds", { method: "POST", body: "{}" });
    expectNoIdentityOrCredentials(calls[0]);
  });

  it("giu nguyen method, body va header cua caller", async () => {
    const { calls } = installFetch(() => json({}));
    await apiFetch("/v1/events/e1/holds", {
      method: "POST",
      headers: { "Idempotency-Key": "k1", "X-Queue-Token": "qt", "Content-Type": "application/json" },
      body: JSON.stringify({ a: 1 }),
    });
    expect(calls[0].method).toBe("POST");
    expect(calls[0].headers.get("Idempotency-Key")).toBe("k1");
    expect(calls[0].headers.get("X-Queue-Token")).toBe("qt");
    expect(calls[0].body).toEqual({ a: 1 });
  });

  it("loi khong phai 401 (404) tra ve nguyen Response, khong nem va khong refresh", async () => {
    seedSession();
    const { calls } = installFetch(() => problem(404, "/problems/not-found"));
    const res = await apiFetch("/v1/orders/o1");
    expect(res.status).toBe(404);
    expect(calls).toHaveLength(1);
  });
});

describe("refresh don le (AC-4)", () => {
  function expiredThenOk() {
    return installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        return json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2" });
      }
      return c.headers.get("Authorization") === "Bearer new"
        ? json({ ok: true })
        : problem(401, "/problems/token-expired");
    });
  }

  it("401 token-expired -> POST /api/v1/auth/refresh {refresh_token, device_fingerprint} dung 1 lan", async () => {
    seedSession("old", "r1");
    const { calls } = expiredThenOk();
    await apiFetch("/v1/orders/o1");
    const refresh = calls.filter((c) => c.path === "/api/v1/auth/refresh");
    expect(refresh).toHaveLength(1);
    expect(refresh[0].url).toBe(`${BASE}/api/v1/auth/refresh`);
    expect(refresh[0].method).toBe("POST");
    expect(refresh[0].body).toEqual({ refresh_token: "r1", device_fingerprint: getDeviceFingerprint() });
    expectNoIdentityOrCredentials(refresh[0]);
  });

  it("luu cap token moi va retry request goc dung 1 lan voi Bearer moi", async () => {
    seedSession("old", "r1");
    const { calls } = expiredThenOk();
    const res = await apiFetch("/v1/orders/o1");
    expect(res.status).toBe(200);
    expect(localStorage.getItem("ef_token")).toBe("new");
    expect(localStorage.getItem("ef_refresh")).toBe("r2");
    const originals = calls.filter((c) => c.path === "/v1/orders/o1");
    expect(originals).toHaveLength(2);
    expect(originals[1].headers.get("Authorization")).toBe("Bearer new");
  });

  it("retry giu nguyen method, body va Idempotency-Key", async () => {
    seedSession("old", "r1");
    const { calls } = expiredThenOk();
    await apiFetch("/v1/events/e1/payments", {
      method: "POST",
      headers: { "Idempotency-Key": "idem-9", "Content-Type": "application/json" },
      body: JSON.stringify({ amount: 5 }),
    });
    const originals = calls.filter((c) => c.path === "/v1/events/e1/payments");
    expect(originals).toHaveLength(2);
    expect(originals[1].method).toBe("POST");
    expect(originals[1].headers.get("Idempotency-Key")).toBe("idem-9");
    expect(originals[1].body).toEqual({ amount: 5 });
  });

  it("request 401 lan hai (sau khi da refresh) van het han: KHONG refresh lan 2", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        return json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2" });
      }
      return problem(401, "/problems/token-expired");
    });
    await apiFetch("/v1/orders/o1").catch(() => undefined);
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
    expect(calls.filter((c) => c.path === "/v1/orders/o1")).toHaveLength(2);
  });

  it("401 het han o dot sau van refresh lai (single-flight khong ket vinh vien)", async () => {
    seedSession("old", "r1");
    let wave = 1;
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        return json({ id: "u-1", role: "buyer", access_token: `new${wave}`, refresh_token: `r${wave + 1}` });
      }
      return c.headers.get("Authorization") === `Bearer new${wave}`
        ? json({ ok: true })
        : problem(401, "/problems/token-expired");
    });
    await apiFetch("/v1/orders/o1");
    wave = 2;
    await apiFetch("/v1/orders/o2");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(2);
    expect(localStorage.getItem("ef_token")).toBe("new2");
  });
});

describe("refresh dong thoi (AC-4, edge case: concurrent 401)", () => {
  it("3 request cung 401 -> dung 1 lan refresh, ca 3 deu retry thanh cong", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch(async (c) => {
      if (c.path === "/api/v1/auth/refresh") {
        await new Promise((r) => setTimeout(r, 10));
        return json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2" });
      }
      return c.headers.get("Authorization") === "Bearer new"
        ? json({ path: c.path })
        : problem(401, "/problems/token-expired");
    });
    const results = await Promise.all([apiFetch("/v1/orders/a"), apiFetch("/v1/orders/b"), apiFetch("/v1/orders/c")]);
    expect(results.map((r) => r.status)).toEqual([200, 200, 200]);
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
    for (const p of ["a", "b", "c"]) {
      const mine = calls.filter((c) => c.path === `/v1/orders/${p}`);
      expect(mine).toHaveLength(2);
      expect(mine[1].headers.get("Authorization")).toBe("Bearer new");
    }
  });

  it("refresh that bai khi dong thoi: ca 3 reject AUTH_REQUIRED, van chi 1 lan refresh", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch(async (c) => {
      if (c.path === "/api/v1/auth/refresh") {
        await new Promise((r) => setTimeout(r, 10));
        return problem(401, "/problems/unauthorized");
      }
      return problem(401, "/problems/token-expired");
    });
    const settled = await Promise.allSettled([
      apiFetch("/v1/orders/a"),
      apiFetch("/v1/orders/b"),
      apiFetch("/v1/orders/c"),
    ]);
    expect(settled.every((s) => s.status === "rejected")).toBe(true);
    for (const s of settled) {
      expect((s as PromiseRejectedResult).reason).toMatchObject({ code: "AUTH_REQUIRED" });
    }
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
  });
});

describe("refresh that bai (AC-4)", () => {
  it.each([401, 400])("refresh tra %i -> xoa phien va nem ApiError AUTH_REQUIRED", async (status) => {
    seedSession("old", "r1");
    installFetch((c) =>
      c.path === "/api/v1/auth/refresh"
        ? problem(status, "/problems/unauthorized")
        : problem(401, "/problems/token-expired"),
    );
    const err = await apiFetch("/v1/orders/o1").then(
      () => null,
      (e: unknown) => e,
    );
    expect(err).toBeInstanceOf(ApiError);
    expect(err).toBeInstanceOf(Error);
    expect((err as ApiError).code).toBe("AUTH_REQUIRED");
    expect(typeof (err as ApiError).status).toBe("number");
    expect(localStorage.getItem("ef_token")).toBeNull();
    expect(localStorage.getItem("ef_refresh")).toBeNull();
    expect(localStorage.getItem("ef_user")).toBeNull();
  });
});

describe("401/403 cua queue token KHONG refresh (AC-5)", () => {
  it.each([
    [401, "/problems/queue-token-expired"],
    [401, "/problems/queue-token-invalid"],
    [403, "/problems/queue-token-mismatch"],
    [403, "/problems/queue-token-expired"],
  ])("%i %s -> khong goi refresh, giu phien, tra nguyen Response", async (status, type) => {
    seedSession("old", "r1");
    const { calls } = installFetch(() => problem(status, type));
    const res = await apiFetch("/v1/events/e1/queue/status");
    expect(res.status).toBe(status);
    expect(calls).toHaveLength(1);
    expect(localStorage.getItem("ef_token")).toBe("old");
    expect(localStorage.getItem("ef_refresh")).toBe("r1");
  });

  it("401 /problems/unauthorized (khong phai token-expired) cung khong refresh", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch(() => problem(401, "/problems/unauthorized"));
    await apiFetch("/v1/orders/o1").catch(() => undefined);
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
  });
});

describe("ApiError & readProblem", () => {
  it("readProblem doc type/title/status/detail tu body problem+json", async () => {
    const res = problem(422, "/problems/quantity-not-allowed", "so luong khong hop le");
    await expect(readProblem(res)).resolves.toEqual({
      type: "/problems/quantity-not-allowed",
      title: "t",
      status: 422,
      detail: "so luong khong hop le",
    });
  });

  it("readProblem voi body khong phai JSON van tra status cua Response", async () => {
    const res = new Response("<html>bad gateway</html>", { status: 502 });
    const p = await readProblem(res);
    expect(p.status).toBe(502);
    expect(p.type).toBeUndefined();
  });
});

function jwtExp(secondsFromNow: number) {
  const b64 = (o: unknown) => btoa(JSON.stringify(o)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  return `${b64({ alg: "HS256", typ: "JWT" })}.${b64({ sub: "u-1", exp: Math.floor(Date.now() / 1000) + secondsFromNow })}.sig`;
}

describe("refresh chu dong khi exp <= 30s (plan round 2)", () => {
  function server(expectedBearer: string) {
    return installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        return json({ id: "u-1", role: "buyer", access_token: "fresh", refresh_token: "r2" });
      }
      return c.headers.get("Authorization") === `Bearer ${expectedBearer}`
        ? json({ ok: true })
        : problem(401, "/problems/token-expired");
    });
  }

  it("access token con 10s -> refresh TRUOC khi gui, request goc chi gui 1 lan voi Bearer moi", async () => {
    seedSession(jwtExp(10), "r1");
    const { calls } = server("fresh");
    const res = await apiFetch("/v1/orders/o1");
    expect(res.status).toBe(200);
    expect(calls.map((c) => c.path)).toEqual(["/api/v1/auth/refresh", "/v1/orders/o1"]);
    expect(calls[1].headers.get("Authorization")).toBe("Bearer fresh");
    expect(localStorage.getItem("ef_refresh")).toBe("r2");
  });

  it("access token da het han (exp qua khu) cung refresh truoc", async () => {
    seedSession(jwtExp(-60), "r1");
    const { calls } = server("fresh");
    await apiFetch("/v1/orders/o1");
    expect(calls[0].path).toBe("/api/v1/auth/refresh");
  });

  it("access token con 10 phut -> KHONG refresh", async () => {
    const tok = jwtExp(600);
    seedSession(tok, "r1");
    const { calls } = server(tok);
    await apiFetch("/v1/orders/o1");
    expect(calls.map((c) => c.path)).toEqual(["/v1/orders/o1"]);
  });

  it("token khong doc duoc exp (opaque) -> khong refresh chu dong", async () => {
    seedSession("opaque", "r1");
    const { calls } = server("opaque");
    await apiFetch("/v1/orders/o1");
    expect(calls.map((c) => c.path)).toEqual(["/v1/orders/o1"]);
  });

  it("3 request song song khi sap het han -> dung 1 lan refresh", async () => {
    seedSession(jwtExp(5), "r1");
    const { calls } = installFetch(async (c) => {
      if (c.path === "/api/v1/auth/refresh") {
        await new Promise((r) => setTimeout(r, 10));
        return json({ id: "u-1", role: "buyer", access_token: "fresh", refresh_token: "r2" });
      }
      return json({ ok: true });
    });
    await Promise.all([apiFetch("/v1/orders/a"), apiFetch("/v1/orders/b"), apiFetch("/v1/orders/c")]);
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
  });
});

describe("hai tab: doc lai storage truoc khi refresh (plan round 2, M3)", () => {
  it("tab khac da xoay cap token (refresh khac) -> dung access token trong storage, KHONG goi /refresh", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") return json({ access_token: "WRONG", refresh_token: "WRONG" });
      if (c.headers.get("Authorization") === "Bearer tabB-access") return json({ ok: true });
      // Gia lap tab B vua refresh xong trong luc request cua tab A dang bay.
      localStorage.setItem("ef_token", "tabB-access");
      localStorage.setItem("ef_refresh", "r2");
      return problem(401, "/problems/token-expired");
    });
    const res = await apiFetch("/v1/orders/o1");
    expect(res.status).toBe(200);
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
    const originals = calls.filter((c) => c.path === "/v1/orders/o1");
    expect(originals).toHaveLength(2);
    expect(originals[1].headers.get("Authorization")).toBe("Bearer tabB-access");
    expect(localStorage.getItem("ef_refresh")).toBe("r2");
  });

  it("refresh token trong storage KHONG doi -> van goi /refresh", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        return json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2" });
      }
      return c.headers.get("Authorization") === "Bearer new" ? json({ ok: true }) : problem(401, "/problems/token-expired");
    });
    await apiFetch("/v1/orders/o1");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
  });

  it("tab khac da dang xuat (storage trong) trong luc cho -> AUTH_REQUIRED, khong goi /refresh", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") return json({ access_token: "WRONG", refresh_token: "WRONG" });
      localStorage.clear();
      return problem(401, "/problems/token-expired");
    });
    const err = await apiFetch("/v1/orders/o1").then(
      () => null,
      (e: unknown) => e,
    );
    expect((err as ApiError | null)?.code).toBe("AUTH_REQUIRED");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
  });

  describe("Web Locks", () => {
    afterEach(() => {
      delete (navigator as unknown as { locks?: unknown }).locks;
    });

    it("dung navigator.locks.request('ef-auth-refresh') khi co", async () => {
      const request = vi.fn(async (...args: unknown[]) => {
        const cb = args[args.length - 1] as (lock: unknown) => Promise<unknown>;
        return cb({ name: args[0] });
      });
      Object.defineProperty(navigator, "locks", { value: { request }, configurable: true });
      seedSession("old", "r1");
      installFetch((c) => {
        if (c.path === "/api/v1/auth/refresh") {
          return json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2" });
        }
        return c.headers.get("Authorization") === "Bearer new" ? json({ ok: true }) : problem(401, "/problems/token-expired");
      });
      const res = await apiFetch("/v1/orders/o1");
      expect(res.status).toBe(200);
      expect(request).toHaveBeenCalled();
      expect(request.mock.calls[0][0]).toBe("ef-auth-refresh");
    });

    it("khong co navigator.locks -> van refresh binh thuong (single-flight trong tab)", async () => {
      seedSession("old", "r1");
      const { calls } = installFetch((c) => {
        if (c.path === "/api/v1/auth/refresh") {
          return json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2" });
        }
        return c.headers.get("Authorization") === "Bearer new" ? json({ ok: true }) : problem(401, "/problems/token-expired");
      });
      await apiFetch("/v1/orders/o1");
      expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
    });
  });
});
