/** Regression F-7 (X-5, C-7): 409 idempotency-in-progress waits Retry-After and retries with the SAME key; hold key is stable. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { requestHold } from "@/lib/hold-client";
import { initiatePayment } from "@/lib/payment-client";
import { installFetch, json, problem, rejection, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

const IN_PROGRESS = "urn:eventflow:problem:idempotency-in-progress";
const inProgress = () => json({ type: IN_PROGRESS, title: "c", status: 409, detail: "x" }, 409, { "Retry-After": "1" });
const HOLD = { eventId: "e1", ticketTypeId: "t1", quantity: 2, queueToken: "q" };
const HOLD_OK = { order_id: "o1", hold_id: "h1", expires_at: "2030-01-01T00:00:00Z" };
const PAY = {
  eventId: "e1", orderId: "o1", holdId: "h1", amount: 1, method: "sandbox" as const,
  buyerName: "a", buyerEmail: "a@b.c", buyerPhone: "0912345678",
};
const PAY_OK = { payment_id: "p1", order_id: "o1", redirect_url: "/x", status: "PENDING" };
const code = (e: unknown) => (e as { type?: string }).type;

beforeEach(() => {
  setupEnv();
  seedSession();
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  resetEnv();
});

describe("F-7 hold", () => {
  it("in-progress then success: waits Retry-After and resends the same key", async () => {
    let n = 0;
    const { calls } = installFetch(() => (++n === 1 ? inProgress() : json(HOLD_OK)));
    const p = requestHold(HOLD);
    await vi.advanceTimersByTimeAsync(500);
    expect(calls).toHaveLength(1); // still waiting
    await vi.advanceTimersByTimeAsync(600);
    const r = await p;
    expect(r.holdId).toBe("h1");
    expect(calls).toHaveLength(2);
    expect(calls[0].headers.get("Idempotency-Key")).toBe(calls[1].headers.get("Idempotency-Key"));
  });

  it("in-progress forever: at most 3 sends then AMBIGUOUS (not SOLD_OUT)", async () => {
    const { calls } = installFetch(() => inProgress());
    const p = rejection(requestHold(HOLD));
    await vi.advanceTimersByTimeAsync(10_000);
    expect(code(await p)).toBe("AMBIGUOUS");
    expect(calls).toHaveLength(3);
    expect(new Set(calls.map((c) => c.headers.get("Idempotency-Key"))).size).toBe(1);
  });

  it("key is stable across user retries after AMBIGUOUS, then rotates after success", async () => {
    let n = 0;
    const { calls } = installFetch(() => (++n <= 2 ? problem(503, "/problems/upstream-unavailable") : json(HOLD_OK)));
    await rejection(requestHold(HOLD));
    await rejection(requestHold(HOLD));
    await requestHold(HOLD);
    const k = calls.map((c) => c.headers.get("Idempotency-Key"));
    expect(k[0]).toBe(k[1]);
    expect(k[1]).toBe(k[2]);
    installFetch(() => json(HOLD_OK));
    const again = installFetch(() => json(HOLD_OK));
    await requestHold(HOLD);
    expect(again.calls[0].headers.get("Idempotency-Key")).not.toBe(k[0]);
  });

  it("different tier/quantity gets a different key; definitive error rotates the key", async () => {
    const a = installFetch(() => problem(409, "/problems/conflict"));
    await rejection(requestHold(HOLD));
    await rejection(requestHold(HOLD));
    expect(a.calls[0].headers.get("Idempotency-Key")).not.toBe(a.calls[1].headers.get("Idempotency-Key"));
    const b = installFetch(() => problem(503, "/problems/x"));
    await rejection(requestHold(HOLD));
    await rejection(requestHold({ ...HOLD, quantity: 3 }));
    expect(b.calls[0].headers.get("Idempotency-Key")).not.toBe(b.calls[1].headers.get("Idempotency-Key"));
  });

  it("other 409 stays SOLD_OUT", async () => {
    installFetch(() => problem(409, "/problems/conflict"));
    expect(code(await rejection(requestHold(HOLD)))).toBe("SOLD_OUT");
  });
});

describe("F-7 payment", () => {
  it("in-progress then success: same key, waits, no HOLD_EXPIRED", async () => {
    let n = 0;
    const { calls } = installFetch(() => (++n === 1 ? inProgress() : json(PAY_OK)));
    const p = initiatePayment(PAY);
    await vi.advanceTimersByTimeAsync(1100);
    expect((await p).paymentId).toBe("p1");
    expect(calls[0].headers.get("Idempotency-Key")).toBe(calls[1].headers.get("Idempotency-Key"));
  });

  it("in-progress forever -> AMBIGUOUS after 3 sends; other 409 -> HOLD_EXPIRED", async () => {
    const a = installFetch(() => inProgress());
    const p = rejection(initiatePayment(PAY));
    await vi.advanceTimersByTimeAsync(10_000);
    expect(code(await p)).toBe("AMBIGUOUS");
    expect(a.calls).toHaveLength(3);
    installFetch(() => problem(409, "/problems/conflict"));
    expect(code(await rejection(initiatePayment({ ...PAY, orderId: "o2" })))).toBe("HOLD_EXPIRED");
  });
});
