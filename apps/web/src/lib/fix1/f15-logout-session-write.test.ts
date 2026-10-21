/** Regression F-15 (X-12, X-13): logout with corrupt ef_user still revokes; setSession does not end logged out. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { logout } from "@/lib/api/auth";
import { getSession, setSession } from "@/lib/session";
import { installFetch, noContent, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(setupEnv);
afterEach(() => {
  vi.restoreAllMocks();
  resetEnv();
});

describe("F-15", () => {
  it("corrupt ef_user: logout still POSTs /logout with the stored refresh token", async () => {
    localStorage.setItem("ef_token", "acc");
    localStorage.setItem("ef_refresh", "ref-valid-on-server");
    localStorage.setItem("ef_user", "{not json");
    const { calls } = installFetch(() => noContent());
    await logout();
    const out = calls.filter((c) => c.path === "/api/v1/auth/logout");
    expect(out).toHaveLength(1);
    expect(out[0].body).toEqual({ refresh_token: "ref-valid-on-server" });
    expect(localStorage.getItem("ef_refresh")).toBeNull();
  });

  it("setItem throwing mid-write keeps the previous session", () => {
    setSession({ accessToken: "a1", refreshToken: "r1", user: { id: "u", role: "buyer" } });
    const orig = Storage.prototype.setItem;
    let n = 0;
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(function (this: Storage, k: string, v: string) {
      n++;
      if (n === 2) throw new DOMException("QuotaExceededError");
      return orig.call(this, k, v);
    });
    setSession({ accessToken: "a2", refreshToken: "r2", user: { id: "u", role: "buyer" } });
    vi.restoreAllMocks();
    expect(getSession()?.accessToken).toBe("a1");
    expect(getSession()?.refreshToken).toBe("r1");
  });

  it("switching storage keeps old copy until the new write succeeded", () => {
    setSession({ accessToken: "a1", refreshToken: "r1", user: { id: "u", role: "buyer" } }, { persist: "local" });
    const orig = Storage.prototype.setItem;
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(function (this: Storage, k: string, v: string) {
      if (this === window.sessionStorage) throw new DOMException("QuotaExceededError");
      return orig.call(this, k, v);
    });
    setSession({ accessToken: "a2", refreshToken: "r2", user: { id: "u", role: "buyer" } }, { persist: "session" });
    vi.restoreAllMocks();
    expect(getSession()?.accessToken).toBe("a1");
  });
});
