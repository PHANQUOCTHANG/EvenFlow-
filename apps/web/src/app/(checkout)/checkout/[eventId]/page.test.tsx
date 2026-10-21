import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import CheckoutPage, { metadata } from "./page";

// WP2 (AC-18): trang la vo server mong, su kien duoc tai phia client trong CheckoutPageClient.
vi.mock("@/components/checkout/checkout-page-client", () => ({
  CheckoutPageClient: ({ eventId }: { eventId: string }) => <div data-testid="checkout-client">Event: {eventId}</div>,
}));

vi.mock("@/lib/event-service", () => ({
  getEventById: vi.fn(() => {
    throw new Error("server shell khong duoc goi backend");
  }),
}));

describe("CheckoutPage (EV-183, EVF-113, AC-18)", () => {
  it("render client component voi eventId tu URL, khong goi backend phia server", async () => {
    const Component = await CheckoutPage({ params: Promise.resolve({ eventId: "evt-1" }) });
    render(Component);

    expect(screen.getByTestId("checkout-client")).toBeInTheDocument();
    expect(screen.getByText("Event: evt-1")).toBeInTheDocument();
  });

  it("metadata tinh co noindex", () => {
    expect(metadata.title).toBe("Chọn vé | EvenFlow");
    expect(metadata.robots).toEqual({ index: false, follow: false });
  });
});
