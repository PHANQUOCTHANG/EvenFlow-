/** AC-6: usePaymentSession exposes a `forbidden` flag when initiatePayment fails with FORBIDDEN. */
import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as paymentClient from "@/lib/payment-client";

import { usePaymentSession } from "./use-payment-session";

afterEach(() => vi.restoreAllMocks());

const err = (type: string, status: number) =>
  new paymentClient.PaymentClientError("x", type as paymentClient.PaymentClientErrorType, status);

async function submitWith(rejection: unknown) {
  vi.spyOn(paymentClient, "initiatePayment").mockRejectedValue(rejection);
  const hook = renderHook(() => usePaymentSession({ eventId: "e1", orderId: "o1", holdId: "h1", amount: 100 }));
  act(() =>
    hook.result.current.setFormData({
      buyerName: "A",
      buyerEmail: "a@b.co",
      buyerPhone: "0901234567",
      method: "sandbox",
    }),
  );
  await act(async () => {
    await hook.result.current.submitPayment();
  });
  return hook.result.current as ReturnType<typeof usePaymentSession> & { forbidden?: boolean };
}

describe("usePaymentSession forbidden (AC-6)", () => {
  it("FORBIDDEN error -> forbidden flag true, not authRequired, not holdExpired", async () => {
    const r = await submitWith(err("FORBIDDEN", 403));
    expect(r.forbidden).toBe(true);
    expect(r.authRequired).toBe(false);
    expect(r.holdExpired).toBe(false);
  });

  it("GUARD: UNKNOWN 403 leaves forbidden false", async () => {
    const r = await submitWith(err("UNKNOWN", 403));
    expect(r.forbidden ?? false).toBe(false);
    expect(r.error).not.toBeNull();
  });
});
