/** AC-2 (R4-1 / W-3), spec as clarified 2026-10-21:
 *  - the call that OWNS the refresh POST waits for it, bounded by the 120 s abort (Z-1b stays valid);
 *  - every OTHER caller (later same-tab calls joining the in-flight refresh, other tabs waiting for the lock) rejects
 *    NETWORK_ERROR after at most 45 s, session kept, never a second POST while the first is in flight;
 *  - the POST is aborted at 120 s (AbortController + setTimeout; stubs honour init.signal) -> AUTH_REQUIRED and the
 *    session is cleared only if epoch and stored refresh token still belong to the POSTed token;
 *  - the lock is released when the POST settles; the in-flight refresh is keyed by user.
 *  Assumed interface: `REFRESH_POST_TIMEOUT_MS = 120_000` exported from "@/lib/api-client". */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { fakeLocks, hangUntilAborted, installLocks, newTab, outcome, removeLocks, seedUser } from "./harness";

beforeEach(() => {
  setupEnv();
  vi.useFakeTimers();
  installLocks(fakeLocks());
});
afterEach(() => {
  vi.useRealTimers();
  removeLocks();
  resetEnv();
});

const isRefresh = (c: { path: string }) => c.path === "/api/v1/auth/refresh";
const refreshBodies = (calls: Array<{ path: string; body: unknown }>) =>
  calls.filter(isRefresh).map((c) => (c.body as { refresh_token: string }).refresh_token);

/** Refresh POST hangs until aborted by default; API calls answer 401 token-expired unless Bearer matches valid(). */
function server(valid: () => string = () => "acc-never") {
  let refreshHandler: ((init: RequestInit, body: { refresh_token: string }) => Promise<Response>) | null = null;
  const state = installFetch(async (c) => {
    if (isRefresh(c)) {
      return refreshHandler ? refreshHandler(c.init, c.body as { refresh_token: string }) : hangUntilAborted(c.init);
    }
    return c.headers.get("Authorization") === `Bearer ${valid()}`
      ? json({ ok: true })
      : problem(401, "/problems/token-expired");
  });
  return { ...state, setRefresh: (h: NonNullable<typeof refreshHandler>) => (refreshHandler = h) };
}

describe("AC-2 non-owner callers are bounded at 45 s", () => {
  it("W-3: other tab waiting on the lock rejects NETWORK_ERROR at 45 s each time; 1 POST total; owner still waiting; session kept", async () => {
    const { calls } = server();
    seedUser("u-1", "old", "r1");
    const tabA = await newTab();
    const tabB = await newTab();
    let ownerSettled = false;
    tabA.api.apiFetch("/v1/orders/o1").then(
      () => (ownerSettled = true),
      () => (ownerSettled = true),
    );
    await vi.advanceTimersByTimeAsync(5);
    const out: string[] = [];
    for (let i = 0; i < 2; i++) {
      const p = outcome("B" + i, tabB.api.apiFetch("/v1/orders/o2"));
      await vi.advanceTimersByTimeAsync(46_000);
      out.push(await p);
    }
    expect(out).toEqual(["B0:NETWORK_ERROR", "B1:NETWORK_ERROR"]);
    expect(ownerSettled).toBe(false); // t = 92 s < 120 s
    expect(calls.filter(isRefresh)).toHaveLength(1);
    expect(tabB.sess.getSession()?.refreshToken).toBe("r1");
  });

  it("a later same-tab caller joins the in-flight refresh and rejects NETWORK_ERROR at 45 s; a call at 50 s joins too (still 1 POST)", async () => {
    const { calls } = server();
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    let ownerSettled = false;
    tab.api.apiFetch("/v1/orders/o1").then(
      () => (ownerSettled = true),
      () => (ownerSettled = true),
    );
    await vi.advanceTimersByTimeAsync(1_000);
    const joiner = outcome("J", tab.api.apiFetch("/v1/orders/o1b"));
    await vi.advanceTimersByTimeAsync(44_000);
    let joinerSettled = false;
    void joiner.then(() => (joinerSettled = true));
    await vi.advanceTimersByTimeAsync(0);
    expect(joinerSettled).toBe(false); // t = 45 s, joiner started at 1 s -> deadline 46 s
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await joiner).toBe("J:NETWORK_ERROR");
    expect(ownerSettled).toBe(false);
    expect(tab.sess.getSession()?.refreshToken).toBe("r1");
    await vi.advanceTimersByTimeAsync(4_000); // t = 52 s ... start a new call at ~52 s while POST still pending
    const late = outcome("L", tab.api.apiFetch("/v1/orders/o3"));
    await vi.advanceTimersByTimeAsync(46_000);
    expect(await late).toBe("L:NETWORK_ERROR");
    expect(calls.filter(isRefresh)).toHaveLength(1);
    expect(tab.sess.getSession()?.refreshToken).toBe("r1");
  });

  it("GUARD (Z-1b): the owner whose POST answers at 60 s gets 200; the abandoned waiter never POSTs afterwards", async () => {
    let valid = "acc-never";
    const srv = server(() => valid);
    srv.setRefresh(
      () =>
        new Promise<Response>((resolve) =>
          setTimeout(() => {
            valid = "acc-2";
            resolve(json({ id: "u-1", role: "buyer", access_token: "acc-2", refresh_token: "r2", expires_in: 900 }));
          }, 60_000),
        ),
    );
    seedUser("u-1", "old", "r1");
    const tabA = await newTab();
    const tabB = await newTab();
    const a = outcome("A", tabA.api.apiFetch("/v1/orders/o1"));
    await vi.advanceTimersByTimeAsync(5);
    const b = outcome("B", tabB.api.apiFetch("/v1/orders/o2"));
    await vi.advanceTimersByTimeAsync(46_000);
    expect(await b).toBe("B:NETWORK_ERROR");
    await vi.advanceTimersByTimeAsync(20_000);
    expect(await a).toBe("A:200");
    expect(refreshBodies(srv.calls)).toEqual(["r1"]);
    expect(tabA.sess.getSession()?.refreshToken).toBe("r2");
  });

  it("after a joiner gave up, the POST that finally answers (60 s) still stores the pair and frees the lock for another tab (no 2nd POST)", async () => {
    let valid = "acc-never";
    const srv = server(() => valid);
    srv.setRefresh(
      () =>
        new Promise<Response>((resolve) =>
          setTimeout(() => {
            valid = "acc-2";
            resolve(json({ id: "u-1", role: "buyer", access_token: "acc-2", refresh_token: "r2", expires_in: 900 }));
          }, 60_000),
        ),
    );
    seedUser("u-1", "old", "r1");
    const tabA = await newTab();
    const tabB = await newTab();
    tabA.api.apiFetch("/v1/orders/o1").catch(() => undefined);
    await vi.advanceTimersByTimeAsync(1_000);
    const joiner = outcome("J", tabA.api.apiFetch("/v1/orders/o1b"));
    await vi.advanceTimersByTimeAsync(46_000);
    expect(await joiner).toBe("J:NETWORK_ERROR");
    await vi.advanceTimersByTimeAsync(20_000); // t = 67 s: POST answered at 60 s
    expect(tabA.sess.getSession()?.refreshToken).toBe("r2");
    const b = outcome("B", tabB.api.apiFetch("/v1/orders/o2"));
    await vi.advanceTimersByTimeAsync(1_000);
    expect(await b).toBe("B:200");
    expect(srv.calls.filter(isRefresh)).toHaveLength(1);
  });
});

describe("AC-2 the POST is aborted at 120 s", () => {
  it("exports REFRESH_POST_TIMEOUT_MS = 120_000", async () => {
    const mod = (await import("@/lib/api-client")) as unknown as Record<string, unknown>;
    expect(mod.REFRESH_POST_TIMEOUT_MS).toBe(120_000);
  });

  it("owner pending at 119 s, signal aborted and AUTH_REQUIRED at 121 s, session cleared, r1 POSTed once", async () => {
    const signals: AbortSignal[] = [];
    const srv = server();
    srv.setRefresh((init) => {
      if (init.signal) signals.push(init.signal);
      return hangUntilAborted(init);
    });
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    let owner = "pending";
    tab.api.apiFetch("/v1/orders/o1").then(
      () => (owner = "ok"),
      (e: { code?: string }) => (owner = String(e.code)),
    );
    await vi.advanceTimersByTimeAsync(119_000);
    expect(owner).toBe("pending");
    expect(signals).toHaveLength(1);
    expect(signals[0].aborted).toBe(false);
    expect(tab.sess.getSession()).not.toBeNull();
    await vi.advanceTimersByTimeAsync(2_000);
    expect(signals[0].aborted).toBe(true);
    expect(owner).toBe("AUTH_REQUIRED");
    expect(tab.sess.getSession()).toBeNull();
    expect(localStorage.getItem("ef_refresh")).toBeNull();
    expect(refreshBodies(srv.calls)).toEqual(["r1"]);
  });

  it("abort guard (other tab): another tab signed in as u-2 during the POST -> AUTH_REQUIRED but u-2's session is left intact", async () => {
    const srv = server();
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    let owner = "pending";
    tab.api.apiFetch("/v1/orders/o1").then(
      () => (owner = "ok"),
      (e: { code?: string }) => (owner = String(e.code)),
    );
    await vi.advanceTimersByTimeAsync(60_000);
    seedUser("u-2", "acc-u2", "r-u2");
    await vi.advanceTimersByTimeAsync(62_000);
    expect(owner).toBe("AUTH_REQUIRED");
    expect(localStorage.getItem("ef_token")).toBe("acc-u2");
    expect(localStorage.getItem("ef_refresh")).toBe("r-u2");
    expect(JSON.parse(localStorage.getItem("ef_user")!).id).toBe("u-2");
    expect(refreshBodies(srv.calls)).toEqual(["r1"]);
  });

  it("abort guard (same tab): sign-out and new sign-in in this tab during the POST (epoch changed) -> the new session is left intact", async () => {
    const srv = server();
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    let owner = "pending";
    tab.api.apiFetch("/v1/orders/o1").then(
      () => (owner = "ok"),
      (e: { code?: string }) => (owner = String(e.code)),
    );
    await vi.advanceTimersByTimeAsync(60_000);
    tab.sess.clearSession();
    tab.sess.setSession({ accessToken: "acc-new", refreshToken: "r1", user: { id: "u-1", role: "buyer" } });
    await vi.advanceTimersByTimeAsync(62_000);
    expect(owner).toBe("AUTH_REQUIRED");
    expect(tab.sess.getSession()?.accessToken).toBe("acc-new");
    expect(refreshBodies(srv.calls)).toEqual(["r1"]);
  });

  it("after the abort the lock is released: user signs in again (r9); a request in another tab refreshes normally; r1 never POSTed twice", async () => {
    let phase: "hang" | "ok" = "hang";
    const srv = server(() => "acc-9");
    srv.setRefresh((init) =>
      phase === "hang"
        ? hangUntilAborted(init)
        : Promise.resolve(json({ id: "u-1", role: "buyer", access_token: "acc-9", refresh_token: "r10", expires_in: 900 })),
    );
    seedUser("u-1", "old", "r1");
    const tabA = await newTab();
    const tabB = await newTab();
    const a = outcome("A", tabA.api.apiFetch("/v1/orders/o1"));
    await vi.advanceTimersByTimeAsync(121_000);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(tabA.sess.getSession()).toBeNull();
    phase = "ok";
    seedUser("u-1", "old2", "r9");
    const b = outcome("B", tabB.api.apiFetch("/v1/orders/o2"));
    await vi.advanceTimersByTimeAsync(2_000);
    expect(await b).toBe("B:200");
    expect(refreshBodies(srv.calls)).toEqual(["r1", "r9"]);
    expect(tabB.sess.getSession()?.refreshToken).toBe("r10");
  });

  it("GUARD: a plain network failure of the POST (not an abort) keeps the session -> NETWORK_ERROR", async () => {
    const srv = server();
    srv.setRefresh(() => Promise.reject(new TypeError("Failed to fetch")));
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    const p = outcome("A", tab.api.apiFetch("/v1/orders/o1"));
    await vi.advanceTimersByTimeAsync(1_000);
    expect(await p).toBe("A:NETWORK_ERROR");
    expect(tab.sess.getSession()?.refreshToken).toBe("r1");
  });

  it("GUARD (Z-3): POST answered 401 within the window -> AUTH_REQUIRED, session cleared", async () => {
    const srv = server();
    srv.setRefresh(async () => problem(401, "/problems/unauthorized"));
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    const p = outcome("A", tab.api.apiFetch("/v1/orders/o1"));
    await vi.advanceTimersByTimeAsync(1_000);
    expect(await p).toBe("A:AUTH_REQUIRED");
    expect(tab.sess.getSession()).toBeNull();
  });
});

describe("AC-2 / plan m-1: the in-flight refresh is keyed by user", () => {
  it("a u-2 caller in the same tab does not join u-1's hung refresh; it POSTs with its own token and succeeds", async () => {
    removeLocks(); // in-tab single-flight path: no lock serialisation hides the sharing
    let valid = "acc-never";
    const srv = server(() => valid);
    srv.setRefresh((init, body) => {
      if (body.refresh_token === "r-u2") {
        valid = "acc-u2-new";
        return Promise.resolve(
          json({ id: "u-2", role: "buyer", access_token: "acc-u2-new", refresh_token: "r-u2b", expires_in: 900 }),
        );
      }
      return hangUntilAborted(init);
    });
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    tab.api.apiFetch("/v1/orders/o1").catch(() => undefined); // u-1 refresh hangs
    await vi.advanceTimersByTimeAsync(1_000);
    tab.sess.clearSession();
    tab.sess.setSession({ accessToken: "acc-u2-old", refreshToken: "r-u2", user: { id: "u-2", role: "buyer" } });
    const b = outcome("B", tab.api.apiFetch("/v1/orders/o2"));
    await vi.advanceTimersByTimeAsync(5_000);
    expect(await b).toBe("B:200");
    expect(refreshBodies(srv.calls)).toEqual(["r1", "r-u2"]);
    expect(tab.sess.getSession()?.user.id).toBe("u-2");
  });
});

/** Plan M2-1: one rule for the shared in-flight refresh, with and without a usable lock manager. */
describe.each([
  ["no navigator.locks", () => removeLocks()],
  [
    "locks.request rejects SecurityError",
    () => installLocks({ request: () => Promise.reject(new DOMException("denied", "SecurityError")) }),
  ],
])("AC-2 never-answering POST, %s", (_label, setup) => {
  it("calls at 0 s, 46 s and 100 s -> exactly 1 POST; joiner at 46 s NETWORK_ERROR at 91 s; owner AUTH_REQUIRED at 121 s; session cleared", async () => {
    setup();
    const srv = server();
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    const owner = outcome("owner", tab.api.apiFetch("/v1/orders/o1"));
    await vi.advanceTimersByTimeAsync(46_000);
    const j1 = outcome("j1", tab.api.apiFetch("/v1/orders/o2"));
    await vi.advanceTimersByTimeAsync(44_000); // t = 90 s
    let j1Done = false;
    void j1.then(() => (j1Done = true));
    await vi.advanceTimersByTimeAsync(0);
    expect(j1Done).toBe(false);
    await vi.advanceTimersByTimeAsync(2_000); // t = 92 s
    expect(await j1).toBe("j1:NETWORK_ERROR");
    expect(tab.sess.getSession()?.refreshToken).toBe("r1");
    await vi.advanceTimersByTimeAsync(8_000); // t = 100 s
    const j2 = outcome("j2", tab.api.apiFetch("/v1/orders/o3"));
    await vi.advanceTimersByTimeAsync(21_000); // t = 121 s
    expect(await owner).toBe("owner:AUTH_REQUIRED");
    expect(await j2).toBe("j2:AUTH_REQUIRED");
    expect(srv.calls.filter(isRefresh)).toHaveLength(1);
    expect(tab.sess.getSession()).toBeNull();
  });
});

/** Plan m2-3: no timer (45 s caller, lock-wait deadline, 120 s abort) survives any outcome. */
describe("AC-2 timer hygiene: vi.getTimerCount() returns to its starting value after every outcome", () => {
  const run = async (setup: (srv: ReturnType<typeof server>) => void, advance: number, expectCode: string) => {
    const srv = server(() => "acc-2");
    setup(srv);
    seedUser("u-1", "old", "r1");
    const tab = await newTab();
    const start = vi.getTimerCount();
    const p = outcome("A", tab.api.apiFetch("/v1/orders/o1"));
    await vi.advanceTimersByTimeAsync(advance);
    expect(await p).toBe(expectCode);
    await vi.advanceTimersByTimeAsync(0);
    expect(vi.getTimerCount()).toBeLessThanOrEqual(start);
  };

  it("success", () =>
    run(
      (s) => s.setRefresh(async () => json({ id: "u-1", role: "buyer", access_token: "acc-2", refresh_token: "r2", expires_in: 900 })),
      1_000,
      "A:200",
    ));
  it("401 on the POST", () => run((s) => s.setRefresh(async () => problem(401, "/problems/unauthorized")), 1_000, "A:AUTH_REQUIRED"));
  it("network error on the POST", () => run((s) => s.setRefresh(() => Promise.reject(new TypeError("x"))), 1_000, "A:NETWORK_ERROR"));
  it("120 s abort", () => run(() => undefined, 121_000, "A:AUTH_REQUIRED"));
});
