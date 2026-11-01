/** AC-7 (R3-1), AC-8 (R3-2), AC-9 (R3-3): expired session on the result page. Timing tests step act() per tick. */
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { OrderDetails } from "@/lib/payment-client";
import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

import { OrderResultView } from "./order-result-view";

const AUTH_TEXT = "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.";
const LOAD_ERROR = "Không tải được đơn hàng";
const TICKETS_FAILED = "Chưa lấy được vé điện tử";

beforeEach(() => {
  setupEnv();
  seedSession("acc", "r1");
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  resetEnv();
});

async function step(totalMs: number, stepMs = 1000) {
  for (let t = 0; t < totalMs; t += stepMs) {
    await act(async () => {
      await vi.advanceTimersByTimeAsync(stepMs);
    });
  }
}

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
const unauthorized = () => problem(401, "/problems/unauthorized");
const orderGets = (calls: { method: string; path: string }[]) =>
  calls.filter((c) => c.method === "GET" && c.path === "/v1/orders/o1").length;
const issuePosts = (calls: { method: string; path: string }[]) =>
  calls.filter((c) => c.method === "POST" && c.path.endsWith("/issue-tickets")).length;
const signInLinks = () => screen.queryAllByRole("link", { name: "Đăng nhập" });
const emptyParagraphs = () =>
  screen.queryAllByRole("alert").flatMap((a) => Array.from(a.querySelectorAll("p")).filter((p) => (p.textContent ?? "").trim() === ""));

describe("AC-7 401 on the first order GET (no order on screen)", () => {
  it("R3-1: shows the auth title and one sign-in link, no 'Thử lại', no generic load error; 1 GET over 120 s", async () => {
    const { calls } = installFetch(() => unauthorized());
    render(<OrderResultView eventId="e1" orderId="o1" />);
    await step(120_000);
    expect(screen.queryByText(AUTH_TEXT)).not.toBeNull();
    const links = signInLinks();
    expect(links).toHaveLength(1);
    expect(links[0].getAttribute("href")).toBe("/auth/sign-in");
    expect(screen.queryByRole("button", { name: /Thử lại/ })).toBeNull();
    expect(screen.queryByText(LOAD_ERROR)).toBeNull();
    expect(orderGets(calls)).toBe(1);
  });

  it("GUARD: network error on the first GET keeps the generic load error and 'Thử lại'; clicking retries", async () => {
    const { calls } = installFetch(() => Promise.reject(new TypeError("Failed to fetch")));
    render(<OrderResultView eventId="e1" orderId="o1" />);
    await step(5_000);
    expect(screen.queryByText(LOAD_ERROR)).not.toBeNull();
    expect(signInLinks()).toHaveLength(0);
    const before = orderGets(calls);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /Thử lại/ }));
      await vi.advanceTimersByTimeAsync(10);
    });
    expect(orderGets(calls)).toBeGreaterThan(before);
  });

  it("GUARD: 500 on the first GET keeps 'Thử lại' and the generic error", async () => {
    installFetch(() => problem(500, "about:blank"));
    render(<OrderResultView eventId="e1" orderId="o1" />);
    await step(5_000);
    expect(screen.queryByRole("button", { name: /Thử lại/ })).not.toBeNull();
    expect(signInLinks()).toHaveLength(0);
  });

  it("GUARD: PENDING order on screen then 401 still shows the prompt with one link", async () => {
    let n = 0;
    installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") return problem(401, "/problems/invalid-token");
      n += 1;
      return n === 1 ? json(wirePending) : problem(401, "/problems/token-expired");
    });
    render(<OrderResultView eventId="e1" orderId="o1" />);
    await step(60_000);
    expect(screen.queryByText(AUTH_TEXT)).not.toBeNull();
    expect(signInLinks()).toHaveLength(1);
    expect(screen.queryByRole("button", { name: /Thử lại/ })).toBeNull();
  });
});

describe("AC-8 no empty paragraph in the auth prompt", () => {
  it("no order: the alert has no empty <p>", async () => {
    installFetch(() => unauthorized());
    render(<OrderResultView eventId="e1" orderId="o1" />);
    await step(5_000);
    expect(screen.queryByText(AUTH_TEXT)).not.toBeNull();
    expect(emptyParagraphs()).toHaveLength(0);
  });

  it("PENDING order then 401: the alert has no empty <p>", async () => {
    let n = 0;
    installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") return problem(401, "/problems/invalid-token");
      n += 1;
      return n === 1 ? json(wirePending) : problem(401, "/problems/token-expired");
    });
    render(<OrderResultView eventId="e1" orderId="o1" />);
    await step(60_000);
    expect(screen.queryByText(AUTH_TEXT)).not.toBeNull();
    expect(emptyParagraphs()).toHaveLength(0);
  });

  it("GUARD: other error kinds keep their body text", async () => {
    installFetch(() => problem(404, "/problems/not-found"));
    render(<OrderResultView eventId="e1" orderId="o1" />);
    await step(5_000);
    expect(screen.queryByText("Không tìm thấy đơn hàng")).not.toBeNull();
    const alert = screen.getByRole("alert");
    expect(alert.querySelectorAll("p").length).toBeGreaterThanOrEqual(2);
    expect(emptyParagraphs()).toHaveLength(0);
  });
});

describe("AC-9 ticket issuing stops on an expired session", () => {
  it("R3-3: PAID + issue-tickets 401 -> one prompt with sign-in link, no 'Lấy vé lại', no failed alert, 1 POST over 120 s", async () => {
    const { calls } = installFetch(() => unauthorized());
    render(<OrderResultView eventId="e1" orderId="o1" initialOrder={paid} />);
    await step(120_000);
    expect(screen.queryByText(AUTH_TEXT)).not.toBeNull();
    expect(signInLinks()).toHaveLength(1);
    expect(signInLinks()[0].getAttribute("href")).toBe("/auth/sign-in");
    expect(screen.queryByRole("button", { name: /Lấy vé lại/ })).toBeNull();
    expect(screen.queryByText(TICKETS_FAILED)).toBeNull();
    expect(issuePosts(calls)).toBe(1);
  });

  it("the auth prompt appears exactly once (no duplicate text)", async () => {
    installFetch(() => unauthorized());
    render(<OrderResultView eventId="e1" orderId="o1" initialOrder={paid} />);
    await step(10_000);
    expect(screen.queryAllByText(AUTH_TEXT)).toHaveLength(1);
  });

  it("GUARD (F-13): issue-tickets 404 still shows 'Lấy vé lại' (failed stays true) and no sign-in prompt", async () => {
    installFetch(() => problem(404, "/problems/not-found"));
    render(<OrderResultView eventId="e1" orderId="o1" initialOrder={paid} />);
    await step(10_000);
    expect(screen.queryByRole("button", { name: /Lấy vé lại/ })).not.toBeNull();
    expect(signInLinks()).toHaveLength(0);
  });

  it("GUARD: issue-tickets 503 keeps retrying with backoff (more than one POST) and no sign-in prompt", async () => {
    const { calls } = installFetch((c) =>
      c.path.endsWith("/issue-tickets") ? problem(503, "about:blank") : json({}, 200),
    );
    render(<OrderResultView eventId="e1" orderId="o1" initialOrder={paid} />);
    await step(20_000);
    expect(issuePosts(calls)).toBeGreaterThan(1);
    expect(signInLinks()).toHaveLength(0);
  });
});
