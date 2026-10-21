/** F-4 (R-4 / X-2): a new submit resets the sticky `forbidden` flag. */
import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import * as paymentClient from "@/lib/payment-client";

import { usePaymentSession } from "./use-payment-session";

afterEach(() => vi.restoreAllMocks());

describe("usePaymentSession forbidden reset (F-4)", () => {
  it("FORBIDDEN then AUTH_REQUIRED: forbidden false, authRequired true", async () => {
    const spy = vi.spyOn(paymentClient, "initiatePayment");
    spy.mockRejectedValueOnce(new paymentClient.PaymentClientError("x", "FORBIDDEN", 403));
    const h = renderHook(() => usePaymentSession({ eventId: "e1", orderId: "o1", holdId: "h1", amount: 100 }));
    act(() =>
      h.result.current.setFormData({
        buyerName: "A",
        buyerEmail: "a@b.co",
        buyerPhone: "0901234567",
        method: "sandbox",
      }),
    );
    await act(async () => {
      await h.result.current.submitPayment();
    });
    expect(h.result.current.forbidden).toBe(true);
    spy.mockRejectedValueOnce(new paymentClient.PaymentClientError("expired", "AUTH_REQUIRED", 401));
    await act(async () => {
      await h.result.current.submitPayment();
    });
    expect(h.result.current.forbidden).toBe(false);
    expect(h.result.current.authRequired).toBe(true);
  });
});
