/** Regression G-5 (Y-7, R2-7) and G-6 (Y-8): payment Idempotency-Key scope and stale in-progress flag. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { initiatePayment, type InitiatePaymentParams } from "@/lib/payment-client";
import { installFetch, json, problem, rejection, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

const IN_PROGRESS = "urn:eventflow:problem:idempotency-in-progress";
const PAY: InitiatePaymentParams = {
  eventId: "e1",
  orderId: "o1",
  holdId: "h1",
  amount: 100,
  method: "sandbox",
  buyerName: "A",
  buyerEmail: "a@b.co",
  buyerPhone: "0912345678",
};
const OK = { payment_id: "p1", order_id: "o1", redirect_url: "/x", status: "PENDING" };

beforeEach(() => {
  setupEnv();
  seedSession();
});
afterEach(() => {
  vi.useRealTimers();
  resetEnv();
});

describe("G-5", () => {
  it("identical retry after NETWORK_ERROR reuses the key; a changed method/phone/amount gets a new key", async () => {
    let n = 0;
    const { calls } = installFetch(() => {
      n++;
      if (n === 1) throw new TypeError("Failed to fetch");
      if (n <= 4) return problem(503, "/problems/upstream-unavailable");
      return json(OK);
    });
    await rejection(initiatePayment(PAY));
    await rejection(initiatePayment(PAY));
    await rejection(initiatePayment({ ...PAY, method: "momo" }));
    await rejection(initiatePayment({ ...PAY, buyerPhone: "0987654321" }));
    const keys = calls.map((c) => c.headers.get("Idempotency-Key"));
    expect(keys[1]).toBe(keys[0]);
    expect(keys[2]).not.toBe(keys[0]);
    expect(keys[3]).not.toBe(keys[0]);
    expect(keys[3]).not.toBe(keys[2]);
    // Going back to the first body reuses its original key.
    await initiatePayment(PAY);
    expect(calls[4].headers.get("Idempotency-Key")).toBe(keys[0]);
  });
});

describe("G-6", () => {
  it("409 in-progress followed by a definitive 4xx forgets the key (next attempt gets a fresh one)", async () => {
    vi.useFakeTimers();
    let n = 0;
    const { calls } = installFetch(() => {
      n++;
      if (n === 1) return json({ type: IN_PROGRESS, title: "c", status: 409, detail: "x" }, 409, { "Retry-After": "1" });
      if (n === 2) return problem(422, "/problems/amount-mismatch");
      return json(OK);
    });
    const p = rejection(initiatePayment(PAY));
    await vi.advanceTimersByTimeAsync(2000);
    expect(((await p) as { code: string }).code).toBe("INVALID_INPUT");
    await initiatePayment(PAY);
    expect(calls[2].headers.get("Idempotency-Key")).not.toBe(calls[0].headers.get("Idempotency-Key"));
  });
});
