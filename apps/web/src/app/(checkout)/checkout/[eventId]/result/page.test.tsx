import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import OrderResultPage, { generateMetadata } from "./page";

describe("OrderResultPage route (EVF-114, EV-184)", () => {
  it("generateMetadata tạo tiêu đề chứa tên sự kiện", async () => {
    const meta = await generateMetadata({
      params: Promise.resolve({ eventId: "evt-tech-summit-2026" }),
      searchParams: Promise.resolve({}),
    });

    expect(meta.title).toContain("EvenFlow");
  });

  it("render trang kết quả đơn hàng với mã đơn", async () => {
    const pageComponent = await OrderResultPage({
      params: Promise.resolve({ eventId: "evt-tech-summit-2026" }),
      searchParams: Promise.resolve({
        orderId: "ord-test-999",
      }),
    });

    render(pageComponent);

    expect(screen.getByRole("heading", { level: 1, name: /Kết quả đơn hàng/i })).toBeDefined();
    expect(screen.getByText(/ord-test-999/i)).toBeDefined();
  });
});
