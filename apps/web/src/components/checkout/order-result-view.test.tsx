import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as paymentClient from "@/lib/payment-client";
import type { IssuedTicketQR, OrderDetails } from "@/lib/payment-client";
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

  it("WP2 (AC-15, BR-O8): PAID -> gọi issueTickets, mỗi vé một khung QR từ qr_payload thật, không có QR demo", async () => {
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
      items: [{ ticketTypeId: "tier-vip", tierName: "Vé VIP", quantity: 1, unitCents: 500000 }],
    };
    const issue = vi.spyOn(paymentClient, "issueTickets").mockResolvedValue([
      { ticketId: "abcdef12-0000", ticketTypeId: "tier-vip", status: "ISSUED", qrPayload: "QR.SIGNED.PAYLOAD-001" },
    ]);

    const { container } = render(
      <OrderResultView
        eventId="evt-1"
        orderId="ord-paid-1"
        initialOrder={paidOrder}
      />,
    );

    expect(screen.getByText(/Đã thanh toán thành công/i)).toBeDefined();
    // F-13: tieu de "da san sang" chi xuat hien khi da co ve QR (truoc do la "dang phat hanh ve").
    expect(await screen.findByRole("img", { name: "Mã QR vé ABCDEF12" })).toBeDefined();
    expect(screen.getByRole("heading", { level: 2, name: /Vé điện tử của bạn đã sẵn sàng/i })).toBeDefined();
    expect(issue).toHaveBeenCalledWith("ord-paid-1");
    expect(container.querySelector('[data-qr-payload="QR.SIGNED.PAYLOAD-001"]')).not.toBeNull();
    expect(screen.queryByText(/DEMO/)).toBeNull();
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

  it("WP2: thử thanh toán lại mang đủ orderId, holdId, hạng vé và số lượng khi đơn có dữ liệu", () => {
    const failedOrder: OrderDetails = {
      orderId: "ord-failed-2",
      eventId: "evt-1",
      eventTitle: "Concert Đêm Nhạc",
      venue: "Nhà Hát Lớn",
      tierName: "Vé VIP",
      quantity: 2,
      totalAmount: 1000000,
      status: "FAILED",
      holdId: "hld-9",
      items: [{ ticketTypeId: "tier-vip", tierName: "Vé VIP", quantity: 2, unitCents: 500000 }],
    };
    render(<OrderResultView eventId="evt-1" orderId="ord-failed-2" initialOrder={failedOrder} />);

    const href = screen.getByRole("link", { name: /Thử thanh toán lại/i }).getAttribute("href") ?? "";
    const url = new URL(href, "http://x");
    expect(url.pathname).toBe("/checkout/evt-1/payment");
    expect(Object.fromEntries(url.searchParams)).toEqual({
      orderId: "ord-failed-2",
      holdId: "hld-9",
      tierId: "tier-vip",
      quantity: "2",
    });
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

// ---------------------------------------------------------------------------
// EV-188 AC-3 (axe V4 `aria-prohibited-attr`): khoi QR co `aria-label` phai mang role hop le
// (role="img"), neu khong axe bao serious va screen reader bo qua ten.
// WP2: ve (va QR) den tu issueTickets, khong con tu issuedTickets demo cua don hang.
// ---------------------------------------------------------------------------
describe("OrderResultView — khoi ma QR truy cap duoc (EV-188 V4)", () => {
  const paidWithTwoTickets: OrderDetails = {
    orderId: "ord-a11y-1",
    eventId: "evt-1",
    eventTitle: "Concert Đêm Nhạc",
    venue: "Nhà Hát Lớn",
    tierName: "Vé VIP",
    quantity: 2,
    totalAmount: 1000000,
    status: "PAID",
    paidAt: "2026-10-10T21:00:00Z",
  };
  const TICKETS: IssuedTicketQR[] = ["a11y0001-x", "a11y0002-x"].map((id, i) => ({
    ticketId: id,
    ticketTypeId: "tier-vip",
    status: "ISSUED",
    qrPayload: `SIGNED-${i + 1}`,
  }));

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(paymentClient, "issueTickets").mockResolvedValue(TICKETS);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("moi ve co mot khoi QR role img, ten 'Mã QR vé <mã vé>'", async () => {
    render(<OrderResultView eventId="evt-1" orderId="ord-a11y-1" initialOrder={paidWithTwoTickets} />);

    expect(await screen.findAllByRole("img", { name: /Mã QR vé/ })).toHaveLength(2);
    expect(screen.getByRole("img", { name: "Mã QR vé A11Y0001" })).toBeInTheDocument();
    expect(screen.getByRole("img", { name: "Mã QR vé A11Y0002" })).toBeInTheDocument();
  });

  it("khong con aria-label tren div/span khong co role (aria-prohibited-attr)", async () => {
    const { container } = render(
      <OrderResultView eventId="evt-1" orderId="ord-a11y-1" initialOrder={paidWithTwoTickets} />,
    );
    await screen.findAllByRole("img", { name: /Mã QR vé/ });

    const offenders = Array.from(container.querySelectorAll("div[aria-label], span[aria-label]")).filter(
      (el) => !el.hasAttribute("role"),
    );
    expect(offenders.map((el) => el.outerHTML.slice(0, 80))).toEqual([]);
  });
});

// ---------------------------------------------------------------------------
// WP2 (AC-14, AC-15): cho phat hanh ve, loi, khong bia du lieu.
// ---------------------------------------------------------------------------
describe("OrderResultView — phat hanh ve va loi (AC-14, AC-15)", () => {
  const paid: OrderDetails = {
    orderId: "ord-wp2-1",
    eventId: "evt-1",
    eventTitle: "Concert Đêm Nhạc",
    venue: "Nhà Hát Lớn",
    tierName: "Vé VIP",
    quantity: 1,
    totalAmount: 500000,
    status: "PAID",
  };

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("chua phat hanh (rong): hien trang thai cho, KHONG ve QR nao, thanh cong khi thu lai", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const issue = vi
      .spyOn(paymentClient, "issueTickets")
      .mockResolvedValueOnce([])
      .mockResolvedValue([{ ticketId: "late0001-x", ticketTypeId: "t", status: "ISSUED", qrPayload: "LATE-QR" }]);
    vi.spyOn(paymentClient, "getOrderDetails").mockResolvedValue(paid);

    render(<OrderResultView eventId="evt-1" orderId="ord-wp2-1" initialOrder={paid} />);

    expect(await screen.findByText("Đang phát hành vé")).toBeInTheDocument();
    expect(screen.queryAllByRole("img", { name: /Mã QR vé/ })).toHaveLength(0);

    await vi.advanceTimersByTimeAsync(2500);
    expect(await screen.findByRole("img", { name: "Mã QR vé LATE0001" })).toBeInTheDocument();
    expect(issue).toHaveBeenCalledTimes(2);
  });

  it("het luot thu lai van chua co ve: bao loi + nut lay ve lai, van khong co QR, so lan thu co gioi han", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const issue = vi
      .spyOn(paymentClient, "issueTickets")
      .mockRejectedValue(new paymentClient.PaymentClientError("422", "UNKNOWN", 422));
    vi.spyOn(paymentClient, "getOrderDetails").mockResolvedValue(paid);

    render(<OrderResultView eventId="evt-1" orderId="ord-wp2-1" initialOrder={paid} />);

    await vi.advanceTimersByTimeAsync(120_000);
    expect(await screen.findByText("Chưa lấy được vé điện tử")).toBeInTheDocument();
    expect(screen.queryAllByRole("img", { name: /Mã QR vé/ })).toHaveLength(0);
    const calls = issue.mock.calls.length;
    expect(calls).toBeGreaterThan(1);
    expect(calls).toBeLessThanOrEqual(6);

    issue.mockResolvedValue([{ ticketId: "retry001-x", ticketTypeId: "t", status: "ISSUED", qrPayload: "RETRY-QR" }]);
    fireEvent.click(screen.getByRole("button", { name: "Lấy vé lại" }));
    expect(await screen.findByRole("img", { name: "Mã QR vé RETRY001" })).toBeInTheDocument();
  });

  it("don khong ton tai (404): bao khong tim thay, khong tao du lieu mac dinh", async () => {
    vi.spyOn(paymentClient, "getOrderDetails").mockRejectedValue(
      new paymentClient.PaymentClientError("nf", "NOT_FOUND", 404),
    );
    render(<OrderResultView eventId="evt-1" orderId="ord-missing" />);

    expect(await screen.findByText("Không tìm thấy đơn hàng")).toBeInTheDocument();
    expect(screen.queryByText(/Sự kiện EvenFlow/)).toBeNull();
    expect(screen.queryByText(/Vé Tiêu Chuẩn/)).toBeNull();
    expect(screen.queryByRole("button", { name: "Thử lại" })).toBeNull();
  });

  it("loi mang khi tai don: bao loi va nut thu lai goi lai API", async () => {
    const get = vi
      .spyOn(paymentClient, "getOrderDetails")
      .mockRejectedValueOnce(new paymentClient.PaymentClientError("net", "NETWORK_ERROR"))
      .mockResolvedValue({ ...paid, status: "CANCELLED" });
    render(<OrderResultView eventId="evt-1" orderId="ord-wp2-1" />);

    expect(await screen.findByText("Không tải được đơn hàng")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
    await waitFor(() => expect(screen.getByText(/Đơn hàng đã được huỷ/)).toBeInTheDocument());
    expect(get).toHaveBeenCalledTimes(2);
  });
});
