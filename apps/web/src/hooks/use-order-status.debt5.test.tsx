/** AC-9: useIssuedTickets exposes authRequired (true on AUTH_REQUIRED, reset by retry()); `failed` keeps its F-13 value. */
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

import { useIssuedTickets } from "./use-order-status";

beforeEach(() => {
  setupEnv();
  seedSession("acc", "r1");
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  resetEnv();
});

const tick = async (ms = 1000) => {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
};

describe("useIssuedTickets authRequired", () => {
  it("401 -> authRequired true, pending false, forbidden false; failed keeps today's value (true: attempts exhausted)", async () => {
    installFetch(() => problem(401, "/problems/unauthorized"));
    const { result } = renderHook(() => useIssuedTickets("o1", true));
    await tick(5);
    await tick(1000);
    expect(result.current.authRequired).toBe(true);
    expect(result.current.pending).toBe(false);
    expect(result.current.forbidden).toBe(false);
    expect(result.current.failed).toBe(true);
  });

  it("retry() resets authRequired and issues again; success clears it", async () => {
    let ok = false;
    const { calls } = installFetch(() =>
      ok
        ? json({ tickets: [{ ticket_id: "tk-1", ticket_type_id: "t1", qr_payload: "qr" }] })
        : problem(401, "/problems/unauthorized"),
    );
    const { result } = renderHook(() => useIssuedTickets("o1", true));
    await tick(1000);
    expect(result.current.authRequired).toBe(true);
    ok = true;
    await act(async () => {
      result.current.retry();
    });
    await tick(1000);
    expect(calls.filter((c) => c.path.endsWith("/issue-tickets")).length).toBe(2);
    expect(result.current.authRequired).toBe(false);
  });

  it("GUARD: 403 -> forbidden true, authRequired false", async () => {
    installFetch(() => problem(403, "/problems/forbidden"));
    const { result } = renderHook(() => useIssuedTickets("o1", true));
    await tick(1000);
    expect(result.current.forbidden).toBe(true);
    expect(result.current.authRequired).toBe(false);
  });

  it("GUARD: not paid -> authRequired false and no request", async () => {
    const { calls } = installFetch(() => problem(401, "/problems/unauthorized"));
    const { result } = renderHook(() => useIssuedTickets("o1", false));
    await tick(5000);
    expect(result.current.authRequired).toBe(false);
    expect(calls).toHaveLength(0);
  });
});
