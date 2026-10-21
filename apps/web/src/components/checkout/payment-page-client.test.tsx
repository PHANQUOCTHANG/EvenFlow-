import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { saveActiveHold } from "@/lib/hold-client";
import { installFetch, json, resetEnv, setupEnv } from "@/test-utils/fetch-mock";
import { PaymentPageClient } from "./payment-page-client";

vi.mock("next/navigation", () => ({
  notFound: vi.fn(() => {
    throw new Error("NEXT_NOT_FOUND");
  }),
}));

const mockView = vi.fn();
vi.mock("@/components/checkout/payment-handoff-view", () => ({
  PaymentHandoffView: (props: Record<string, unknown>) => {
    mockView(props);
    return <div data-testid="handoff">{String(props.tierName)}</div>;
  },
}));

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
const TIER = "11111111-1111-4111-8111-111111111111";

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
      tiers: [{ id: TIER, name: "Standard Pass", price_cents: 900000, currency: "VND" }],
    }),
  );
});
afterEach(resetEnv);

describe("PaymentPageClient (AC-18)", () => {
  it("tai su kien phia client va truyen hang ve, don gia, so luong vao PaymentHandoffView", async () => {
    render(<PaymentPageClient eventId={EVT} orderId="ord-1" holdId="hld-1" tierId={TIER} quantity="2" />);

    expect(await screen.findByTestId("handoff")).toHaveTextContent("Standard Pass");
    const props = mockView.mock.calls.at(-1)![0];
    expect(props).toMatchObject({
      eventId: EVT,
      eventTitle: "Vietnam Tech Summit 2026",
      orderId: "ord-1",
      holdId: "hld-1",
      tierName: "Standard Pass",
      quantity: 2,
      unitPrice: 900000,
    });
  });

  it("lay han giu ve cua server tu hold luu cuc bo khi holdId khop", async () => {
    saveActiveHold(EVT, {
      orderId: "ord-1",
      holdId: "hld-1",
      expiresAt: "2099-01-01T00:00:00Z",
      ticketTypeId: TIER,
      quantity: 2,
    });
    render(<PaymentPageClient eventId={EVT} orderId="ord-1" holdId="hld-1" tierId={TIER} quantity="2" />);
    await screen.findByTestId("handoff");

    await vi.waitFor(() => expect(mockView.mock.calls.at(-1)![0]).toMatchObject({ expiresAt: "2099-01-01T00:00:00Z" }));
  });

  it("thieu orderId/holdId/hang ve: bao loi, KHONG bia id va KHONG render form thanh toan", async () => {
    render(<PaymentPageClient eventId={EVT} />);

    expect(await screen.findByText("Thiếu thông tin đơn hàng")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Quay lại chọn vé" })).toHaveAttribute("href", `/checkout/${EVT}`);
    expect(mockView).not.toHaveBeenCalled();
  });

  it("tierId khong khop hang ve nao cua su kien -> cung bao loi (khong roi ve hang ve dau tien)", async () => {
    render(<PaymentPageClient eventId={EVT} orderId="ord-1" holdId="hld-1" tierId="khong-ton-tai" />);

    expect(await screen.findByText("Thiếu thông tin đơn hàng")).toBeInTheDocument();
    expect(mockView).not.toHaveBeenCalled();
  });
});
