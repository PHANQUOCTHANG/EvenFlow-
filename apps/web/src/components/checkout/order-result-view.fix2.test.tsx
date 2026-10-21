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

describe("G-2 terminal poll errors on a PENDING order stop the silent spinner", () => {
  it("NOT_FOUND: not-found message shown, no spinner, polling stopped", async () => {
    let n = 0;
    const { calls } = installFetch(() => {
      n += 1;
      return n === 1 ? json(wirePending) : problem(404, "/problems/not-found", "gone");
    });
    render(<OrderResultView eventId="e1" orderId="o1" />);
    await step(120_000);
    expect(orderGets(calls)).toBe(2);
    expect(screen.queryByText("Không tìm thấy đơn hàng")).not.toBeNull();
    expect(screen.queryByText(SPINNER_TITLE)).toBeNull();
    expect(screen.queryByRole("button", { name: /Kiểm tra trạng thái ngay/ })).toBeNull();
  });

  it("AUTH_REQUIRED: sign-in prompt with link, no spinner, polling stopped", async () => {
    let n = 0;
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") return problem(401, "/problems/invalid-token");
      n += 1;
      return n === 1 ? json(wirePending) : problem(401, "/problems/token-expired");
    });
    render(<OrderResultView eventId="e1" orderId="o1" />);
    await step(120_000);
    expect(orderGets(calls)).toBe(2);
    expect(screen.queryByText("Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.")).not.toBeNull();
    expect(screen.getByRole("link", { name: "Đăng nhập" }).getAttribute("href")).toBe("/auth/sign-in");
    expect(screen.queryByText(SPINNER_TITLE)).toBeNull();
  });

  it("guard: network error keeps polling with spinner and manual button", async () => {
    let n = 0;
    const { calls } = installFetch(() => {
      n += 1;
      return n === 1 ? json(wirePending) : Promise.reject(new TypeError("Failed to fetch"));
    });
    render(<OrderResultView eventId="e1" orderId="o1" />);
    await step(30_000);
    expect(orderGets(calls)).toBeGreaterThan(3);
    expect(screen.queryByText(SPINNER_TITLE)).not.toBeNull();
    expect(screen.queryByRole("button", { name: /Kiểm tra trạng thái ngay/ })).not.toBeNull();
    expect(screen.queryByText(FORBIDDEN_TEXT)).toBeNull();
  });
});
