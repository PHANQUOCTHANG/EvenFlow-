import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { OrganizerEvent, RealtimeSalesSnapshot } from "@/lib/organizer-service";

import OrganizerSalesPage from "./page";

const mocks = vi.hoisted(() => ({ getById: vi.fn(), sales: vi.fn() }));

vi.mock("@/lib/organizer-service", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/organizer-service")>()),
  getOrganizerEventById: mocks.getById,
  getRealtimeSales: mocks.sales,
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({
    user: { id: "u-1", role: "organizer" },
    loading: false,
    isAuthenticated: true,
    hasRole: (roles: string[]) => roles.includes("organizer"),
  }),
}));

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

const event: OrganizerEvent = {
  id: EVT,
  slug: "rock-fest",
  title: "Rock Fest",
  description: "",
  venue: "Mỹ Đình",
  startsAt: "2027-12-01T12:00:00Z",
  saleStartAt: "2027-11-01T03:00:00Z",
  state: "ON_SALE",
  organizerId: "org-1",
  tiers: [{ id: "t", name: "GA", price: 100000, quota: 100 }],
  createdAt: "",
  updatedAt: "",
};

const snapshot: RealtimeSalesSnapshot = {
  isSample: true,
  eventId: EVT,
  timestamp: "12:00:00",
  grossRevenue: 7200000,
  totalTicketsSold: 72,
  totalQuota: 100,
  overallFillRatePercentage: 72,
  paidOrdersCount: 40,
  activeHoldsCount: 5,
  tierSales: [
    {
      tierId: "t",
      name: "GA",
      price: 100000,
      quota: 100,
      sold: 72,
      activeHolds: 5,
      revenue: 7200000,
      fillRatePercentage: 72,
    },
  ],
};

beforeEach(() => {
  mocks.getById.mockReset();
  mocks.sales.mockReset();
});

describe("OrganizerSalesPage route (/organizer/events/[eventId]/sales - AC-24)", () => {
  it("hiện dashboard doanh số được gắn nhãn dữ liệu mẫu", async () => {
    mocks.getById.mockResolvedValue(event);
    mocks.sales.mockResolvedValue(snapshot);
    render(await OrganizerSalesPage({ params: Promise.resolve({ eventId: EVT }) }));

    expect(await screen.findByText("Rock Fest")).toBeInTheDocument();
    expect(await screen.findByText("Tổng doanh thu")).toBeInTheDocument();
    expect(screen.getByText("Dữ liệu mẫu — chưa khả dụng từ hệ thống")).toBeInTheDocument();
    expect(screen.getByText(/Dữ liệu mẫu · mô phỏng Realtime/)).toBeInTheDocument();
    expect(mocks.sales).toHaveBeenCalledWith(EVT);
  });

  it("render thẻ không tìm thấy khi mã sự kiện không tồn tại", async () => {
    mocks.getById.mockResolvedValue(null);
    render(await OrganizerSalesPage({ params: Promise.resolve({ eventId: "non-existent-event" }) }));
    expect(await screen.findByText("Không tìm thấy sự kiện")).toBeInTheDocument();
    expect(mocks.sales).not.toHaveBeenCalled();
  });
});
