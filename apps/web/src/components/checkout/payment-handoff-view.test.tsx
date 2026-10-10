import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as paymentClient from "@/lib/payment-client";
import { PaymentHandoffView } from "./payment-handoff-view";

describe("PaymentHandoffView component (EVF-114, BR-O2, BR-O5)", () => {
  const baseProps = {
    eventId: "evt-rock-2026",
    eventTitle: "Rock Fest 2026",
    venue: "Sân vận động Mỹ Đình",
    orderId: "ord-test-1",
    holdId: "hld-test-1",
    tierName: "Vé VIP",
    quantity: 2,
    unitPrice: 500000,
    expiresAt: new Date(Date.now() + 10 * 60 * 1000).toISOString(),
  };

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("render đầy đủ thông tin đơn hàng, tổng tiền và các cổng thanh toán", () => {
    render(<PaymentHandoffView {...baseProps} />);

    expect(screen.getByRole("heading", { level: 1, name: /Thanh toán đơn hàng/i })).toBeDefined();
    expect(screen.getByText(/Rock Fest 2026/i)).toBeDefined();
    expect(screen.getByText("Vé VIP")).toBeDefined();
    expect(screen.getByText("2 vé")).toBeDefined();
    expect(screen.getAllByText(/1.000.000/i).length).toBeGreaterThanOrEqual(1);

    // 3 phương thức thanh toán
    expect(screen.getByLabelText(/Sandbox Mock/i)).toBeDefined();
    expect(screen.getByLabelText(/Cổng thanh toán VNPay/i)).toBeDefined();
    expect(screen.getByLabelText(/Ví điện tử MoMo/i)).toBeDefined();

    // Đồng hồ giữ vé
    expect(screen.getAllByText(/Thời gian giữ vé còn/i).length).toBeGreaterThanOrEqual(1);
  });

  it("khoá form và hiển thị thông báo hết hạn khi hold đã quá hạn (BR-O2)", () => {
    const expiredProps = {
      ...baseProps,
      expiresAt: new Date(Date.now() - 60 * 1000).toISOString(), // 1 phút trước
    };

    render(<PaymentHandoffView {...expiredProps} />);

    expect(screen.getByText(/Đã hết thời gian giữ vé \(BR-O2\)/i)).toBeDefined();
    expect(screen.getByRole("link", { name: /Quay lại chọn vé/i })).toBeDefined();

    const submitBtn = screen.getByRole("button", { name: /Hết hạn giữ vé/i });
    expect(submitBtn.hasAttribute("disabled")).toBe(true);
  });

  it("hiển thị lỗi validation khi submit form với thông tin trống", async () => {
    render(<PaymentHandoffView {...baseProps} />);

    const submitBtn = screen.getByRole("button", { name: /Thanh toán 1\.000\.000/i });
    fireEvent.click(submitBtn);

    expect(await screen.findByText(/Vui lòng nhập họ và tên/i)).toBeDefined();
  });

  it("gọi onPaymentSuccess khi nhập đầy đủ thông tin và submit thành công (BR-O5)", async () => {
    const onPaymentSuccess = vi.fn();
    vi.spyOn(paymentClient, "initiatePayment").mockResolvedValue({
      paymentId: "pay_ok",
      orderId: "ord-test-1",
      redirectUrl: "/checkout/evt-rock-2026/result?orderId=ord-test-1",
      status: "PENDING",
    });

    render(<PaymentHandoffView {...baseProps} onPaymentSuccess={onPaymentSuccess} />);

    fireEvent.change(screen.getByLabelText(/Họ và tên người nhận/i), {
      target: { value: "Nguyễn Văn A" },
    });
    fireEvent.change(screen.getByLabelText(/Email nhận vé điện tử/i), {
      target: { value: "nguyenvana@example.com" },
    });
    fireEvent.change(screen.getByLabelText(/Số điện thoại liên hệ/i), {
      target: { value: "0901234567" },
    });

    const submitBtn = screen.getByRole("button", { name: /Thanh toán 1\.000\.000/i });
    await vi.waitFor(async () => {
      fireEvent.click(submitBtn);
    });

    await vi.waitFor(() => {
      expect(onPaymentSuccess).toHaveBeenCalledTimes(1);
    });
  });
});
