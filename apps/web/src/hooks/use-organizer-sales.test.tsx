import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as organizerService from "@/lib/organizer-service";

import { useOrganizerSales } from "./use-organizer-sales";

describe("useOrganizerSales — Hook doanh số realtime (AC-4)", () => {
  const mockSnapshot: organizerService.RealtimeSalesSnapshot = {
    eventId: "evt-tech-summit-2026",
    timestamp: "12:00:00",
    grossRevenue: 150000000,
    totalTicketsSold: 250,
    totalQuota: 500,
    overallFillRatePercentage: 50,
    paidOrdersCount: 140,
    activeHoldsCount: 20,
    tierSales: [
      {
        tierId: "t1",
        name: "Vé Thường",
        price: 600000,
        quota: 500,
        sold: 250,
        activeHolds: 20,
        revenue: 150000000,
        fillRatePercentage: 50,
      },
    ],
  };

  beforeEach(() => {
    vi.useFakeTimers();
    vi.spyOn(organizerService, "getRealtimeSales").mockResolvedValue(mockSnapshot);
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.useRealTimers();
  });

  it("fetch dữ liệu lần đầu khi mount và tắt isLoading khi hoàn tất", async () => {
    const { result } = renderHook(() =>
      useOrganizerSales({ eventId: "evt-tech-summit-2026", initialAutoRefresh: false }),
    );

    expect(result.current.isLoading).toBe(true);
    expect(result.current.data).toBeNull();

    await act(async () => {
      await Promise.resolve(); // flush initial fetch promise
    });

    expect(result.current.isLoading).toBe(false);
    expect(result.current.data).toEqual(mockSnapshot);
    expect(result.current.lastUpdatedAt).toBeInstanceOf(Date);
    expect(organizerService.getRealtimeSales).toHaveBeenCalledWith("evt-tech-summit-2026");
  });

  it("dùng initialData nếu được truyền vào để tránh loading skeleton", () => {
    const { result } = renderHook(() =>
      useOrganizerSales({
        eventId: "evt-tech-summit-2026",
        initialData: mockSnapshot,
        initialAutoRefresh: false,
      }),
    );

    expect(result.current.isLoading).toBe(false);
    expect(result.current.data).toEqual(mockSnapshot);
  });

  it("tự động cập nhật theo chu kỳ polling <= 5s (AC-4)", async () => {
    renderHook(() =>
      useOrganizerSales({
        eventId: "evt-tech-summit-2026",
        pollingIntervalMs: 4000,
        initialAutoRefresh: true,
      }),
    );

    await act(async () => {
      await Promise.resolve();
    });
    expect(organizerService.getRealtimeSales).toHaveBeenCalledTimes(1);

    // Tiến hành nhảy 4s
    await act(async () => {
      vi.advanceTimersByTime(4000);
      await Promise.resolve();
    });

    expect(organizerService.getRealtimeSales).toHaveBeenCalledTimes(2);

    // Nhảy tiếp 4s
    await act(async () => {
      vi.advanceTimersByTime(4000);
      await Promise.resolve();
    });

    expect(organizerService.getRealtimeSales).toHaveBeenCalledTimes(3);
  });

  it("tắt autoRefresh thì ngưng polling timer", async () => {
    const { result } = renderHook(() =>
      useOrganizerSales({
        eventId: "evt-tech-summit-2026",
        pollingIntervalMs: 4000,
        initialAutoRefresh: true,
      }),
    );

    await act(async () => {
      await Promise.resolve();
    });
    expect(organizerService.getRealtimeSales).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.setAutoRefresh(false);
    });

    await act(async () => {
      vi.advanceTimersByTime(8000);
      await Promise.resolve();
    });

    // Không gọi thêm lần nào nữa do đã tắt autoRefresh
    expect(organizerService.getRealtimeSales).toHaveBeenCalledTimes(1);
  });

  it("hàm refresh gọi fetch thủ công và bật isRefreshing", async () => {
    const { result } = renderHook(() =>
      useOrganizerSales({
        eventId: "evt-tech-summit-2026",
        initialAutoRefresh: false,
      }),
    );

    await act(async () => {
      await Promise.resolve();
    });

    expect(organizerService.getRealtimeSales).toHaveBeenCalledTimes(1);

    await act(async () => {
      await result.current.refresh();
    });

    expect(organizerService.getRealtimeSales).toHaveBeenCalledTimes(2);
    expect(result.current.isRefreshing).toBe(false);
  });

  it("xử lý lỗi an toàn khi getRealtimeSales thất bại", async () => {
    vi.spyOn(organizerService, "getRealtimeSales").mockRejectedValue(
      new Error("Lỗi kết nối máy chủ"),
    );

    const { result } = renderHook(() =>
      useOrganizerSales({
        eventId: "evt-tech-summit-2026",
        initialAutoRefresh: false,
      }),
    );

    await act(async () => {
      await Promise.resolve();
    });

    expect(result.current.error?.message).toBe("Lỗi kết nối máy chủ");
    expect(result.current.isLoading).toBe(false);
  });
});
