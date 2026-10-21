/** AC-6 UI: 403 /problems/forbidden on order polling or ticket issuing shows checkout.forbidden and stops retrying. */
import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { OrderDetails } from "@/lib/payment-client";
import { installFetch, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

import { OrderResultView } from "./order-result-view";

const FORBIDDEN_TEXT = "Tài khoản của bạn không có quyền mua vé cho sự kiện này.";

beforeEach(() => {
  setupEnv();
  seedSession("acc", "r1");
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  resetEnv();
});

const settle = async (ms: number) => {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
};

describe("OrderResultView 403 forbidden (AC-6)", () => {
  it("order poll 403: shows checkout.forbidden, no retry button, and does not poll again (1 GET over 120 s)", async () => {
    const { calls } = installFetch(() => problem(403, "/problems/forbidden", "not yours"));
    render(<OrderResultView eventId="e1" orderId="o1" />);
    await settle(120_000);
    expect(screen.getByText(FORBIDDEN_TEXT)).toBeDefined();
    expect(screen.queryByRole("button", { name: /Thử lại/ })).toBeNull();
    expect(calls.filter((c) => c.method === "GET" && c.path === "/v1/orders/o1")).toHaveLength(1);
  });

  it("issue-tickets 403 on a PAID order: shows checkout.forbidden and stops after 1 POST (no 6-attempt retry loop)", async () => {
    const paid: OrderDetails = {
      orderId: "o1",
      eventId: "e1",
      eventTitle: "Rock",
      venue: "HN",
      tierName: "VIP",
      quantity: 1,
      totalAmount: 100000,
      status: "PAID",
      items: [{ ticketTypeId: "t1", tierName: "VIP", quantity: 1, unitCents: 100000 }],
    };
    const { calls } = installFetch(() => problem(403, "/problems/forbidden", "not yours"));
    render(<OrderResultView eventId="e1" orderId="o1" initialOrder={paid} />);
    await settle(120_000);
    expect(screen.getByText(FORBIDDEN_TEXT)).toBeDefined();
    expect(calls.filter((c) => c.method === "POST" && c.path.endsWith("/issue-tickets"))).toHaveLength(1);
  });
});
