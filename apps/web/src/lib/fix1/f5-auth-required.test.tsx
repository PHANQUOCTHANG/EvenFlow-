/** Regression F-5 (X-2, X-3): non-refreshable 401 -> AUTH_REQUIRED; CheckoutView never silent. */
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { CheckoutView } from "@/components/checkout/checkout-view";
import { cancelHold, requestHold } from "@/lib/hold-client";
import { getOrderDetails, initiatePayment, issueTickets } from "@/lib/payment-client";
import { saveQueueToken } from "@/lib/queue-token";
import type { EventDetailSnapshot } from "@/lib/event-service";
import { installFetch, json, problem, rejection, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

vi.mock("@/hooks/use-server-time-sync", () => ({
  useServerTimeSync: () => ({ offsetMs: 0, synced: true, isLoading: false, error: null, getServerNow: () => Date.now() }),
}));

beforeEach(setupEnv);
afterEach(resetEnv);

const HOLD = { eventId: "e1", ticketTypeId: "t1", quantity: 1, queueToken: "q" };
const PAY = {
  eventId: "e1", orderId: "o1", holdId: "h1", amount: 1, method: "sandbox" as const,
  buyerName: "a", buyerEmail: "a@b.c", buyerPhone: "0912345678",
};
const code = (e: unknown) => (e as { type?: string }).type;

const event: EventDetailSnapshot = {
  id: "e1", slug: "s", title: "T", description: "", venue: "V",
  startsAt: "2026-12-01T00:00:00Z", saleStartAt: "2026-11-01T00:00:00Z", state: "ON_SALE", maxPerOrder: 4,
  tiers: [{ id: "t1", name: "Std", price: 1000, currency: "VND", availability: "AVAILABLE" }],
} as unknown as EventDetailSnapshot;

describe("F-5 clients", () => {
  it("hold: no session 401 -> AUTH_REQUIRED", async () => {
    installFetch(() => problem(401, "/problems/unauthorized"));
    expect(code(await rejection(requestHold(HOLD)))).toBe("AUTH_REQUIRED");
  });
  it("hold: retried request still token-expired -> AUTH_REQUIRED", async () => {
    seedSession("old", "r1");
    installFetch((c) =>
      c.path === "/api/v1/auth/refresh"
        ? json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2" })
        : problem(401, "/problems/token-expired"),
    );
    expect(code(await rejection(requestHold(HOLD)))).toBe("AUTH_REQUIRED");
  });
  it("hold: queue-token 401 stays QUEUE_TOKEN_INVALID", async () => {
    installFetch(() => problem(401, "/problems/queue-token-expired"));
    expect(code(await rejection(requestHold(HOLD)))).toBe("QUEUE_TOKEN_INVALID");
  });
  it("cancelHold 401 -> AUTH_REQUIRED", async () => {
    installFetch(() => problem(401, "/problems/unauthorized"));
    expect(code(await rejection(cancelHold("h1")))).toBe("AUTH_REQUIRED");
  });
  it("payment / order / issue-tickets 401 -> AUTH_REQUIRED", async () => {
    installFetch(() => problem(401, "/problems/unauthorized"));
    expect(code(await rejection(initiatePayment(PAY)))).toBe("AUTH_REQUIRED");
    expect(code(await rejection(getOrderDetails("o1", "e1")))).toBe("AUTH_REQUIRED");
    expect(code(await rejection(issueTickets("o1")))).toBe("AUTH_REQUIRED");
  });
});

describe("F-5 CheckoutView", () => {
  async function click() {
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /Giữ vé và thanh toán/ }));
    });
    await act(async () => {
      await Promise.resolve();
    });
  }
  it("401 shows the sign-in prompt", async () => {
    saveQueueToken("e1", "q-token");
    installFetch(() => problem(401, "/problems/unauthorized"));
    render(<CheckoutView event={event} initialQueueToken="q-token" />);
    await click();
    expect(screen.getByRole("link", { name: /Đăng nhập/ })).toHaveAttribute("href", "/auth/sign-in");
  });
  it("HTTP 500 shows a visible generic error", async () => {
    saveQueueToken("e1", "q-token");
    installFetch(() => problem(500, "/problems/internal"));
    render(<CheckoutView event={event} initialQueueToken="q-token" />);
    await click();
    expect(screen.getByRole("alert").textContent).toMatch(/500/);
  });
});
