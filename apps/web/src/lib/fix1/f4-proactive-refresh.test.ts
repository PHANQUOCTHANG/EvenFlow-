/** Regression F-4 (X-1, X-11, C-9): proactive refresh must not depend on client clock vs JWT exp. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { apiFetch, ApiError } from "@/lib/api-client";
import { getSession } from "@/lib/session";
import { installFetch, json, problem, rejection, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

function jwt(exp: number, tag = "a") {
  const b = (o: unknown) => Buffer.from(JSON.stringify(o)).toString("base64url");
  return `${b({ alg: "HS256" })}.${b({ sub: "u-1", exp, tag })}.sig`;
}

beforeEach(setupEnv);
afterEach(() => {
  vi.useRealTimers();
  resetEnv();
});

const S = 1_800_000_000;

describe("F-4 proactive refresh", () => {
  it("skewed client clock (+20min): refresh once, then no refresh while expires_in has not elapsed locally", async () => {
    vi.useFakeTimers();
    vi.setSystemTime((S + 20 * 60) * 1000);
    let n = 0;
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        n++;
        return json({ id: "u-1", role: "buyer", access_token: jwt(S + 15 * 60, `t${n}`), refresh_token: `r${n + 1}`, expires_in: 900 });
      }
      return json({ ok: true });
    });
    seedSession(jwt(S + 15 * 60, "t0"), "r1");
    const refreshes = () => calls.filter((c) => c.path === "/api/v1/auth/refresh").length;
    await apiFetch("/v1/orders/o1");
    expect(refreshes()).toBe(1);
    vi.advanceTimersByTime(40_000);
    await apiFetch("/v1/orders/o2");
    await apiFetch("/v1/orders/o3");
    vi.advanceTimersByTime(60_000);
    await apiFetch("/v1/orders/o4");
    expect(refreshes()).toBe(1);
    expect(localStorage.getItem("ef_expires_at")).toBe(String(Date.now() - 100_000 + 900_000));
  });

  it("refreshes proactively only when stored expiresAt is within 30s", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(S * 1000);
    const { calls } = installFetch((c) =>
      c.path === "/api/v1/auth/refresh"
        ? json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2", expires_in: 900 })
        : json({ ok: true }),
    );
    seedSession("acc-1", "r1");
    localStorage.setItem("ef_expires_at", String(Date.now() + 120_000));
    await apiFetch("/v1/orders/o1");
    expect(calls.some((c) => c.path === "/api/v1/auth/refresh")).toBe(false);
    vi.advanceTimersByTime(100_000); // 20s left
    await apiFetch("/v1/orders/o2");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
    expect(calls[calls.length - 1].headers.get("Authorization")).toBe("Bearer new");
  });

  it("another tab's newer pair (own expires_at) is used without refreshing", async () => {
    vi.useFakeTimers();
    vi.setSystemTime((S + 20 * 60) * 1000);
    const { calls } = installFetch(() => json({ ok: true }));
    seedSession(jwt(S + 15 * 60, "fromTabB"), "rB");
    localStorage.setItem("ef_expires_at", String(Date.now() + 800_000));
    await apiFetch("/v1/orders/o3");
    expect(calls.some((c) => c.path === "/api/v1/auth/refresh")).toBe(false);
  });

  it("proactive refresh network failure still sends the original request with the current token", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(S * 1000);
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") throw new TypeError("Failed to fetch");
      return json({ ok: true });
    });
    seedSession(jwt(S + 20, "n1"), "r1");
    const res = await apiFetch("/v1/orders/o1");
    expect(res.status).toBe(200);
    expect(calls.filter((c) => c.path === "/v1/orders/o1")).toHaveLength(1);
  });

  it("proactive refresh 429 / 5xx still sends the original request", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(S * 1000);
    const { calls } = installFetch((c) =>
      c.path === "/api/v1/auth/refresh" ? problem(429, "/problems/rate-limited") : json({ ok: true }),
    );
    seedSession(jwt(S + 20, "n2"), "r1");
    const res = await apiFetch("/v1/orders/o1");
    expect(res.status).toBe(200);
    expect(calls.filter((c) => c.path === "/v1/orders/o1")).toHaveLength(1);
    expect(getSession()?.refreshToken).toBe("r1");
  });

  it("proactive refresh rejected (401) still surfaces AUTH_REQUIRED", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(S * 1000);
    installFetch((c) => (c.path === "/api/v1/auth/refresh" ? problem(401, "/problems/unauthorized") : json({})));
    seedSession(jwt(S + 20, "n3"), "r1");
    const err = (await rejection(apiFetch("/v1/orders/o1"))) as ApiError;
    expect(err.code).toBe("AUTH_REQUIRED");
  });
});
