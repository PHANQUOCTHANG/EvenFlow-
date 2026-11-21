/** AC-2 (R2-3, R2-S1): a refresh token that got a response (2xx, 400/401/403, bad body) or was aborted is never POSTed
 *  again from this tab. Record = sessionStorage `ef:rt:used` (JSON array of short hashes, max 16, never the token).
 *  Assumed interface: `ef:rt:used` holds a JSON array of strings. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { BUY_FLOW_KEYS, fakeLocks, installLocks, newTab, outcome, removeLocks, seedBuyFlow, seedUser } from "../debt/harness";
import { apiServer, IDENT, jwtExpIn, posts } from "./harness5";

const USED = "ef:rt:used";
beforeEach(() => {
  setupEnv();
  vi.useFakeTimers();
  installLocks(fakeLocks());
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
  removeLocks();
  resetEnv();
});

const run = async (tab: Awaited<ReturnType<typeof newTab>>, tag: string, ms = 200) => {
  const p = outcome(tag, tab.api.apiFetch("/v1/orders/" + tag));
  await vi.advanceTimersByTimeAsync(ms);
  return p;
};

describe("AC-2 repros", () => {
  it("R2-3: mixed persist, abort at 120 s, other user signs out -> r1 POSTed once; tab-local copy wiped; buy-flow kept; other store untouched", async () => {
    const s = apiServer();
    seedUser("u-1", "old", "r1", sessionStorage);
    const A = await newTab();
    seedBuyFlow();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(60_000);
    seedUser("u-2", "acc-u2", "r-u2", localStorage);
    await vi.advanceTimersByTimeAsync(61_000);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(localStorage.getItem("ef_refresh")).toBe("r-u2"); // other user's session kept
    localStorage.clear();
    const b = outcome("B", A.api.apiFetch("/v1/orders/b"));
    await vi.advanceTimersByTimeAsync(121_000);
    expect(await b).toBe("B:AUTH_REQUIRED");
    expect(posts(s.calls)).toEqual(["r1"]);
    expect(sessionStorage.getItem("ef_token")).toBeNull();
    expect(sessionStorage.getItem("ef_refresh")).toBeNull();
    expect(sessionStorage.getItem("ef_user")).toBeNull();
    for (const k of BUY_FLOW_KEYS) expect(sessionStorage.getItem(k)).not.toBeNull();
    expect(sessionStorage.getItem("ef:lastuid")).not.toBeNull();
  });

  it("R2-S1: same-tab clearSession + setSession of the identical pair during the hung POST -> posts [r1]", async () => {
    const s = apiServer();
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(30_000);
    A.sess.clearSession();
    A.sess.setSession({ accessToken: "old", refreshToken: "r1", user: { id: "u-1", role: "buyer" } });
    await vi.advanceTimersByTimeAsync(91_000);
    await a;
    const b = outcome("B", A.api.apiFetch("/v1/orders/b"));
    await vi.advanceTimersByTimeAsync(121_000);
    expect(await b).toBe("B:AUTH_REQUIRED");
    expect(posts(s.calls)).toEqual(["r1"]);
    expect(A.sess.getSession()).toBeNull();
  });
});

describe("AC-2 marking", () => {
  it("2xx rotation r1->r2, then a same-tab setSession writes r1 back -> next 401 sends no POST, AUTH_REQUIRED", async () => {
    const s = apiServer(() => "new");
    s.setRefresh(() => json(IDENT()));
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    expect(await run(A, "a")).toBe("a:200");
    A.sess.setSession({ accessToken: "old", refreshToken: "r1", user: { id: "u-1", role: "buyer" } });
    expect(await run(A, "b")).toBe("b:AUTH_REQUIRED");
    expect(posts(s.calls)).toEqual(["r1"]);
  });

  it.each([400, 401, 403])("%i answer then the same pair is stored again -> no second POST", async (status) => {
    const s = apiServer();
    s.setRefresh(() => problem(status, "/problems/invalid-token"));
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    expect(await run(A, "a")).toBe("a:AUTH_REQUIRED");
    seedUser("u-1", "old", "r1");
    expect(await run(A, "b")).toBe("b:AUTH_REQUIRED");
    expect(posts(s.calls)).toEqual(["r1"]);
    expect(A.sess.getSession()).toBeNull();
  });

  it("2xx with unusable body marks the token used (Google X-15): restored pair gets no second POST", async () => {
    const s = apiServer();
    s.setRefresh(() => new Response("junk", { status: 200 }));
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    expect(await run(A, "a")).toBe("a:AUTH_REQUIRED");
    seedUser("u-1", "old", "r1");
    expect(await run(A, "b")).toBe("b:AUTH_REQUIRED");
    expect(posts(s.calls)).toEqual(["r1"]);
  });

  it("survives a reload of the tab (new module graph, same sessionStorage)", async () => {
    const s = apiServer();
    s.setRefresh(() => problem(401, "/problems/invalid-token"));
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    await run(A, "a");
    const B = await newTab();
    seedUser("u-1", "old", "r1");
    expect(await run(B, "b")).toBe("b:AUTH_REQUIRED");
    expect(posts(s.calls)).toEqual(["r1"]);
  });

  it("stale used copy on the PROACTIVE path -> AUTH_REQUIRED, no request sent at all", async () => {
    const s = apiServer();
    s.setRefresh(() => problem(401, "/problems/invalid-token"));
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    await run(A, "a");
    seedUser("u-1", jwtExpIn(5), "r1");
    const before = s.calls.length;
    expect(await run(A, "b")).toBe("b:AUTH_REQUIRED");
    expect(s.calls.length).toBe(before);
  });

  it("only the store holding the used token is wiped; the other store (another token) survives; no POST", async () => {
    const s = apiServer();
    s.setRefresh(() => problem(401, "/problems/invalid-token"));
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    await run(A, "a");
    seedUser("u-1", "old", "r1", localStorage);
    seedUser("u-1", "s-acc", "r5", sessionStorage);
    expect(await run(A, "b")).toBe("b:AUTH_REQUIRED");
    expect(posts(s.calls)).toEqual(["r1"]);
    expect(localStorage.getItem("ef_refresh")).toBeNull();
    expect(localStorage.getItem("ef_token")).toBeNull();
    expect(sessionStorage.getItem("ef_refresh")).toBe("r5");
    expect(sessionStorage.getItem("ef_token")).toBe("s-acc");
  });
});

describe("AC-2 record format", () => {
  it("record is a JSON array of at most 16 entries and never contains a raw token", async () => {
    const s = apiServer();
    s.setRefresh(() => problem(401, "/problems/invalid-token"));
    const A = await newTab();
    for (let i = 0; i < 20; i++) {
      seedUser("u-1", "old", "secret-token-" + i);
      await run(A, "x" + i, 10);
    }
    const raw = sessionStorage.getItem(USED);
    expect(raw).not.toBeNull();
    const arr = JSON.parse(raw as string) as unknown[];
    expect(Array.isArray(arr)).toBe(true);
    expect(arr).toHaveLength(16);
    expect(raw).not.toContain("secret-token");
    expect(posts(s.calls)).toHaveLength(20);
  });
});

describe("AC-2 GUARDS: not marked without a response", () => {
  it("fetch rejection (offline) then retry -> 2 POSTs of r1, second succeeds", async () => {
    const s = apiServer(() => "new");
    let n = 0;
    s.setRefresh(() => {
      if (n++ === 0) throw new TypeError("Failed to fetch");
      return json(IDENT());
    });
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    expect(await run(A, "a")).toBe("a:NETWORK_ERROR");
    expect(A.sess.getSession()?.refreshToken).toBe("r1");
    expect(await run(A, "b")).toBe("b:200");
    expect(posts(s.calls)).toEqual(["r1", "r1"]);
  });

  it.each([429, 500, 503])("%i then retry -> 2 POSTs of r1", async (status) => {
    const s = apiServer(() => "new");
    let n = 0;
    s.setRefresh(() => (n++ === 0 ? problem(status, "about:blank") : json(IDENT())));
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    expect((await run(A, "a")).startsWith("a:")).toBe(true);
    expect(A.sess.getSession()?.refreshToken).toBe("r1");
    expect(await run(A, "b")).toBe("b:200");
    expect(posts(s.calls)).toEqual(["r1", "r1"]);
  });

  it.each(["{not json", '"str"', '{"a":1}', "123", "null"])("corrupt record %s counts as empty (refresh works, 1 POST)", async (raw) => {
    const s = apiServer(() => "new");
    s.setRefresh(() => json(IDENT()));
    sessionStorage.setItem(USED, raw);
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    expect(await run(A, "a")).toBe("a:200");
    expect(posts(s.calls)).toEqual(["r1"]);
  });

  it("storage errors on the record key never throw (refresh works)", async () => {
    const s = apiServer(() => "new");
    s.setRefresh(() => json(IDENT()));
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const g = Storage.prototype.getItem;
    const st = Storage.prototype.setItem;
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(function (this: Storage, k: string) {
      if (k === USED) throw new Error("blocked");
      return g.call(this, k);
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(function (this: Storage, k: string, v: string) {
      if (k === USED) throw new Error("blocked");
      return st.call(this, k, v);
    });
    expect(await run(A, "a")).toBe("a:200");
    expect(posts(s.calls)).toEqual(["r1"]);
  });
});
