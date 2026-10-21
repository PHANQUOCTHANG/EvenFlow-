/** AC-6 UI: 403 /problems/forbidden while initiating payment shows the existing checkout.forbidden message. */
import { fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

import { PaymentHandoffView } from "./payment-handoff-view";

const FORBIDDEN_TEXT = "Tài khoản của bạn không có quyền mua vé cho sự kiện này.";

beforeEach(() => {
  setupEnv();
  seedSession("acc", "r1");
});
afterEach(resetEnv);

describe("PaymentHandoffView 403 forbidden (AC-6)", () => {
  it("shows checkout.forbidden (not the generic HTTP 403 text) and sends exactly one payment POST", async () => {
    const { calls } = installFetch((c) => {
      if (c.path === "/v1/payments/methods" || c.path.endsWith("/payments/methods")) {
        return json({ methods: [{ code: "sandbox", name: "Sandbox Mock", description: "", enabled: true, icon: "x" }] });
      }
      return problem(403, "/problems/forbidden", "role mismatch");
    });
    render(
      <PaymentHandoffView
        eventId="e1"
        eventTitle="Rock"
        venue="HN"
        orderId="o1"
        holdId="h1"
        tierName="VIP"
        quantity={1}
        unitPrice={100000}
      />,
    );
    await screen.findByLabelText(/Sandbox Mock/);
    fireEvent.change(screen.getByLabelText(/Họ và tên người nhận/i), { target: { value: "Nguyễn Văn A" } });
    fireEvent.change(screen.getByLabelText(/Email nhận vé điện tử/i), { target: { value: "a@example.com" } });
    fireEvent.change(screen.getByLabelText(/Số điện thoại liên hệ/i), { target: { value: "0901234567" } });
    fireEvent.click(screen.getByRole("button", { name: /Thanh toán 100\.000/i }));
    expect(await screen.findByText(FORBIDDEN_TEXT)).toBeDefined();
    expect(screen.queryByText(/Lỗi thanh toán HTTP 403/)).toBeNull();
    expect(calls.filter((c) => c.method === "POST" && c.path.endsWith("/payments"))).toHaveLength(1);
  });
});
