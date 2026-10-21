/** Regression F-20 (X-18): CheckoutView honours event.maxPerOrder (fallback 4). */
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { CheckoutView } from "@/components/checkout/checkout-view";
import type { EventDetailSnapshot } from "@/lib/event-service";

vi.mock("@/hooks/use-server-time-sync", () => ({
  useServerTimeSync: () => ({ offsetMs: 0, synced: true, isLoading: false, error: null, getServerNow: () => Date.now() }),
}));

const make = (maxPerOrder: number) =>
  ({
    id: "e1", slug: "s", title: "T", description: "", venue: "V",
    startsAt: "2026-12-01T00:00:00Z", saleStartAt: "2026-11-01T00:00:00Z", state: "ON_SALE", maxPerOrder,
    tiers: [{ id: "t1", name: "Std", price: 1000, currency: "VND", availability: "AVAILABLE" }],
  }) as unknown as EventDetailSnapshot;

describe("F-20", () => {
  it("uses event.maxPerOrder = 2", () => {
    render(<CheckoutView event={make(2)} initialQueueToken="q" />);
    expect(screen.getByText("Tối đa 2 vé")).toBeInTheDocument();
  });
  it("uses event.maxPerOrder = 8", () => {
    render(<CheckoutView event={make(8)} initialQueueToken="q" />);
    expect(screen.getByText("Tối đa 8 vé")).toBeInTheDocument();
  });
  it("falls back to 4 when absent (0)", () => {
    render(<CheckoutView event={make(0)} initialQueueToken="q" />);
    expect(screen.getByText("Tối đa 4 vé")).toBeInTheDocument();
  });
});
