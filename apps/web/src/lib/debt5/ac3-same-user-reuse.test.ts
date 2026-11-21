/** AC-3 (X-4, R2-4): the POST of r1 ends in 400/401/403, in the 120 s abort or in an AC-1 body failure, and meanwhile the
 *  stored session is the SAME user with a DIFFERENT refresh token and an unchanged epoch -> owner and joiners get the stored
 *  access token and apiFetch retries the original request once with it. Otherwise AUTH_REQUIRED and the store is untouched. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { fakeLocks, installLocks, newTab, outcome, removeLocks, seedUser } from "../debt/harness";
import { apiCalls, apiServer, posts } from "./harness5";

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

const withBearer = (calls: ReturnType<typeof apiServer>["calls"], path: string, token: string) =>
  apiCalls(calls).filter((c) => c.path === path && c.headers.get("Authorization") === `Bearer ${token}`);

describe("AC-3 abort path (X-4)", () => {
  it("owner and joiner both get acc-9 written by another tab at 100 s; 1 POST; original retried once with acc-9", async () => {
    const s = apiServer(() => "acc-9");
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(99_000);
    const j = outcome("J", A.api.apiFetch("/v1/orders/j"));
    await vi.advanceTimersByTimeAsync(1_000);
    seedUser("u-1", "acc-9", "r9");
    await vi.advanceTimersByTimeAsync(21_000);
    expect(await a).toBe("A:200");
    expect(await j).toBe("J:200");
    expect(posts(s.calls)).toEqual(["r1"]);
    expect(withBearer(s.calls, "/v1/orders/a", "acc-9")).toHaveLength(1);
    expect(withBearer(s.calls, "/v1/orders/j", "acc-9")).toHaveLength(1);
    expect(A.sess.getSession()?.refreshToken).toBe("r9");
  });

  it("GUARD: another user stored at abort -> AUTH_REQUIRED, stored session untouched, original not retried with its token", async () => {
    const s = apiServer(() => "acc-u2");
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(100_000);
    seedUser("u-2", "acc-u2", "r-u2");
    await vi.advanceTimersByTimeAsync(21_000);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(A.sess.getSession()?.user.id).toBe("u-2");
    expect(A.sess.getSession()?.refreshToken).toBe("r-u2");
    expect(withBearer(s.calls, "/v1/orders/a", "acc-u2")).toHaveLength(0);
  });

  it("GUARD: session removed at abort -> AUTH_REQUIRED", async () => {
    apiServer();
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(100_000);
    localStorage.clear();
    await vi.advanceTimersByTimeAsync(21_000);
    expect(await a).toBe("A:AUTH_REQUIRED");
  });

  it("GUARD: same-tab sign-out + same-user login (epoch changed) at abort -> AUTH_REQUIRED, new session kept", async () => {
    apiServer(() => "acc-9");
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(50_000);
    A.sess.clearSession();
    A.sess.setSession({ accessToken: "acc-9", refreshToken: "r9", user: { id: "u-1", role: "buyer" } });
    await vi.advanceTimersByTimeAsync(71_000);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(A.sess.getSession()?.refreshToken).toBe("r9");
  });
});

describe("AC-3 answered path (R2-4)", () => {
  it.each([
    ["401", () => problem(401, "/problems/invalid-token")],
    ["400", () => problem(400, "/problems/invalid-token")],
    ["403", () => problem(403, "/problems/forbidden")],
    ["200 with bad body (AC-1)", () => new Response("junk", { status: 200 })],
  ] as Array<[string, () => Response]>)("%s while same user signed in again (r9) -> A:200 with acc-9, then B:200, 1 POST", async (_n, mk) => {
    const s = apiServer(() => "acc-9");
    s.controlled();
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(10_000);
    seedUser("u-1", "acc-9", "r9");
    s.answer(mk());
    await vi.advanceTimersByTimeAsync(1_000);
    expect(await a).toBe("A:200");
    expect(withBearer(s.calls, "/v1/orders/a", "acc-9")).toHaveLength(1);
    const b = outcome("B", A.api.apiFetch("/v1/orders/b"));
    await vi.advanceTimersByTimeAsync(100);
    expect(await b).toBe("B:200");
    expect(posts(s.calls)).toEqual(["r1"]);
    expect(A.sess.getSession()?.refreshToken).toBe("r9");
  });

  it("GUARD: 401 while ANOTHER user is stored -> AUTH_REQUIRED, stored session untouched, no retry", async () => {
    const s = apiServer(() => "acc-u2");
    s.controlled();
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(10_000);
    seedUser("u-2", "acc-u2", "r-u2");
    s.answer(problem(401, "/problems/invalid-token"));
    await vi.advanceTimersByTimeAsync(1_000);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(A.sess.getSession()?.refreshToken).toBe("r-u2");
    expect(withBearer(s.calls, "/v1/orders/a", "acc-u2")).toHaveLength(0);
  });

  it("GUARD: 401 after same-tab sign-out + same-user login (epoch changed) -> AUTH_REQUIRED, new session kept", async () => {
    const s = apiServer(() => "acc-9");
    s.controlled();
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(5_000);
    A.sess.clearSession();
    A.sess.setSession({ accessToken: "acc-9", refreshToken: "r9", user: { id: "u-1", role: "buyer" } });
    s.answer(problem(401, "/problems/invalid-token"));
    await vi.advanceTimersByTimeAsync(1_000);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(A.sess.getSession()?.refreshToken).toBe("r9");
  });

  it("GUARD: 401 and the stored session is still the posted one -> cleared, AUTH_REQUIRED", async () => {
    const s = apiServer();
    s.setRefresh(() => problem(401, "/problems/invalid-token"));
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(100);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(A.sess.getSession()).toBeNull();
  });
});
