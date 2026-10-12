import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { OrderDetails } from "@/lib/payment-client";
import { OrderResultView } from "./order-result-view";

describe("OrderResultView component (EVF-114, BR-O2, BR-O6, BR-O8)", () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("render trạng thái PENDING khi đang chờ xác nhận từ cổng (AC-4, BR-O6)", () => {
    const pendingOrder: OrderDetails = {
      orderId: "ord-p-1",
      eventId: "evt-1",
      eventTitle: "Concert Đêm Nhạc",
      venue: "Nhà Hát Lớn",
      tierName: "Vé VIP",
      quantity: 2,
      totalAmount: 1000000,
      status: "PENDING",
    };

    render(
      <OrderResultView
        eventId="evt-1"
        orderId="ord-p-1"
        initialOrder={pendingOrder}
      />,
    );

    expect(screen.getByRole("heading", { level: 2, name: /Đang xác nhận thanh toán/i })).toBeDefined();
    expect(screen.getByText(/Hệ thống đang chờ tín hiệu xác nhận/i)).toBeDefined();
    expect(screen.getByRole("button", { name: /Kiểm tra trạng thái ngay/i })).toBeDefined();
  });

  it("render trạng thái PAID với vé điện tử và mã QR demo (AC-5, BR-O8)", () => {
    const paidOrder: OrderDetails = {
      orderId: "ord-paid-1",
      eventId: "evt-1",
      eventTitle: "Concert Đêm Nhạc",
      venue: "Nhà Hát Lớn",
      tierName: "Vé VIP",
      quantity: 1,
      totalAmount: 500000,
      status: "PAID",
      paidAt: "2026-10-10T21:00:00Z",
      issuedTickets: [
        {
          ticketId: "tkt-1",
          ticketCode: "EF-VIP-001",
          tierName: "Vé VIP",
          attendeeName: "Nguyễn Văn A",
          qrPayload: "DEMO-PAYLOAD-001",
        },
      ],
    };

    render(
      <OrderResultView
        eventId="evt-1"
        orderId="ord-paid-1"
        initialOrder={paidOrder}
      />,
    );

    expect(screen.getByText(/Đã thanh toán thành công/i)).toBeDefined();
    expect(screen.getByRole("heading", { level: 2, name: /Vé điện tử của bạn đã sẵn sàng/i })).toBeDefined();
    expect(screen.getAllByText("EF-VIP-001").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText(/DEMO/i)).toBeDefined();
    expect(screen.getByRole("button", { name: /In \/ Tải vé/i })).toBeDefined();

    // In vé
    const printSpy = vi.spyOn(window, "print").mockImplementation(() => {});
    fireEvent.click(screen.getByRole("button", { name: /In \/ Tải vé/i }));
    expect(printSpy).toHaveBeenCalledTimes(1);
  });

  it("render trạng thái FAILED với thông báo lỗi và nút thử lại (AC-6)", () => {
    const failedOrder: OrderDetails = {
      orderId: "ord-failed-1",
      eventId: "evt-1",
      eventTitle: "Concert Đêm Nhạc",
      venue: "Nhà Hát Lớn",
      tierName: "Vé VIP",
      quantity: 1,
      totalAmount: 500000,
      status: "FAILED",
      failureReason: "Thẻ ngân hàng không đủ số dư",
    };

    render(
      <OrderResultView
        eventId="evt-1"
        orderId="ord-failed-1"
        initialOrder={failedOrder}
      />,
    );

    expect(screen.getByText(/Thanh toán không thành công/i)).toBeDefined();
    expect(screen.getByText(/Thẻ ngân hàng không đủ số dư/i)).toBeDefined();
    expect(screen.getByRole("link", { name: /Thử thanh toán lại/i })).toBeDefined();
    expect(screen.getByRole("link", { name: /Chọn lại vé/i })).toBeDefined();
  });

  it("render trạng thái EXPIRED khi phiên giữ vé hết hạn (AC-3, BR-O2)", () => {
    const expiredOrder: OrderDetails = {
      orderId: "ord-exp-1",
      eventId: "evt-1",
      eventTitle: "Concert Đêm Nhạc",
      venue: "Nhà Hát Lớn",
      tierName: "Vé VIP",
      quantity: 1,
      totalAmount: 500000,
      status: "EXPIRED",
    };

    render(
      <OrderResultView
        eventId="evt-1"
        orderId="ord-exp-1"
        initialOrder={expiredOrder}
      />,
    );

    expect(screen.getByText(/Phiên giữ vé đã hết hạn \(BR-O2\)/i)).toBeDefined();
    expect(screen.getByText(/Toàn bộ số vé đã được tự động hoàn trả lại kho/i)).toBeDefined();
    expect(screen.getByRole("link", { name: /Quay lại chọn vé/i })).toBeDefined();
  });

  it("render trạng thái CANCELLED khi đơn hàng đã huỷ", () => {
    const cancelledOrder: OrderDetails = {
      orderId: "ord-can-1",
      eventId: "evt-1",
      eventTitle: "Concert Đêm Nhạc",
      venue: "Nhà Hát Lớn",
      tierName: "Vé VIP",
      quantity: 1,
      totalAmount: 500000,
      status: "CANCELLED",
    };

    render(
      <OrderResultView
        eventId="evt-1"
        orderId="ord-can-1"
        initialOrder={cancelledOrder}
      />,
    );

    expect(screen.getByText(/Đơn hàng đã được huỷ/i)).toBeDefined();
    expect(screen.getByRole("link", { name: /Về trang chủ/i })).toBeDefined();
  });
});
