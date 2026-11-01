/**
 * ops-warroom (test change request): dashboard chạy trên số liệu thật (isSample = false), không còn nhãn / alert mẫu,
 * peak TPS hiện "Chưa có dữ liệu", cơ cấu thanh toán là danh sách theo nhà cung cấp, tự làm mới bật mặc định.
 * Hành vi polling / lỗi / rỗng chi tiết nằm ở sales-dashboard.real.test.tsx. getRealtimeSales được giữ treo để
 * test không gọi mạng.
 */
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { OrganizerEvent, RealtimeSalesSnapshot } from "@/lib/organizer-service";

import { SalesDashboard } from "./sales-dashboard";

vi.mock("@/lib/organizer-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/organizer-service")>();
  return { ...actual, getRealtimeSales: vi.fn(() => new Promise(() => {})) };
});

describe("SalesDashboard component (EV-186, EVF-116, AC-4, AC-5, Stitch 10.14)", () => {
  const mockEvent: OrganizerEvent = {
    id: "evt-tech-summit",
    slug: "tech-summit-2026",
    title: "Vietnam Tech Summit 2026",
    description: "Hội thảo công nghệ",
    venue: "Hà Nội",
    startsAt: "2026-12-05T08:30:00Z",
    saleStartAt: "2026-09-01T09:00:00Z",
    state: "ON_SALE",
    organizerId: "org-1",
    tiers: [
      { id: "t1", name: "Standard Pass", price: 900000, quota: 1000 },
      { id: "t2", name: "Executive Pass", price: 2500000, quota: 200 },
    ],
    createdAt: "2026-08-01T00:00:00Z",
    updatedAt: "2026-09-01T00:00:00Z",
  };

  const mockSnapshot: RealtimeSalesSnapshot = {
    isSample: false,
    eventId: "evt-tech-summit",
    timestamp: "2026-11-01T05:30:00Z",
    grossRevenue: 950000000,
    totalTicketsSold: 750,
    totalQuota: 1200,
    overallFillRatePercentage: 63,
    paidOrdersCount: 420,
    activeHoldsCount: 35,
    tierSales: [
      {
        tierId: "t1",
        name: "Standard Pass",
        price: 900000,
        quota: 1000,
        sold: 650,
        activeHolds: 30,
        revenue: 585000000,
        fillRatePercentage: 65,
      },
      {
        tierId: "t2",
        name: "Executive Pass",
        price: 2500000,
        quota: 200,
        sold: 100,
        activeHolds: 5,
        revenue: 250000000,
        fillRatePercentage: 50,
      },
    ],
    postSaleReport: {
      averageOrderValue: 2261904,
      peakSalesVelocityTps: null,
      paymentBreakdown: [
        { provider: "vnpay", orders: 231, amount: 522500000 },
        { provider: "momo", orders: 126, amount: 285000000 },
        { provider: "card", orders: 63, amount: 142500000 },
      ],
    },
  };

  it("số liệu thật: không có nhãn / alert 'Dữ liệu mẫu' trước các khối số liệu (ops-warroom AC-30)", () => {
    render(<SalesDashboard event={mockEvent} initialSalesData={mockSnapshot} />);

    expect(screen.queryByText(/Dữ liệu mẫu/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Mẫu — chưa phải doanh thu thật/)).not.toBeInTheDocument();
    expect(screen.getByText("Tổng doanh thu")).toBeInTheDocument();
    expect(screen.queryByText("98.6%")).not.toBeInTheDocument();
  });

  it("render tiêu đề, huy hiệu Realtime (≤ 5s) và các thẻ chỉ số (AC-4)", () => {
    render(<SalesDashboard event={mockEvent} initialSalesData={mockSnapshot} />);

    expect(screen.getByText("Vietnam Tech Summit 2026")).toBeInTheDocument();
    expect(screen.getByText(/Realtime \(≤ 5s\)/)).toBeInTheDocument();

    // 4 Thẻ chỉ số
    expect(screen.getByText("Tổng doanh thu")).toBeInTheDocument();
    expect(screen.getByText("Tổng vé đã bán")).toBeInTheDocument();
    expect(screen.getByText("Đơn hàng thành công")).toBeInTheDocument();
    expect(screen.getByText("Vé đang giữ (Hold)")).toBeInTheDocument();

    // Vé đang giữ 35
    expect(screen.getByText("35")).toBeInTheDocument();
  });

  it("render bảng chi tiết theo từng hạng vé và thanh tiến độ lấp đầy", () => {
    render(<SalesDashboard event={mockEvent} initialSalesData={mockSnapshot} />);

    expect(screen.getByText("Chi tiết doanh số theo từng hạng vé")).toBeInTheDocument();
    expect(screen.getByText("Standard Pass")).toBeInTheDocument();
    expect(screen.getByText("Executive Pass")).toBeInTheDocument();

    // Kiểm tra progress bar
    expect(screen.getByLabelText("Tiến độ bán vé Standard Pass")).toBeInTheDocument();
    expect(screen.getByLabelText("Tiến độ bán vé Executive Pass")).toBeInTheDocument();
  });

  it("chuyển sang tab Báo cáo tổng kết sau bán (AC-5)", () => {
    render(<SalesDashboard event={mockEvent} initialSalesData={mockSnapshot} />);

    const reportTab = screen.getByRole("tab", { name: /Báo cáo tổng kết sau bán/ });
    fireEvent.click(reportTab);

    expect(screen.getByText("Giá trị trung bình mỗi đơn (AOV)")).toBeInTheDocument();
    const tps = screen.getByText("Tốc độ giao dịch cao điểm").parentElement as HTMLElement;
    expect(tps).toHaveTextContent("Chưa có dữ liệu");
    expect(tps).not.toHaveTextContent(/TPS/);
    expect(screen.getByText("Cơ cấu doanh thu theo phương thức thanh toán")).toBeInTheDocument();
    expect(screen.getByText("vnpay")).toBeInTheDocument();
    expect(screen.getByText("momo")).toBeInTheDocument();
    expect(screen.getByText("card")).toBeInTheDocument();
  });

  it("cho phép bật/tắt tự động cập nhật (mặc định bật, EVF-116 AC-2)", () => {
    render(<SalesDashboard event={mockEvent} initialSalesData={mockSnapshot} />);

    const toggleBtn = screen.getByRole("button", { name: /Tắt tự động cập nhật/ });
    fireEvent.click(toggleBtn);
    expect(screen.getByRole("button", { name: /Bật tự động/ })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Bật tự động/ }));
    expect(screen.getByRole("button", { name: /Tắt tự động cập nhật/ })).toBeInTheDocument();
  });
});
