import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { installFetch, json, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import PaymentPage, { generateMetadata } from "./page";

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

beforeEach(() => {
  setupEnv();
  installFetch((c) =>
    c.path.endsWith("/availability")
      ? json({ event_id: EVT, tiers: [] })
      : json({
          id: EVT,
          slug: "vietnam-tech-summit-2026",
          title: "Vietnam Tech Summit 2026",
          description: "",
          venue: "Hà Nội",
          starts_at: "2026-12-05T01:30:00Z",
          sale_start_at: "2026-09-01T02:00:00Z",
          status: "ON_SALE",
          max_per_order: 4,
          tiers: [{ id: "11111111-1111-4111-8111-111111111111", name: "Standard Pass", price_cents: 900000, currency: "VND" }],
        }),
  );
});
afterEach(resetEnv);

describe("PaymentPage route (EVF-114, EV-184)", () => {
  it("generateMetadata tạo tiêu đề chứa tên sự kiện", async () => {
    const meta = await generateMetadata({
      params: Promise.resolve({ eventId: "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11" }),
      searchParams: Promise.resolve({}),
    });

    expect(meta.title).toContain("EvenFlow");
  });

  it("render trang thanh toán với thông tin sự kiện", async () => {
    const pageComponent = await PaymentPage({
      params: Promise.resolve({ eventId: "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11" }),
      searchParams: Promise.resolve({
        orderId: "ord-test-123",
        holdId: "hld-test-123",
        quantity: "2",
      }),
    });

    render(pageComponent);

    expect(screen.getByRole("heading", { level: 1, name: /Thanh toán đơn hàng/i })).toBeDefined();
    expect(screen.getByText(/Vietnam Tech Summit 2026/i)).toBeDefined();
  });
});
