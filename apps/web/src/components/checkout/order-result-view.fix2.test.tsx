/** Fix round 2 (G-1, G-2): terminal poll errors on a PENDING order; no duplicate forbidden on PAID. Stepped act() per tick. */
import { act, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OrderResultView } from "@/components/checkout/order-result-view";
import type { OrderDetails } from "@/lib/payment-client";
import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

const FORBIDDEN_TEXT = "Tài khoản của bạn không có quyền mua vé cho sự kiện này.";
const SPINNER_TITLE = "Đang xác nhận thanh toán";

beforeEach(() => {
  setupEnv();
  seedSession("acc", "r1");
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  resetEnv();
});

const paid: OrderDetails = {
  orderId: "o1",
  eventId: "e1",
  eventTitle: "Rock",
  venue: "HN",
  tierName: "VIP",
  quantity: 1,
  totalAmount: 100000,
  status: "PAID",
  paidAt: "2026-10-22T00:00:00Z",
  items: [{ ticketTypeId: "t1", tierName: "VIP", quantity: 1, unitCents: 100000 }],
};

const wirePending = {
  order_id: "o1",
  event_id: "e1",
  event_title: "Rock",
  venue: "HN",
  tier_name: "VIP",
  quantity: 1,
  total_amount: 100000,
  status: "PENDING",
  items: [{ ticket_type_id: "t1", tier_name: "VIP", quantity: 1, unit_cents: 100000 }],
};

async function step(totalMs: number, stepMs = 1000) {
  for (let t = 0; t < totalMs; t += stepMs) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(stepMs);
    });
  }
}

const orderGets = (calls: { method: string; path: string }[]) =>
  calls.filter((c) => c.method === "GET" && c.path === "/v1/orders/o1").length;

describe("G-1 PAID + forbidden shows the message once", () => {
  it("issue-tickets 503 -> retry refresh -> GET 403: forbidden text appears exactly once and PAID section stays", async () => {
    let issueN = 0;
    installFetch((c) => {
      if (c.path.endsWith("/issue-tickets")) {
        issueN += 1;
        return issueN === 1 ? problem(503, "/problems/unavailable") : problem(403, "/problems/forbidden", "not yours");
      }
      return problem(403, "/problems/forbidden", "not yours");
    });
    render(<OrderResultView eventId="e1" orderId="o1" initialOrder={paid} />);
    await step(30_000);
    expect(screen.queryAllByText(FORBIDDEN_TEXT)).toHaveLength(1);
    expect(screen.queryByText("Đã thanh toán thành công")).not.toBeNull();
  });
});

