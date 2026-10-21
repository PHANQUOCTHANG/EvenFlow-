/** Regression G-1 (Y-1, R2-3): the refresh call must never run outside the Web Lock.
 *  Two "tabs" = two module instances sharing localStorage and a FIFO LockManager stub. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

interface Held {
  release: () => void;
  abort: () => void;
}

/** FIFO lock manager: pending requests honour AbortSignal; `steal` preempts the current holder. */
function fakeLocks() {
  let holder: Held | null = null;
  const queue: Array<() => void> = [];
  const request = (_name: string, a: unknown, b?: unknown) => {
    const cb = (typeof a === "function" ? a : b) as () => Promise<unknown>;
    const opts = (typeof a === "function" ? {} : a) as { signal?: AbortSignal; steal?: boolean };
    return new Promise<unknown>((resolve, reject) => {
      const run = () => {
        let done = false;
        const h: Held = {
          release: () => {
            if (done) return;
            done = true;
            if (holder === h) holder = null;
            queue.shift()?.();
          },
          abort: () => {
            if (done) return;
            done = true;
            reject(new DOMException("stolen", "AbortError"));
          },
        };
        holder = h;
        Promise.resolve()
          .then(cb)
          .then(
            (v) => {
              if (!done) resolve(v);
              h.release();
            },
            (e) => {
              if (!done) reject(e);
              h.release();
            },
          );
      };
      if (opts.steal) {
        holder?.abort();
        holder = null;
        run();
        return;
      }
      if (!holder) {
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

beforeEach(setupEnv);
afterEach(() => {
  vi.useRealTimers();
  Reflect.deleteProperty(navigator, "locks");
  resetEnv();
});

describe("G-1", () => {
  it("slow lock holder (12 s) + a waiting tab: one POST /refresh per token, nobody is logged out", async () => {
    vi.useFakeTimers();
    const locks = fakeLocks();
    Object.defineProperty(navigator, "locks", { value: locks, configurable: true });
    const srv = identityServer();
    let validAccess = "acc-never";
    let slowOnce = true;
    installFetch(async (c) => {
      if (c.path === "/api/v1/auth/refresh") {
        const res = srv.refresh((c.body as { refresh_token: string }).refresh_token);
        if (slowOnce) {
          slowOnce = false;
          await new Promise((r) => setTimeout(r, 12_000));
        }
        const b = (await res.clone().json()) as { access_token?: string };
        if (b.access_token) validAccess = b.access_token;
        return res;
      }
      return c.headers.get("Authorization") === `Bearer ${validAccess}`
        ? json({ ok: true })
        : problem(401, "/problems/token-expired");
    });
    seedSession("old", "r1");
    seedBuyFlow();
    const tabA = await newTab();
    const tabB = await newTab();
    const pA = tabA.api.apiFetch("/v1/orders/o1").then(
      (r) => `A:${r.status}`,
      (e: Error) => `A:${e.message}`,
    );
    await vi.advanceTimersByTimeAsync(5);
    const pB = tabB.api.apiFetch("/v1/orders/o2").then(
      (r) => `B:${r.status}`,
      (e: { code?: string }) => `B:${e.code}`,
    );
    await vi.advanceTimersByTimeAsync(20_000);
    expect(await pA).toBe("A:200");
    expect(await pB).toBe("B:200");
    expect(srv.postsWith("r1")).toBe(1);
    expect(tabB.sess.getSession()).not.toBeNull();
    expect(sessionStorage.getItem("ef:queue:token:e1")).toBe("q-tok");
    expect(sessionStorage.getItem("evenflow_hold_e1")).not.toBeNull();
  });

  it("suspended lock holder + two waiting tabs: lock stolen, still one POST /refresh, no tab logged out", async () => {
    vi.useFakeTimers();
    const locks = fakeLocks();
    Object.defineProperty(navigator, "locks", { value: locks, configurable: true });
    // The suspended holder: acquires the lock and never returns.
    locks.request("ef-auth-refresh", () => new Promise(() => {})).catch(() => undefined);
    const srv = identityServer();
    let validAccess = "acc-never";
    installFetch(async (c) => {
      if (c.path === "/api/v1/auth/refresh") {
        const res = srv.refresh((c.body as { refresh_token: string }).refresh_token);
        await new Promise((r) => setTimeout(r, 200));
        const b = (await res.clone().json()) as { access_token?: string };
        if (b.access_token) validAccess = b.access_token;
        return res;
      }
      return c.headers.get("Authorization") === `Bearer ${validAccess}`
        ? json({ ok: true })
        : problem(401, "/problems/token-expired");
    });
    seedSession("old", "r1");
    seedBuyFlow();
    const tabB = await newTab();
    const tabC = await newTab();
    const pB = tabB.api.apiFetch("/v1/orders/o2").then(
      (r) => `B:${r.status}`,
      (e: { code?: string }) => `B:${e.code}`,
    );
    const pC = tabC.api.apiFetch("/v1/orders/o3").then(
      (r) => `C:${r.status}`,
      (e: { code?: string }) => `C:${e.code}`,
    );
    await vi.advanceTimersByTimeAsync(30_000);
    expect(await pB).toBe("B:200");
    expect(await pC).toBe("C:200");
    expect(srv.postsWith("r1")).toBe(1);
    expect(srv.log.filter((l) => l.includes("REUSE"))).toHaveLength(0);
    expect(tabB.sess.getSession()).not.toBeNull();
    expect(sessionStorage.getItem("ef:queue:token:e1")).toBe("q-tok");
  });

  it("another tab rotated the pair while we waited: use it, no POST at all", async () => {
    vi.useFakeTimers();
    const locks = fakeLocks();
    Object.defineProperty(navigator, "locks", { value: locks, configurable: true });
    locks.request("ef-auth-refresh", () => new Promise(() => {})).catch(() => undefined);
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
    await vi.advanceTimersByTimeAsync(12_000);
    expect((await p).status).toBe(200);
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
  });

  it("lock cannot be obtained before the hard deadline: NETWORK_ERROR, session and buy-flow keys kept, no POST", async () => {
    vi.useFakeTimers();
    // Pending requests never settle, even with steal.
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
