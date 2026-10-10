import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import PaymentPage, { generateMetadata } from "./page";

describe("PaymentPage route (EVF-114, EV-184)", () => {
  it("generateMetadata tạo tiêu đề chứa tên sự kiện", async () => {
    const meta = await generateMetadata({
      params: Promise.resolve({ eventId: "evt-tech-summit-2026" }),
      searchParams: Promise.resolve({}),
    });

    expect(meta.title).toContain("EvenFlow");
  });

  it("render trang thanh toán với thông tin sự kiện", async () => {
    const pageComponent = await PaymentPage({
      params: Promise.resolve({ eventId: "evt-tech-summit-2026" }),
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
