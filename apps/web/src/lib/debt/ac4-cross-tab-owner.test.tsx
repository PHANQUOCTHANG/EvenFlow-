/** AC-4 (R4-3 / W-1): two tabs share localStorage, each has its own sessionStorage (buy-flow keys). When another tab
 *  switches the signed-in user, this tab clears ITS buy-flow keys on its next observation: a `storage` event for ef_user,
 *  or its next getSession() / setSession(), compared against the tab-local `ef:lastuid`. Same user keeps the keys. */
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { buyFlowRemaining, BUY_FLOW_KEYS, seedBuyFlow, seedUser } from "./harness";

beforeEach(setupEnv);
afterEach(() => {
  vi.useRealTimers();
  resetEnv();
});

async function tabSignedInAs(uid: string) {
  vi.resetModules();
  const sess = await import("@/lib/session");
  sess.setSession({ accessToken: "acc-" + uid, refreshToken: "r-" + uid, user: { id: uid, role: "buyer" } });
  seedBuyFlow();
  return sess;
}

/** What the OTHER tab does to the shared localStorage: sign in as `uid` (its sessionStorage is not ours). */
function otherTabSignsInAs(uid: string) {
  localStorage.clear();
  seedUser(uid, "acc-" + uid, "r-" + uid);
}

function fireStorage(key: string | null) {
  window.dispatchEvent(new StorageEvent("storage", { key, storageArea: localStorage }));
}

describe("AC-4 user switch made in another tab", () => {
  it("getSession(): stored user id differs from this tab's ef:lastuid -> buy-flow keys cleared, lastuid updated", async () => {
    const sess = await tabSignedInAs("u-A");
    expect(sessionStorage.getItem("ef:lastuid")).toBe("u-A");
    expect(buyFlowRemaining()).toHaveLength(BUY_FLOW_KEYS.length);
    otherTabSignsInAs("u-B");
    expect(sess.getSession()?.user.id).toBe("u-B");
    expect(buyFlowRemaining()).toEqual([]);
    expect(sessionStorage.getItem("ef:lastuid")).toBe("u-B");
  });

  it("W-1 setSession(): this tab then signs in as u-B (shared storage already holds u-B) -> keys of u-A are cleared", async () => {
    const sess = await tabSignedInAs("u-A");
    otherTabSignsInAs("u-B");
    sess.setSession({ accessToken: "acc-u-B2", refreshToken: "r-u-B2", user: { id: "u-B", role: "buyer" } });
    expect(buyFlowRemaining()).toEqual([]);
    expect(sessionStorage.getItem("ef:lastuid")).toBe("u-B");
  });

  it("storage event for ef_user (no getSession call by the test) -> keys cleared once the tab observes it", async () => {
    const sess = await tabSignedInAs("u-A");
    const { useAuth } = await import("@/hooks/use-auth");
    const hook = renderHook(() => useAuth());
    otherTabSignsInAs("u-B");
    act(() => fireStorage("ef_user"));
    expect(buyFlowRemaining()).toEqual([]);
    expect(sessionStorage.getItem("ef:lastuid")).toBe("u-B");
    expect(hook.result.current.user?.id).toBe("u-B");
    void sess;
  });

  it("GUARD: same user signed in by the other tab (token rotation) keeps the keys on getSession and on a storage event", async () => {
    const sess = await tabSignedInAs("u-A");
    const { useAuth } = await import("@/hooks/use-auth");
    renderHook(() => useAuth());
    localStorage.setItem("ef_token", "acc-rotated");
    localStorage.setItem("ef_refresh", "r-rotated");
    act(() => fireStorage("ef_token"));
    expect(sess.getSession()?.user.id).toBe("u-A");
    expect(buyFlowRemaining()).toHaveLength(BUY_FLOW_KEYS.length);
    expect(sessionStorage.getItem("ef:lastuid")).toBe("u-A");
  });

  it("GUARD (G-4): other tab signs out (no new user): getSession() is null and the keys / lastuid are kept", async () => {
    const sess = await tabSignedInAs("u-A");
    localStorage.clear();
    expect(sess.getSession()).toBeNull();
    expect(buyFlowRemaining()).toHaveLength(BUY_FLOW_KEYS.length);
    expect(sessionStorage.getItem("ef:lastuid")).toBe("u-A");
  });

  it("A -> B -> A in the other tab before this tab observes anything: same uid as lastuid, keys are kept", async () => {
    const sess = await tabSignedInAs("u-A");
    otherTabSignsInAs("u-B");
    otherTabSignsInAs("u-A");
    expect(sess.getSession()?.user.id).toBe("u-A");
    expect(buyFlowRemaining()).toHaveLength(BUY_FLOW_KEYS.length);
  });

  it("a storage event for an unrelated key does not touch buy-flow keys", async () => {
    await tabSignedInAs("u-A");
    const { useAuth } = await import("@/hooks/use-auth");
    renderHook(() => useAuth());
    act(() => fireStorage("some_other_key"));
    expect(buyFlowRemaining()).toHaveLength(BUY_FLOW_KEYS.length);
  });

  it("reconciliation never touches the auth keys in localStorage (safe while a refresh is in flight)", async () => {
    const sess = await tabSignedInAs("u-A");
    const { useAuth } = await import("@/hooks/use-auth");
    renderHook(() => useAuth());
    otherTabSignsInAs("u-B");
    const before = ["ef_token", "ef_refresh", "ef_user"].map((k) => localStorage.getItem(k));
    act(() => fireStorage("ef_user"));
    sess.getSession();
    expect(["ef_token", "ef_refresh", "ef_user"].map((k) => localStorage.getItem(k))).toEqual(before);
  });

  it("an explicit logout (buyFlow:true) still clears keys and lastuid (unchanged behaviour)", async () => {
    const sess = await tabSignedInAs("u-A");
    sess.clearSession({ buyFlow: true });
    expect(buyFlowRemaining()).toEqual([]);
    expect(sessionStorage.getItem("ef:lastuid")).toBeNull();
  });

  it("ef:lastuid absent (fresh tab, session already stored): getSession adopts the stored user id and KEEPS the keys", async () => {
    seedUser("u-B", "acc-u-B", "r-u-B");
    seedBuyFlow();
    vi.resetModules();
    const sess = await import("@/lib/session");
    expect(sessionStorage.getItem("ef:lastuid")).toBeNull();
    expect(sess.getSession()?.user.id).toBe("u-B");
    expect(sessionStorage.getItem("ef:lastuid")).toBe("u-B");
    expect(buyFlowRemaining()).toHaveLength(BUY_FLOW_KEYS.length);
  });
});
