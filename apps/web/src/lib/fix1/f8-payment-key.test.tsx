/** Regression F-8 (X-6): payment Idempotency-Key is stable per order across retries after NETWORK_ERROR. */
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { usePaymentSession } from "@/hooks/use-payment-session";
import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession();
});
afterEach(resetEnv);

const OK = { payment_id: "p2", order_id: "o1", redirect_url: "/x", status: "PENDING" };

async function submitTwice(second: () => Response | never) {
  let n = 0;
  const { calls } = installFetch(() => {
    n++;
    if (n === 1) throw new TypeError("Failed to fetch");
    return second();
  });
  const { result } = renderHook(() => usePaymentSession({ eventId: "e1", orderId: "o1", holdId: "h1", amount: 100 }));
  act(() => result.current.setFormData({ buyerName: "A", buyerEmail: "a@b.c", buyerPhone: "0912345678", method: "sandbox" }));
  await act(async () => {
    await result.current.submitPayment();
  });
  await act(async () => {
    await result.current.submitPayment();
  });
  await act(async () => {
    await result.current.submitPayment();
  });
  return calls.filter((c) => c.path === "/v1/events/e1/payments");
}

describe("F-8", () => {
  it("same key is re-sent after NETWORK_ERROR", async () => {
    const p = await submitTwice(() => json(OK));
    expect(p.length).toBeGreaterThanOrEqual(2);
    expect(p[0].headers.get("Idempotency-Key")).toBe(p[1].headers.get("Idempotency-Key"));
  });

  it("key is rotated after a definitive 4xx", async () => {
    const p = await submitTwice(() => problem(422, "/problems/validation", "bad"));
    expect(p[0].headers.get("Idempotency-Key")).toBe(p[1].headers.get("Idempotency-Key"));
    expect(p[2].headers.get("Idempotency-Key")).not.toBe(p[1].headers.get("Idempotency-Key"));
  });

  it("key is cleared after success", async () => {
    await submitTwice(() => json(OK));
    expect(sessionStorage.getItem("ef:pay:o1")).toBeNull();
  });
});
