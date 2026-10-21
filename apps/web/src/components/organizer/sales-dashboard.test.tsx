import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { OrganizerEvent, RealtimeSalesSnapshot } from "@/lib/organizer-service";

import { SalesDashboard } from "./sales-dashboard";

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
    isSample: true,
    eventId: "evt-tech-summit",
    timestamp: "12:30:00",
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
      peakSalesVelocityTps: 78.4,
      paymentBreakdown: {
        vnpay: 522500000,
        momo: 285000000,
        card: 142500000,
      },
    },
  };

  it("gắn nhãn Dữ liệu mẫu rõ ràng trước các khối số liệu (AC-24)", () => {
    render(<SalesDashboard event={mockEvent} initialSalesData={mockSnapshot} />);

    const label = screen.getByText("Dữ liệu mẫu — chưa khả dụng từ hệ thống");
    expect(label).toBeInTheDocument();
    expect(
      label.compareDocumentPosition(screen.getByText("Tổng doanh thu")) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
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
    expect(screen.getByText(/78.4 TPS/)).toBeInTheDocument();
    expect(screen.getByText("Cơ cấu doanh thu theo phương thức thanh toán")).toBeInTheDocument();
    expect(screen.getByText("Cổng thanh toán VNPay")).toBeInTheDocument();
    expect(screen.getByText("Ví điện tử MoMo")).toBeInTheDocument();
    expect(screen.getByText("Thẻ Quốc Tế (Visa/Master)")).toBeInTheDocument();
  });

  it("cho phép bật/tắt tự động cập nhật", () => {
    render(<SalesDashboard event={mockEvent} initialSalesData={mockSnapshot} />);

    // Dữ liệu mẫu: mặc định không tự động gọi API, người dùng có thể bật.
    const toggleBtn = screen.getByRole("button", { name: /Bật tự động/ });
    fireEvent.click(toggleBtn);

    expect(screen.getByRole("button", { name: /Tắt tự động cập nhật/ })).toBeInTheDocument();
  });
});
