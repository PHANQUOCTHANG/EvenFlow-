/** X-4 (AC-6, plan P-6): a localStorage.getItem that throws is "absent", not "invalid" -> fall through to sessionStorage. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { fakeLocks, installLocks, newTab, removeLocks, seedUser } from "../debt/harness";

beforeEach(() => {
  setupEnv();
  installLocks(fakeLocks());
});
afterEach(() => {
  vi.restoreAllMocks();
  removeLocks();
  resetEnv();
});

function throwOnLocal(keys: string[]) {
  const g = Storage.prototype.getItem;
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(function (this: Storage, k: string) {
    if (this === localStorage && keys.includes(k)) throw new Error("blocked");
    return g.call(this, k);
  });
}

describe("X-4 throwing localStorage.getItem", () => {
  it.each([["ef_token"], ["ef_user"]])("getItem(%s) throws on localStorage -> valid sessionStorage session is used", async (key) => {
    seedUser("u-1", "acc-s", "r-s", sessionStorage);
    throwOnLocal([key]);
    const A = await newTab();
    expect(A.sess.peekSession()?.user.id ?? null).toBe("u-1");
    expect(A.sess.peekSession()?.accessToken ?? null).toBe("acc-s");
  });

  it("GUARD: a present but corrupt localStorage session is still invalid (no fall-through)", async () => {
    seedUser("u-1", "acc-s", "r-s", sessionStorage);
    localStorage.setItem("ef_token", "acc-l");
    localStorage.setItem("ef_user", "{not json");
    const A = await newTab();
    expect(A.sess.peekSession()).toBeNull();
  });
});
