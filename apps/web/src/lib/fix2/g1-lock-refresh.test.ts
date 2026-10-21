/** Regression G-1 / H-1 (Y-1, R2-3, Z-1..Z-3, R3-1): the refresh call runs only inside the Web Lock, once per token.
 *  Two "tabs" = two module instances sharing localStorage and a FIFO LockManager stub (no `steal`). */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

/** FIFO lock manager: an aborted pending request never gets the lock. */
function fakeLocks() {
  let held = false;
  const queue: Array<() => void> = [];
  const next = () => {
    held = false;
    queue.shift()?.();
  };
  const request = (_name: string, a: unknown, b?: unknown) => {
    const cb = (typeof a === "function" ? a : b) as () => Promise<unknown>;
    const opts = (typeof a === "function" ? {} : a) as { signal?: AbortSignal };
    return new Promise<unknown>((resolve, reject) => {
      const run = () => {
        held = true;
        Promise.resolve()
          .then(cb)
          .then(
            (v) => {
              resolve(v);
              next();
            },
            (e) => {
              reject(e);
              next();
            },
          );
      };
      if (!held) {
        run();
        return;
      }
      const entry = () => run();
      queue.push(entry);
      opts.signal?.addEventListener("abort", () => {
        const i = queue.indexOf(entry);
        if (i >= 0) {
          queue.splice(i, 1);
          reject(new DOMException("aborted", "AbortError"));
        }
      });
    });
  };
  return { request };
}

/** Identity service with refresh-token reuse detection (family revoked on reuse). */
function identityServer() {
  let current = "r1";
  let n = 1;
  let revoked = false;
  const consumed = new Set<string>();
  const log: string[] = [];
  return {
    log,
    postsWith(tok: string) {
      return log.filter((l) => l.startsWith(`refresh(${tok})`)).length;
    },
    refresh(tok: string) {
      if (revoked) return problem(401, "/problems/unauthorized");
      if (tok === current) {
        consumed.add(tok);
        n++;
        current = `r${n}`;
        log.push(`refresh(${tok}) -> ok`);
        return json({ id: "u-1", role: "buyer", access_token: `acc-${n}`, refresh_token: current, expires_in: 900 });
      }
      log.push(`refresh(${tok}) -> REUSE`);
      if (consumed.has(tok)) revoked = true;
      return problem(401, "/problems/unauthorized", "reuse");
    },
  };
}

type ApiModule = typeof import("@/lib/api-client");

async function newTab(): Promise<{ api: ApiModule; sess: typeof import("@/lib/session") }> {
  vi.resetModules();
  const api = await import("@/lib/api-client");
  const sess = await import("@/lib/session");
  return { api, sess };
}

function seedBuyFlow() {
  sessionStorage.setItem("ef:queue:token:e1", "q-tok");
  sessionStorage.setItem(
    "evenflow_hold_e1",
    JSON.stringify({ orderId: "o1", holdId: "h1", expiresAt: "2030-01-01T00:00:00Z" }),
  );
  sessionStorage.setItem("ef:pay:o1", "key-1");
}

/** Identity-backed fetch: the n-th refresh response is delayed by delays[n] ms (last value repeats). */
function installIdentity(srv: ReturnType<typeof identityServer>, delays: number[]) {
  let validAccess = "acc-never";
  let n = 0;
  installFetch(async (c) => {
    if (c.path === "/api/v1/auth/refresh") {
      const res = srv.refresh((c.body as { refresh_token: string }).refresh_token);
      const d = delays[Math.min(n++, delays.length - 1)];
      if (d) await new Promise((r) => setTimeout(r, d));
      const b = (await res.clone().json()) as { access_token?: string };
      if (b.access_token) validAccess = b.access_token;
      return res;
    }
    return c.headers.get("Authorization") === `Bearer ${validAccess}`
      ? json({ ok: true })
      : problem(401, "/problems/token-expired");
  });
}

const outcome = (tag: string, p: Promise<Response>) =>
  p.then(
    (r) => `${tag}:${r.status}`,
    (e: { code?: string }) => `${tag}:${e.code}`,
  );

beforeEach(setupEnv);
afterEach(() => {
  vi.useRealTimers();
  Reflect.deleteProperty(navigator, "locks");
  resetEnv();
});

describe("H-1", () => {
  it("Z-1: holder POST takes 40 s + a waiting tab: exactly one POST per token, nobody logged out", async () => {
    vi.useFakeTimers();
    Object.defineProperty(navigator, "locks", { value: fakeLocks(), configurable: true });
    const srv = identityServer();
    installIdentity(srv, [40_000]);
    seedSession("old", "r1");
    seedBuyFlow();
    const tabA = await newTab();
    const tabB = await newTab();
    const pA = outcome("A", tabA.api.apiFetch("/v1/orders/o1"));
    await vi.advanceTimersByTimeAsync(5);
    const pB = outcome("B", tabB.api.apiFetch("/v1/orders/o2"));
    await vi.advanceTimersByTimeAsync(60_000);
    expect(await pA).toBe("A:200");
    expect(await pB).toBe("B:200");
    expect(srv.postsWith("r1")).toBe(1);
    expect(srv.log.filter((l) => l.includes("REUSE"))).toHaveLength(0);
    expect(tabB.sess.getSession()).not.toBeNull();
    expect(sessionStorage.getItem("ef:queue:token:e1")).toBe("q-tok");
  });

  it("Z-1b: holder POST takes 60 s: the waiter gives up at 45 s with NETWORK_ERROR (no POST, session kept); the holder finishes", async () => {
    vi.useFakeTimers();
    Object.defineProperty(navigator, "locks", { value: fakeLocks(), configurable: true });
    const srv = identityServer();
    installIdentity(srv, [60_000]);
    seedSession("old", "r1");
    seedBuyFlow();
    const tabA = await newTab();
    const tabB = await newTab();
    const pA = outcome("A", tabA.api.apiFetch("/v1/orders/o1"));
    await vi.advanceTimersByTimeAsync(5);
    const pB = outcome("B", tabB.api.apiFetch("/v1/orders/o2"));
    await vi.advanceTimersByTimeAsync(46_000);
    expect(await pB).toBe("B:NETWORK_ERROR");
    expect(srv.postsWith("r1")).toBe(1);
    expect(tabB.sess.getSession()).not.toBeNull();
    await vi.advanceTimersByTimeAsync(30_000);
    expect(await pA).toBe("A:200");
    expect(srv.postsWith("r1")).toBe(1); // the abandoned waiter never POSTs when the lock frees up
    expect(srv.log.filter((l) => l.includes("REUSE"))).toHaveLength(0);
    expect(tabA.sess.getSession()?.refreshToken).toBe("r2");
  });

  it("Z-2: two waiters time out together behind a stuck holder: 0 extra POSTs, session kept", async () => {
    vi.useFakeTimers();
    const locks = fakeLocks();
    Object.defineProperty(navigator, "locks", { value: locks, configurable: true });
    let release!: () => void;
    locks.request("ef-auth-refresh", () => new Promise<void>((r) => (release = r))).catch(() => undefined);
    const srv = identityServer();
    installIdentity(srv, [0]);
    seedSession("old", "r1");
    seedBuyFlow();
    const tabB = await newTab();
    const tabC = await newTab();
    const pB = outcome("B", tabB.api.apiFetch("/v1/orders/o2"));
    const pC = outcome("C", tabC.api.apiFetch("/v1/orders/o3"));
    await vi.advanceTimersByTimeAsync(0);
    vi.advanceTimersByTime(45_000);
    await vi.advanceTimersByTimeAsync(1_000);
    expect(await pB).toBe("B:NETWORK_ERROR");
    expect(await pC).toBe("C:NETWORK_ERROR");
    release();
    await vi.advanceTimersByTimeAsync(5_000);
    expect(srv.log).toHaveLength(0);
    expect(tabB.sess.getSession()).not.toBeNull();
    expect(sessionStorage.getItem("ef:queue:token:e1")).toBe("q-tok");
  });

  it("Z-3: own POST answered 401 -> AUTH_REQUIRED and the session is cleared", async () => {
    vi.useFakeTimers();
    Object.defineProperty(navigator, "locks", { value: fakeLocks(), configurable: true });
    installFetch(async (c) => {
      if (c.path === "/api/v1/auth/refresh") {
        await new Promise((r) => setTimeout(r, 500));
        return problem(401, "/problems/unauthorized");
      }
      return problem(401, "/problems/token-expired");
    });
    seedSession("old", "r1");
    const tab = await newTab();
    const p = outcome("B", tab.api.apiFetch("/v1/orders/o2"));
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await p).toBe("B:AUTH_REQUIRED");
    expect(tab.sess.getSession()).toBeNull();
  });

  it.each(["local", "session"] as const)(
    "R3-1: the refresh token lives only under ef_refresh (persist=%s), during the POST and after clearSession",
    async (persist) => {
      vi.useFakeTimers();
      Object.defineProperty(navigator, "locks", { value: fakeLocks(), configurable: true });
      const store = persist === "local" ? localStorage : sessionStorage;
      store.setItem("ef_token", "old");
      store.setItem("ef_refresh", "r1");
      store.setItem("ef_user", JSON.stringify({ id: "u-1", role: "buyer" }));
      const leaks: string[] = [];
      const scan = () => {
        for (const st of [localStorage, sessionStorage]) {
          for (let i = 0; i < st.length; i++) {
            const k = st.key(i)!;
            const v = st.getItem(k) ?? "";
            if (k !== "ef_refresh" && (v.includes("r1") || v.includes("r2") || /refresh/.test(k))) leaks.push(k);
          }
        }
      };
      let valid = "acc-never";
      installFetch(async (c) => {
        if (c.path === "/api/v1/auth/refresh") {
          scan();
          await new Promise((r) => setTimeout(r, 1_000));
          scan();
          valid = "acc-2";
          return json({ id: "u-1", role: "buyer", access_token: "acc-2", refresh_token: "r2", expires_in: 900 });
        }
        return c.headers.get("Authorization") === `Bearer ${valid}`
          ? json({ ok: true })
          : problem(401, "/problems/token-expired");
      });
      const tab = await newTab();
      const p = tab.api.apiFetch("/v1/orders/o1");
      await vi.advanceTimersByTimeAsync(3_000);
      expect((await p).status).toBe(200);
      scan();
      tab.sess.clearSession();
      for (const st of [localStorage, sessionStorage]) {
        for (let i = 0; i < st.length; i++) expect(st.key(i)).not.toMatch(/refresh/);
      }
      expect(leaks).toEqual([]);
    },
  );

  it("no navigator.locks: in-tab single-flight, one POST for concurrent requests", async () => {
    const srv = identityServer();
    installIdentity(srv, [0]);
    seedSession("old", "r1");
    const tab = await newTab();
    const [a, b] = await Promise.all([tab.api.apiFetch("/v1/orders/o1"), tab.api.apiFetch("/v1/orders/o2")]);
    expect([a.status, b.status]).toEqual([200, 200]);
    expect(srv.postsWith("r1")).toBe(1);
  });
});

describe("G-1", () => {
  it("another tab rotated the pair while we waited: use it, no POST at all", async () => {
    vi.useFakeTimers();
    const locks = fakeLocks();
    Object.defineProperty(navigator, "locks", { value: locks, configurable: true });
    let release!: () => void;
    locks.request("ef-auth-refresh", () => new Promise<void>((r) => (release = r))).catch(() => undefined);
    const { calls } = installFetch((c) =>
      c.path === "/api/v1/auth/refresh"
        ? json({ id: "u-1", role: "buyer", access_token: "x", refresh_token: "rx" })
        : c.headers.get("Authorization") === "Bearer rotated"
          ? json({ ok: true })
          : problem(401, "/problems/token-expired"),
    );
    seedSession("old", "r1");
    const tab = await newTab();
    const p = tab.api.apiFetch("/v1/orders/o1");
    await vi.advanceTimersByTimeAsync(3_000);
    localStorage.setItem("ef_token", "rotated");
    localStorage.setItem("ef_refresh", "r2");
    release();
    await vi.advanceTimersByTimeAsync(1_000);
    expect((await p).status).toBe(200);
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
  });

  it("lock manager that never settles: NETWORK_ERROR at the deadline, session and buy-flow keys kept, no POST", async () => {
    vi.useFakeTimers();
    Object.defineProperty(navigator, "locks", {
      value: { request: () => new Promise(() => {}) },
      configurable: true,
    });
    const { calls } = installFetch((c) =>
      c.path === "/api/v1/auth/refresh" ? json({}) : problem(401, "/problems/token-expired"),
    );
    seedSession("old", "r1");
    seedBuyFlow();
    const tab = await newTab();
    const p = tab.api.apiFetch("/v1/orders/o1").then(
      () => "ok",
      (e: { code?: string }) => e.code,
    );
    await vi.advanceTimersByTimeAsync(120_000);
    expect(await p).toBe("NETWORK_ERROR");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
    expect(tab.sess.getSession()).not.toBeNull();
    expect(sessionStorage.getItem("ef:queue:token:e1")).toBe("q-tok");
  });
});
