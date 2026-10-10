import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as paymentClient from "@/lib/payment-client";
import { useOrderStatus } from "./use-order-status";

describe("useOrderStatus hook (EVF-114, BR-O6)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.clearAllTimers();
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("khởi tạo với initialDetails và nhận diện đúng trạng thái PAID", () => {
    const mockOrder: paymentClient.OrderDetails = {
      orderId: "ord-1",
      eventId: "evt-1",
      eventTitle: "Rock Fest",
      venue: "Hanoi",
      tierName: "VIP",
      quantity: 1,
      totalAmount: 500000,
      status: "PAID",
    };

    const { result } = renderHook(() =>
      useOrderStatus({
        orderId: "ord-1",
        eventId: "evt-1",
        initialDetails: mockOrder,
      }),
    );

    expect(result.current.status).toBe("PAID");
    expect(result.current.isPendingWebhook).toBe(false);
    expect(result.current.order).toEqual(mockOrder);
  });

  it("polling định kỳ khi đơn ở trạng thái PENDING và dừng lại khi PAID (BR-O6)", async () => {
    const pendingOrder: paymentClient.OrderDetails = {
      orderId: "ord-pending",
      eventId: "evt-1",
      eventTitle: "Concert",
      venue: "HCM",
      tierName: "Standard",
      quantity: 2,
      totalAmount: 400000,
      status: "PENDING",
    };

    const paidOrder: paymentClient.OrderDetails = {
      ...pendingOrder,
      status: "PAID",
      paidAt: "2026-10-10T21:00:00Z",
    };

    let callCount = 0;
    vi.spyOn(paymentClient, "getOrderDetails").mockImplementation(async () => {
      callCount++;
      if (callCount >= 2) {
        return paidOrder;
      }
      return pendingOrder;
    });

    const { result } = renderHook(() =>
      useOrderStatus({
        orderId: "ord-pending",
        eventId: "evt-1",
        initialDetails: pendingOrder,
        pollIntervalMs: 1000,
        maxPollAttempts: 5,
      }),
    );

    expect(result.current.status).toBe("PENDING");
    expect(result.current.isPendingWebhook).toBe(true);

    // Kích hoạt nhịp poll đầu tiên
    await act(async () => {
      vi.advanceTimersByTime(1000);
    });

    expect(result.current.pollAttempts).toBe(1);
    expect(result.current.status).toBe("PENDING");

    // Kích hoạt nhịp poll thứ hai -> chuyển sang PAID
    await act(async () => {
      vi.advanceTimersByTime(1000);
    });

    expect(result.current.pollAttempts).toBe(2);
    expect(result.current.status).toBe("PAID");
    expect(result.current.isPendingWebhook).toBe(false);

    // Tiến thêm thời gian, không được gọi thêm lần nào vì đã dừng polling
    await act(async () => {
      vi.advanceTimersByTime(3000);
    });

    expect(result.current.pollAttempts).toBe(2);
  });

  it("dừng polling khi đạt maxPollAttempts nhưng vẫn giữ nguyên trạng thái PENDING (webhook chậm)", async () => {
    const pendingOrder: paymentClient.OrderDetails = {
      orderId: "ord-slow",
      eventId: "evt-1",
      eventTitle: "Concert",
      venue: "HCM",
      tierName: "Standard",
      quantity: 1,
      totalAmount: 200000,
      status: "PENDING",
    };

    vi.spyOn(paymentClient, "getOrderDetails").mockResolvedValue(pendingOrder);

    const { result } = renderHook(() =>
      useOrderStatus({
        orderId: "ord-slow",
        eventId: "evt-1",
        initialDetails: pendingOrder,
        pollIntervalMs: 500,
        maxPollAttempts: 2,
      }),
    );

    await act(async () => {
      vi.advanceTimersByTime(500);
    });
    expect(result.current.pollAttempts).toBe(1);

    await act(async () => {
      vi.advanceTimersByTime(500);
    });
    expect(result.current.pollAttempts).toBe(2);

    // Lần tiếp theo không tăng nữa
    await act(async () => {
      vi.advanceTimersByTime(1000);
    });
    expect(result.current.pollAttempts).toBe(2);
    expect(result.current.status).toBe("PENDING");
  });
});
