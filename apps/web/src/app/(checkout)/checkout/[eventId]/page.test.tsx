import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import CheckoutPage, { generateMetadata } from "./page";

vi.mock("@/components/checkout/checkout-view", () => ({
  CheckoutView: ({ event }: { event: { id: string; title: string } }) => (
    <div data-testid="checkout-view">
      <span>Event: {event.id}</span>
      <span>Title: {event.title}</span>
    </div>
  ),
}));

vi.mock("@/lib/event-service", () => ({
  getEventById: vi.fn(async (id: string) => {
    if (id === "evt-1") {
      return {
        id: "evt-1",
        title: "EvenFlow Grand Concert",
        venue: "Sân vận động Mỹ Đình",
        tiers: [],
      };
    }
    return null;
  }),
}));

vi.mock("next/navigation", () => ({
  notFound: vi.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
}));

describe("CheckoutPage (EV-183, EVF-113)", () => {
  it("render CheckoutView component khi tìm thấy sự kiện", async () => {
    const params = Promise.resolve({ eventId: "evt-1" });
    const Component = await CheckoutPage({ params });
    render(Component);

    expect(screen.getByTestId("checkout-view")).toBeInTheDocument();
    expect(screen.getByText("Event: evt-1")).toBeInTheDocument();
    expect(screen.getByText("Title: EvenFlow Grand Concert")).toBeInTheDocument();
  });

  it("gọi notFound khi không tìm thấy sự kiện", async () => {
    const params = Promise.resolve({ eventId: "evt-not-found" });
    await expect(CheckoutPage({ params })).rejects.toThrow("NEXT_NOT_FOUND");
  });

  it("generateMetadata tạo title chính xác và noindex", async () => {
    const params = Promise.resolve({ eventId: "evt-1" });
    const meta = await generateMetadata({ params });

    expect(meta.title).toBe("Chọn vé: EvenFlow Grand Concert | EvenFlow");
    expect(meta.robots).toEqual({ index: false, follow: false });
  });

  it("generateMetadata fallback khi không tìm thấy event", async () => {
    const params = Promise.resolve({ eventId: "unknown-id" });
    const meta = await generateMetadata({ params });

    expect(meta.title).toBe("Chọn vé | EvenFlow");
  });
});
