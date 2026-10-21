import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, resetEnv, setupEnv } from "@/test-utils/fetch-mock";
import { ResultPageClient } from "./result-page-client";

vi.mock("next/navigation", () => ({
  notFound: vi.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
}));

const mockView = vi.fn();
vi.mock("@/components/checkout/order-result-view", () => ({
  OrderResultView: (props: Record<string, unknown>) => {
    mockView(props);
    return <div data-testid="result-view">{String(props.orderId)}</div>;
  },
}));

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

beforeEach(() => {
  setupEnv();
  mockView.mockClear();
  installFetch(() =>
    json({
      id: EVT,
      slug: "s",
      title: "Vietnam Tech Summit 2026",
      description: "",
      venue: "Hà Nội",
      starts_at: "2026-12-05T01:30:00Z",
      sale_start_at: "2026-09-01T02:00:00Z",
      status: "SCHEDULED",
      max_per_order: 4,
      tiers: [],
    }),
  );
});
afterEach(resetEnv);

describe("ResultPageClient (AC-18)", () => {
  it("tai su kien phia client roi hien OrderResultView voi orderId tu URL", async () => {
    render(<ResultPageClient eventId={EVT} orderId="ord-77" />);

    expect(await screen.findByTestId("result-view")).toHaveTextContent("ord-77");
    expect(mockView.mock.calls.at(-1)![0]).toMatchObject({ eventId: EVT, orderId: "ord-77" });
  });

  it("thieu orderId: bao loi, KHONG bia ma don", async () => {
    render(<ResultPageClient eventId={EVT} />);

    expect(await screen.findByText("Thiếu mã đơn hàng")).toBeInTheDocument();
    expect(mockView).not.toHaveBeenCalled();
  });
});
