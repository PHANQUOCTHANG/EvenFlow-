/**
 * ops-warroom AC-30: SalesDashboard on real data (hook + component, getRealtimeSales mocked at the service boundary).
 * ASSUMPTIONS: the dashboard is rendered WITHOUT initialSalesData and fetches itself; auto-refresh is ON by default at 4 s;
 * the empty state text matches /chưa (bán|có)[^.]*(vé|đơn)/i; existing labels (tabs, "Lỗi cập nhật doanh số") are kept.
 */
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { getRealtimeSales, type OrganizerEvent, type RealtimeSalesSnapshot } from "@/lib/organizer-service";

import { SalesDashboard } from "./sales-dashboard";

vi.mock("@/lib/organizer-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/organizer-service")>();
  return { ...actual, getRealtimeSales: vi.fn() };
});
const mockSales = vi.mocked(getRealtimeSales);

const event: OrganizerEvent = {
  id: "evt-1", slug: "s", title: "Rock Fest", description: "", venue: "HN", startsAt: "2026-12-05T08:30:00Z",
  saleStartAt: "2026-09-01T09:00:00Z", state: "ON_SALE", organizerId: "o",
  tiers: [{ id: "t1", name: "Standard Pass", price: 900000, quota: 1000 }],
  createdAt: "2026-08-01T00:00:00Z", updatedAt: "2026-09-01T00:00:00Z",
} as OrganizerEvent;

function snap(over: Partial<RealtimeSalesSnapshot> = {}): RealtimeSalesSnapshot {
  return {
    isSample: false, eventId: "evt-1", timestamp: "2026-11-01T10:00:00Z", grossRevenue: 123000000,
    totalTicketsSold: 820, totalQuota: 1200, overallFillRatePercentage: 68, paidOrdersCount: 455, activeHoldsCount: 48,
    tierSales: [{ tierId: "t1", name: "Standard Pass", price: 900000, quota: 1000, sold: 700, activeHolds: 40, revenue: 105000000, fillRatePercentage: 70 }],
    postSaleReport: {
      averageOrderValue: 270330, peakSalesVelocityTps: null,
      paymentBreakdown: [
        { provider: "vnpay", orders: 300, amount: 80000000 },
        { provider: "sandbox", orders: 155, amount: 43000000 },
      ],
    },
    ...over,
  } as RealtimeSalesSnapshot;
}

async function flush() {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  mockSales.mockReset();
  mockSales.mockResolvedValue(snap());
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("SalesDashboard real data (AC-30)", () => {
  it("shows no sample badge or sample alert", async () => {
    render(<SalesDashboard event={event} />);
    await flush();
    expect(screen.queryByText(/Dữ liệu mẫu/)).toBeNull();
    expect(screen.queryByText(/Mẫu — chưa phải doanh thu thật/)).toBeNull();
    expect(screen.queryByText(/Số liệu mẫu/)).toBeNull();
    expect(screen.getByText("Standard Pass")).toBeInTheDocument();
  });

  it("auto-refresh is on by default and polls within 5 s (4 s)", async () => {
    render(<SalesDashboard event={event} />);
    await flush();
    expect(mockSales).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: /Tắt tự động cập nhật/ })).toBeInTheDocument();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(3900);
    });
    expect(mockSales).toHaveBeenCalledTimes(1);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(200);
    });
    expect(mockSales).toHaveBeenCalledTimes(2);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000);
    });
    expect(mockSales).toHaveBeenCalledTimes(3);
  });

  it("peak TPS shows 'Chưa có dữ liệu' instead of a number", async () => {
    render(<SalesDashboard event={event} />);
    await flush();
    fireEvent.click(screen.getByRole("tab", { name: /Báo cáo tổng kết sau bán/ }));
    const card = screen.getByText("Tốc độ giao dịch cao điểm").parentElement as HTMLElement;
    expect(card).toHaveTextContent("Chưa có dữ liệu");
    expect(card).not.toHaveTextContent(/TPS/);
    expect(card).not.toHaveTextContent(/NaN|null/);
  });

  it("renders the payment breakdown from the list, any provider name", async () => {
    render(<SalesDashboard event={event} />);
    await flush();
    fireEvent.click(screen.getByRole("tab", { name: /Báo cáo tổng kết sau bán/ }));
    expect(screen.getByText(/vnpay/i)).toBeInTheDocument();
    expect(screen.getByText(/sandbox/i)).toBeInTheDocument();
    expect(screen.getByText(/80\.000\.000/)).toBeInTheDocument();
    expect(screen.getByText(/43\.000\.000/)).toBeInTheDocument();
  });

  it("empty state when nothing is sold yet", async () => {
    mockSales.mockResolvedValue(
      snap({
        grossRevenue: 0, totalTicketsSold: 0, overallFillRatePercentage: 0, paidOrdersCount: 0, activeHoldsCount: 0,
        tierSales: [], postSaleReport: { averageOrderValue: 0, peakSalesVelocityTps: null, paymentBreakdown: [] },
      }),
    );
    render(<SalesDashboard event={event} />);
    await flush();
    expect(screen.getByText(/chưa (bán|có)[^.]*(vé|đơn)/i)).toBeInTheDocument();
  });

  it("error keeps the last data and shows an error alert", async () => {
    render(<SalesDashboard event={event} />);
    await flush();
    mockSales.mockRejectedValue(new Error("Hệ thống đang bận"));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4100);
    });
    expect(screen.getByText("Lỗi cập nhật doanh số")).toBeInTheDocument();
    expect(screen.getByText("Hệ thống đang bận")).toBeInTheDocument();
    expect(screen.getByText("Standard Pass")).toBeInTheDocument();
  });
});
