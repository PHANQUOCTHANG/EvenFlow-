/** Regression F-5 (payment page): AUTH_REQUIRED from initiatePayment shows a sign-in prompt. */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PaymentHandoffView } from "@/components/checkout/payment-handoff-view";
import * as paymentClient from "@/lib/payment-client";

afterEach(() => vi.restoreAllMocks());

describe("F-5 payment page", () => {
  it("shows sign-in link on AUTH_REQUIRED", async () => {
    vi.spyOn(paymentClient, "listPaymentMethods").mockResolvedValue([
      { code: "sandbox", name: "Sandbox Mock", description: "", enabled: true, icon: "flask" },
    ]);
    vi.spyOn(paymentClient, "initiatePayment").mockRejectedValue(
      new paymentClient.PaymentClientError("Phiên đăng nhập đã hết hạn", "AUTH_REQUIRED", 401),
    );
    render(
      <PaymentHandoffView
        eventId="e1" eventTitle="T" venue="V" orderId="o1" holdId="h1" tierName="VIP" quantity={1} unitPrice={1000}
        expiresAt={new Date(Date.now() + 600_000).toISOString()}
      />,
    );
    await screen.findByLabelText(/Sandbox Mock/);
    fireEvent.change(screen.getByLabelText(/Họ và tên người nhận/i), { target: { value: "A" } });
    fireEvent.change(screen.getByLabelText(/Email nhận vé điện tử/i), { target: { value: "a@example.com" } });
    fireEvent.change(screen.getByLabelText(/Số điện thoại liên hệ/i), { target: { value: "0901234567" } });
    fireEvent.click(screen.getByRole("button", { name: /Thanh toán/i }));
    const link = await waitFor(() => screen.getByRole("link", { name: /Đăng nhập/ }));
    expect(link).toHaveAttribute("href", "/auth/sign-in");
  });
});
