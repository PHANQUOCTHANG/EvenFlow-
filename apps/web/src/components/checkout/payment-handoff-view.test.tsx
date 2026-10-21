import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as paymentClient from "@/lib/payment-client";
import { PaymentHandoffView } from "./payment-handoff-view";

const METHODS: paymentClient.PaymentMethodInfo[] = [
  { code: "sandbox", name: "Sandbox Mock", description: "Giả lập thử nghiệm", enabled: true, icon: "flask" },
  { code: "vnpay", name: "Cổng thanh toán VNPay", description: "VNPAY-QR", enabled: true, icon: "vnpay" },
  { code: "momo", name: "Ví điện tử MoMo", description: "Quét mã QR", enabled: true, icon: "momo" },
];

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
    // WP2 (AC-13): cong thanh toan den tu API, khong con danh sach cung trong component.
    vi.spyOn(paymentClient, "listPaymentMethods").mockResolvedValue(METHODS);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("render đầy đủ thông tin đơn hàng, tổng tiền và các cổng thanh toán", async () => {
    render(<PaymentHandoffView {...baseProps} />);

    expect(screen.getByRole("heading", { level: 1, name: /Thanh toán đơn hàng/i })).toBeDefined();
    expect(screen.getByText(/Rock Fest 2026/i)).toBeDefined();
    expect(screen.getByText("Vé VIP")).toBeDefined();
    expect(screen.getByText("2 vé")).toBeDefined();
    expect(screen.getAllByText(/1.000.000/i).length).toBeGreaterThanOrEqual(1);

    // 3 phương thức thanh toán
    expect(await screen.findByLabelText(/Sandbox Mock/i)).toBeDefined();
    expect(screen.getByLabelText(/Cổng thanh toán VNPay/i)).toBeDefined();
    expect(screen.getByLabelText(/Ví điện tử MoMo/i)).toBeDefined();

    // Đồng hồ giữ vé
    expect(screen.getAllByText(/Thời gian giữ vé còn/i).length).toBeGreaterThanOrEqual(1);
  });

  it("khoá form và hiển thị thông báo hết hạn khi hold đã quá hạn (BR-O2)", async () => {
    const expiredProps = {
      ...baseProps,
      expiresAt: new Date(Date.now() - 60 * 1000).toISOString(), // 1 phút trước
    };

    render(<PaymentHandoffView {...expiredProps} />);

    expect(screen.getByText(/Đã hết thời gian giữ vé \(BR-O2\)/i)).toBeDefined();
    expect(screen.getByRole("link", { name: /Quay lại chọn vé/i })).toBeDefined();

    const submitBtn = screen.getByRole("button", { name: /Hết hạn giữ vé/i });
    expect(submitBtn.hasAttribute("disabled")).toBe(true);
    await screen.findByLabelText(/Sandbox Mock/i);
  });

  it("hiển thị lỗi validation khi submit form với thông tin trống", async () => {
    render(<PaymentHandoffView {...baseProps} />);
    await screen.findByLabelText(/Sandbox Mock/i);

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
    await screen.findByLabelText(/Sandbox Mock/i);

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

  // --- WP2 (AC-13): danh sach cong thanh toan tu API ---

  it("chỉ liệt kê cổng đang bật, hiển thị cả stripe", async () => {
    vi.spyOn(paymentClient, "listPaymentMethods").mockResolvedValue([
      ...METHODS.slice(0, 2),
      { code: "momo", name: "Ví điện tử MoMo", description: "", enabled: false, icon: "momo" },
      { code: "stripe", name: "Stripe", description: "Thẻ quốc tế", enabled: true, icon: "card" },
    ]);
    render(<PaymentHandoffView {...baseProps} />);

    expect(await screen.findByLabelText(/Stripe/)).toBeDefined();
    expect(screen.getByLabelText(/Sandbox Mock/)).toBeDefined();
    expect(screen.queryByLabelText(/Ví điện tử MoMo/)).toBeNull();
  });

  it("lỗi tải cổng thanh toán: hiện lỗi + thử lại, không có danh sách cứng, không cho thanh toán", async () => {
    const spy = vi
      .spyOn(paymentClient, "listPaymentMethods")
      .mockRejectedValueOnce(new paymentClient.PaymentClientError("boom", "NETWORK_ERROR"))
      .mockResolvedValue(METHODS);
    render(<PaymentHandoffView {...baseProps} />);

    expect(await screen.findByText(/Không tải được cổng thanh toán/)).toBeDefined();
    expect(screen.queryByLabelText(/Sandbox Mock/)).toBeNull();
    const pay = screen.getByRole("button", { name: /Thanh toán 1\.000\.000/i });
    expect(pay.hasAttribute("disabled")).toBe(true);

    fireEvent.click(screen.getByRole("button", { name: /Tải lại/ }));
    expect(await screen.findByLabelText(/Sandbox Mock/)).toBeDefined();
    expect(spy).toHaveBeenCalledTimes(2);
  });

  it("không có cổng nào đang bật: báo trống và khoá thanh toán", async () => {
    vi.spyOn(paymentClient, "listPaymentMethods").mockResolvedValue([
      { code: "momo", name: "Ví điện tử MoMo", description: "", enabled: false, icon: "momo" },
    ]);
    render(<PaymentHandoffView {...baseProps} />);

    expect(await screen.findByText(/chưa có cổng thanh toán nào đang mở/)).toBeDefined();
    await waitFor(() =>
      expect(screen.getByRole("button", { name: /Thanh toán 1\.000\.000/i }).hasAttribute("disabled")).toBe(true),
    );
  });

  it("gửi cổng đầu tiên đang bật khi mặc định (sandbox) không có trong danh sách", async () => {
    vi.spyOn(paymentClient, "listPaymentMethods").mockResolvedValue([METHODS[1]]);
    const init = vi.spyOn(paymentClient, "initiatePayment").mockResolvedValue({
      paymentId: "pay_1",
      orderId: "ord-test-1",
      redirectUrl: "/r",
      status: "PENDING",
    });
    render(<PaymentHandoffView {...baseProps} onPaymentSuccess={() => undefined} />);
    await screen.findByLabelText(/VNPay/);

    fireEvent.change(screen.getByLabelText(/Họ và tên người nhận/i), { target: { value: "Nguyễn Văn A" } });
    fireEvent.change(screen.getByLabelText(/Email nhận vé điện tử/i), { target: { value: "a@example.com" } });
    fireEvent.change(screen.getByLabelText(/Số điện thoại liên hệ/i), { target: { value: "0901234567" } });
    fireEvent.click(screen.getByRole("button", { name: /Thanh toán 1\.000\.000/i }));

    await waitFor(() => expect(init).toHaveBeenCalledTimes(1));
    expect(init.mock.calls[0][0].method).toBe("vnpay");
  });
});
