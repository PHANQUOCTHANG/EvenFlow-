import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as holdClient from "@/lib/hold-client";
import type { EventDetailSnapshot } from "@/lib/types/event";
import { CheckoutView } from "./checkout-view";

const event: EventDetailSnapshot = {
  id: "evt-g9",
  slug: "evt-g9",
  title: "G9",
  description: "",
  venue: "Mỹ Đình",
  startsAt: "2026-11-20T19:00:00+07:00",
  saleStartAt: "2026-10-15T10:00:00+07:00",
  state: "ON_SALE",
  maxPerOrder: 4,
  tiers: [{ id: "t1", name: "Vé", price: 100000, currency: "VND", availability: "AVAILABLE" }],
};

describe("CheckoutView - G-9", () => {
  beforeEach(() => {
    sessionStorage.clear();
    sessionStorage.setItem("ef:queue:token:evt-g9", "tok");
  });
  afterEach(() => {
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it("FORBIDDEN shows a distinct message, no 'back to waiting room', token kept", async () => {
    vi.spyOn(holdClient, "requestHold").mockRejectedValue(new holdClient.HoldClientError("x", "FORBIDDEN", 403));
    await act(async () => {
      render(<CheckoutView event={event} />);
    });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Giữ vé và thanh toán" }));
    });
    expect(screen.getByText("Tài khoản của bạn không có quyền mua vé cho sự kiện này.")).toBeInTheDocument();
    expect(screen.queryByText(/phòng chờ/)).toBeNull();
    expect(sessionStorage.getItem("ef:queue:token:evt-g9")).toBe("tok");
  });
});
