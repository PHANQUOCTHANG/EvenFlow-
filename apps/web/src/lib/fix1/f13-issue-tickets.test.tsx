/** Regression F-13 (C-10, C-11, X-19): empty issue-tickets 2xx, headline, no retries on AUTH_REQUIRED / NOT_FOUND. */
import { act, render, renderHook, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OrderResultView } from "@/components/checkout/order-result-view";
import { useIssuedTickets } from "@/hooks/use-order-status";
import { issueTickets, type OrderDetails } from "@/lib/payment-client";
import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession();
});
afterEach(() => {
  vi.useRealTimers();
  resetEnv();
});

const keyOf = (calls: { headers: Headers }[], i: number) => calls[i].headers.get("Idempotency-Key");
const paid: OrderDetails = {
  orderId: "o1", eventId: "e1", eventTitle: "Concert", venue: "V", tierName: "VIP", quantity: 1, totalAmount: 1,
  status: "PAID", items: [{ ticketTypeId: "t1", tierName: "VIP", quantity: 1, unitCents: 1 }],
};

describe("F-13 key rotation", () => {
  it("2xx with an empty ticket list forgets the key so a retry is not replayed", async () => {
    const { calls } = installFetch(() => json({ tickets: [] }));
    expect(await issueTickets("o1")).toEqual([]);
    await issueTickets("o1");
    expect(keyOf(calls, 0)).not.toBe(keyOf(calls, 1));
  });

  it("2xx with tickets lacking qr_payload also rotates the key", async () => {
    const { calls } = installFetch(() => json({ tickets: [{ id: "x", status: "PENDING" }] }));
    await issueTickets("o1");
    await issueTickets("o1");
    expect(keyOf(calls, 0)).not.toBe(keyOf(calls, 1));
  });

  it("2xx with QR tickets keeps the key (reload stays idempotent)", async () => {
    const { calls } = installFetch(() => json({ tickets: [{ id: "x", ticket_type_id: "t1", status: "ISSUED", qr_payload: "QR" }] }));
    await issueTickets("o1");
    await issueTickets("o1");
    expect(keyOf(calls, 0)).toBe(keyOf(calls, 1));
  });

  it("hook retry() starts with a fresh key", async () => {
    sessionStorage.setItem("ef:issue:o1", "stale-key");
    const { calls } = installFetch(() => problem(500, "/problems/x"));
    const { result } = renderHook(() => useIssuedTickets("o1", true, { maxAttempts: 1 }));
    await act(async () => {
      await new Promise((r) => setTimeout(r, 20));
    });
    expect(result.current.failed).toBe(true);
    sessionStorage.setItem("ef:issue:o1", "stale-key-2");
    await act(async () => {
      result.current.retry();
      await new Promise((r) => setTimeout(r, 20));
    });
    expect(calls[calls.length - 1].headers.get("Idempotency-Key")).not.toBe("stale-key-2");
  });
});

describe("F-13 no useless retries", () => {
  it.each([
    ["AUTH_REQUIRED", 401, "/problems/unauthorized"],
    ["NOT_FOUND", 404, "/problems/not-found"],
  ])("%s stops after one attempt and ends in failed", async (_n, status, type) => {
    const { calls } = installFetch(() => problem(status, type));
    const onRetry = vi.fn();
    const { result } = renderHook(() => useIssuedTickets("o1", true, { baseDelayMs: 5, onRetry }));
    await act(async () => {
      await new Promise((r) => setTimeout(r, 120));
    });
    expect(calls.filter((c) => c.path.endsWith("/issue-tickets"))).toHaveLength(1);
    expect(onRetry).not.toHaveBeenCalled();
    expect(result.current.failed).toBe(true);
    expect(result.current.pending).toBe(false);
  });
});

describe("F-13 headline", () => {
  it("does not say tickets are ready while issuing", async () => {
    installFetch(() => json({ tickets: [] }));
    render(<OrderResultView eventId="e1" orderId="o1" initialOrder={paid} />);
    await act(async () => {
      await Promise.resolve();
    });
    expect(screen.queryByText(/đã sẵn sàng/i)).toBeNull();
    expect(screen.getByRole("heading", { level: 2, name: /đang phát hành vé/i })).toBeInTheDocument();
  });

  it("says ready once QR tickets exist", async () => {
    installFetch(() => json({ tickets: [{ id: "abcdef12-0", ticket_type_id: "t1", status: "ISSUED", qr_payload: "QR" }] }));
    render(<OrderResultView eventId="e1" orderId="o1" initialOrder={paid} />);
    expect(await screen.findByRole("heading", { level: 2, name: /đã sẵn sàng/i })).toBeInTheDocument();
  });
});
