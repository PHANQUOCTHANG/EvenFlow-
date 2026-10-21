import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import OrderResultPage, { metadata } from "./page";

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

// WP2 (AC-18): trang la vo server mong; su kien va don duoc tai phia client.
vi.mock("@/components/checkout/result-page-client", () => ({
  ResultPageClient: (props: Record<string, string | undefined>) => (
    <div data-testid="result-client">{JSON.stringify(props)}</div>
  ),
}));

describe("OrderResultPage route (EVF-114, EV-184, AC-18)", () => {
  it("chuyen eventId va orderId cho client component", async () => {
    const pageComponent = await OrderResultPage({
      params: Promise.resolve({ eventId: EVT }),
      searchParams: Promise.resolve({ orderId: "ord-test-123" }),
    });
    render(pageComponent);

    expect(JSON.parse(screen.getByTestId("result-client").textContent ?? "{}")).toEqual({
      eventId: EVT,
      orderId: "ord-test-123",
    });
  });

  it("khong tu bia ma don khi URL thieu orderId", async () => {
    const pageComponent = await OrderResultPage({
      params: Promise.resolve({ eventId: EVT }),
      searchParams: Promise.resolve({}),
    });
    render(pageComponent);

    expect(JSON.parse(screen.getByTestId("result-client").textContent ?? "{}")).toEqual({ eventId: EVT });
  });

  it("metadata tinh co noindex", () => {
    expect(metadata.title).toBe("Kết quả đơn hàng | EvenFlow");
    expect(metadata.robots).toEqual({ index: false, follow: false });
  });
});
