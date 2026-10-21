/** F-1 (X-1): 403 /problems/forbidden on the order poll is shown even when a PENDING order is already on screen. */
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

const pending: OrderDetails = {
  orderId: "o1",
  eventId: "e1",
  eventTitle: "Rock",
  venue: "HN",
  tierName: "VIP",
  quantity: 1,
  totalAmount: 100000,
  status: "PENDING",
  items: [{ ticketTypeId: "t1", tierName: "VIP", quantity: 1, unitCents: 100000 }],
};

/** Stepped act() calls: a single big advance batches React updates and would make "stops polling" pass vacuously. */
async function step(totalMs: number, stepMs = 1000) {
  for (let t = 0; t < totalMs; t += stepMs) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(stepMs);
    });
  }
}

describe("OrderResultView forbidden with a PENDING order on screen (F-1)", () => {
  it("initialOrder PENDING + GET 403: forbidden alert shown, no spinner, no manual check, polling stops", async () => {
    const { calls } = installFetch(() => problem(403, "/problems/forbidden", "not yours"));
    render(<OrderResultView eventId="e1" orderId="o1" initialOrder={pending} />);
    await step(120_000);
    expect(screen.queryByText(FORBIDDEN_TEXT)).not.toBeNull();
    expect(screen.queryByText("Đang xác nhận thanh toán")).toBeNull();
    expect(screen.queryByRole("button", { name: /Kiểm tra trạng thái ngay/ })).toBeNull();
    expect(calls.filter((c) => c.method === "GET" && c.path === "/v1/orders/o1")).toHaveLength(1);
  });
});
