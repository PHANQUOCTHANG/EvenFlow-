/** AC-6 (X-14): only buyer|organizer|moderator|ops are accepted from storage. An unknown role means signed out for the UI
 *  (getSession/peekSession null, no Authorization header) but ef_refresh stays readable so logout() can revoke it (F-15).
 *  An unknown role in localStorage must NOT fall through to a sessionStorage session. Identity responses with an unknown or
 *  missing role keep the previous valid role, else buyer. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { logout } from "@/lib/api/auth";
import { getSession, getStoredRefreshToken, peekSession, sessionFromIdentity, type Role } from "@/lib/session";
import { installFetch, json, noContent, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { fakeLocks, installLocks, newTab, outcome, removeLocks } from "../debt/harness";
import { apiServer, bearers, IDENT, posts } from "./harness5";

beforeEach(setupEnv);
afterEach(() => {
  vi.useRealTimers();
  removeLocks();
  resetEnv();
});

const put = (store: Storage, role: unknown, extra: Record<string, unknown> = {}, id = "u", access = "acc", refresh: string | null = "ref") => {
  store.setItem("ef_token", access);
  if (refresh) store.setItem("ef_refresh", refresh);
  store.setItem("ef_user", JSON.stringify({ id, role, ...extra }));
};

describe("AC-6 stored role validation", () => {
  it.each(["admin", "Buyer", "BUYER", "", "superuser", 5, null, ["buyer"]])("role %j in localStorage -> null for getSession and peekSession", (role) => {
    put(localStorage, role);
    expect(getSession()).toBeNull();
    expect(peekSession()).toBeNull();
  });

  it("role admin in sessionStorage -> null too", () => {
    put(sessionStorage, "admin");
    expect(getSession()).toBeNull();
    expect(peekSession()).toBeNull();
  });

  it("X-14 repro: ef_user {id:'u', role:'admin'} is no longer returned", () => {
    localStorage.setItem("ef_token", "acc");
    localStorage.setItem("ef_user", JSON.stringify({ id: "u", role: "admin" }));
    expect(getSession()?.user.role).toBeUndefined();
  });

  it("invalid role in localStorage does NOT fall through to a valid sessionStorage session", () => {
    put(localStorage, "admin", {}, "u-evil", "acc-evil", "ref-evil");
    put(sessionStorage, "buyer", {}, "u-ok", "acc-ok", "ref-ok");
    expect(peekSession()).toBeNull();
    expect(getSession()).toBeNull();
  });

  it("GUARD: valid role in localStorage still wins over sessionStorage", () => {
    put(localStorage, "ops", {}, "u-l", "acc-l", "ref-l");
    put(sessionStorage, "buyer", {}, "u-s", "acc-s", "ref-s");
    expect(peekSession()?.user.id).toBe("u-l");
  });

  it("ef_refresh stays readable and logout() POSTs it for revocation, then clears", async () => {
    put(localStorage, "admin", {}, "u", "acc", "ref-revoke");
    expect(getStoredRefreshToken()).toBe("ref-revoke");
    const { calls } = installFetch(() => noContent());
    await logout();
    const lo = calls.find((c) => c.path.endsWith("/logout"));
    expect(lo?.body).toMatchObject({ refresh_token: "ref-revoke" });
    expect(localStorage.getItem("ef_refresh")).toBeNull();
  });

  it("no Authorization header is sent for a tampered role", async () => {
    vi.useFakeTimers();
    installLocks(fakeLocks());
    const s = apiServer();
    put(localStorage, "admin", {}, "u", "acc-tamper", "ref");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(130_000);
    await a;
    expect(bearers(s.calls)).toEqual([null]);
    expect(posts(s.calls)).toEqual([]);
  });

  it.each<Role>(["buyer", "organizer", "moderator", "ops"])("GUARD: known role %s accepted unchanged", (role) => {
    put(localStorage, role);
    expect(getSession()?.user.role).toBe(role);
  });

  it("GUARD: valid role plus an unknown extra field is accepted", () => {
    put(localStorage, "organizer", { nickname: "x", flags: [1] });
    expect(getSession()?.user.role).toBe("organizer");
  });
});

describe("AC-6 sessionFromIdentity", () => {
  const base = { id: "u-1", access_token: "a", refresh_token: "r" };
  it.each(["admin", "root", "", "BUYER"])("unknown role %j + valid prev -> prev role", (role) => {
    expect(sessionFromIdentity({ ...base, role }, { id: "u-1", role: "organizer" })?.user.role).toBe("organizer");
  });
  it.each(["admin", "", "BUYER"])("unknown role %j, no prev -> buyer", (role) => {
    expect(sessionFromIdentity({ ...base, role })?.user.role).toBe("buyer");
  });
  it("unknown role, prev role itself invalid -> buyer", () => {
    expect(sessionFromIdentity({ ...base, role: "admin" }, { id: "u-1", role: "root" as Role })?.user.role).toBe("buyer");
  });
  it("missing role + valid prev -> prev role; missing role, no prev -> buyer", () => {
    const noRole = { ...base } as unknown as Parameters<typeof sessionFromIdentity>[0];
    expect(sessionFromIdentity(noRole, { id: "u-1", role: "moderator" })?.user.role).toBe("moderator");
    expect(sessionFromIdentity(noRole)?.user.role).toBe("buyer");
  });
  it.each<Role>(["buyer", "organizer", "moderator", "ops"])("GUARD: known role %s stored unchanged (even with a different prev)", (role) => {
    expect(sessionFromIdentity({ ...base, role }, { id: "u-1", role: role === "ops" ? "buyer" : "ops" })?.user.role).toBe(role);
  });

  it("refresh response with role admin keeps the session's previous role", async () => {
    vi.useFakeTimers();
    installLocks(fakeLocks());
    const s = apiServer(() => "new");
    s.setRefresh(() => json(IDENT({ role: "admin" })));
    put(localStorage, "organizer", {}, "u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(100);
    expect(await a).toBe("A:200");
    expect(A.sess.getSession()?.user.role).toBe("organizer");
  });
});
