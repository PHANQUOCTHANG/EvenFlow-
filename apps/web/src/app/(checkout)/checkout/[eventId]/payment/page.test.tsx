import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import PaymentPage, { metadata } from "./page";

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

// WP2 (AC-18): trang la vo server mong; query chuyen thang xuong client component.
vi.mock("@/components/checkout/payment-page-client", () => ({
  PaymentPageClient: (props: Record<string, string | undefined>) => (
    <div data-testid="payment-client">{JSON.stringify(props)}</div>
  ),
}));

describe("PaymentPage route (EVF-114, EV-184, AC-18)", () => {
  it("chuyen eventId va query (orderId, holdId, tierId, quantity) cho client component", async () => {
    const pageComponent = await PaymentPage({
      params: Promise.resolve({ eventId: EVT }),
      searchParams: Promise.resolve({
        orderId: "ord-test-123",
        holdId: "hld-test-123",
        tierId: "tier-1",
        quantity: "2",
      }),
    });
    render(pageComponent);

    expect(JSON.parse(screen.getByTestId("payment-client").textContent ?? "{}")).toEqual({
      eventId: EVT,
      orderId: "ord-test-123",
      holdId: "hld-test-123",
      tierId: "tier-1",
      quantity: "2",
    });
  });

  it("khong tu bia orderId/holdId khi URL thieu", async () => {
    const pageComponent = await PaymentPage({
      params: Promise.resolve({ eventId: EVT }),
      searchParams: Promise.resolve({}),
    });
    render(pageComponent);

    expect(JSON.parse(screen.getByTestId("payment-client").textContent ?? "{}")).toEqual({ eventId: EVT });
  });

  it("metadata tinh co noindex", () => {
    expect(metadata.title).toBe("Thanh toán đơn hàng | EvenFlow");
    expect(metadata.robots).toEqual({ index: false, follow: false });
  });
});
