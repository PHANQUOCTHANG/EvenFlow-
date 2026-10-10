import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as paymentClient from "@/lib/payment-client";
import { PaymentClientError } from "@/lib/payment-client";
import { usePaymentSession, validatePaymentForm } from "./use-payment-session";

describe("usePaymentSession hook (EVF-114, BR-O2, BR-O5)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("validatePaymentForm bắt lỗi khi thông tin trống hoặc email sai định dạng", () => {
    const emptyErrors = validatePaymentForm({
      buyerName: "",
      buyerEmail: "",
      buyerPhone: "",
      method: "sandbox",
    });

    expect(emptyErrors.buyerName).toBeDefined();
    expect(emptyErrors.buyerEmail).toBeDefined();
    expect(emptyErrors.buyerPhone).toBeDefined();

    const invalidEmailErrors = validatePaymentForm({
      buyerName: "Nguyen Van A",
      buyerEmail: "invalid-email",
      buyerPhone: "0901234567",
      method: "sandbox",
    });

    expect(invalidEmailErrors.buyerEmail).toBeDefined();
    expect(invalidEmailErrors.buyerName).toBeUndefined();
    expect(invalidEmailErrors.buyerPhone).toBeUndefined();
  });

  it("chặn submit khi hold đã hết hạn (BR-O2)", async () => {
    // 5 phút trước -> đã expired
    const pastExpiresAt = new Date(Date.now() - 5 * 60 * 1000).toISOString();

    const { result } = renderHook(() =>
      usePaymentSession({
        eventId: "evt-1",
        orderId: "ord-1",
        holdId: "hld-1",
        amount: 200000,
        expiresAt: pastExpiresAt,
      }),
    );

    expect(result.current.holdExpired).toBe(true);

    let submitResult;
    await act(async () => {
      submitResult = await result.current.submitPayment();
    });

    expect(submitResult).toBeNull();
    expect(result.current.error).toContain("hết hạn");
  });

  it("submit thành công khi form hợp lệ và hold còn hạn (BR-O5)", async () => {
    const futureExpiresAt = new Date(Date.now() + 10 * 60 * 1000).toISOString();
    const onSuccess = vi.fn();

    const initiateSpy = vi.spyOn(paymentClient, "initiatePayment").mockResolvedValue({
      paymentId: "pay-100",
      orderId: "ord-100",
      redirectUrl: "/checkout/evt-1/result?orderId=ord-100",
      status: "PENDING",
    });

    const { result } = renderHook(() =>
      usePaymentSession({
        eventId: "evt-1",
        orderId: "ord-100",
        holdId: "hld-100",
        amount: 500000,
        expiresAt: futureExpiresAt,
        onSuccess,
      }),
    );

    act(() => {
      result.current.setFormData({
        buyerName: "Nguyen Van A",
        buyerEmail: "nguyenvana@example.com",
        buyerPhone: "0901234567",
        method: "vnpay",
      });
    });

    let res;
    await act(async () => {
      res = await result.current.submitPayment();
    });

    expect(initiateSpy).toHaveBeenCalledTimes(1);
    expect(res).toEqual({
      paymentId: "pay-100",
      orderId: "ord-100",
      redirectUrl: "/checkout/evt-1/result?orderId=ord-100",
      status: "PENDING",
    });
    expect(onSuccess).toHaveBeenCalledTimes(1);
  });

  it("xử lý khi backend trả về lỗi HOLD_EXPIRED trong lúc submit (BR-O2)", async () => {
    const futureExpiresAt = new Date(Date.now() + 5 * 60 * 1000).toISOString();

    vi.spyOn(paymentClient, "initiatePayment").mockRejectedValue(
      new PaymentClientError("Hold expired", "HOLD_EXPIRED", 409),
    );

    const { result } = renderHook(() =>
      usePaymentSession({
        eventId: "evt-1",
        orderId: "ord-1",
        holdId: "hld-1",
        amount: 500000,
        expiresAt: futureExpiresAt,
      }),
    );

    act(() => {
      result.current.setFormData({
        buyerName: "Nguyen Van A",
        buyerEmail: "nguyenvana@example.com",
        buyerPhone: "0901234567",
        method: "momo",
      });
    });

    await act(async () => {
      await result.current.submitPayment();
    });

    expect(result.current.holdExpired).toBe(true);
    expect(result.current.error).toContain("hết hạn trong lúc thanh toán");
  });
});
