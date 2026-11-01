/** AC-11 focus handling (plan-review R2-3): focus returns to the trigger after "Giữ lại"; after a confirmed release the
 *  trigger is gone, so focus lands on the OrderSummary primary button, not on <body>. */
import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { EventDetailSnapshot } from "@/lib/types/event";
import { installFetch, json, noContent, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

import { CheckoutView } from "./checkout-view";

const event: EventDetailSnapshot = {
  id: "evt-f",
  slug: "evt-f",
  title: "F",
  description: "",
  venue: "V",
  startsAt: "2026-11-20T19:00:00+07:00",
  saleStartAt: "2026-10-15T10:00:00+07:00",
  state: "ON_SALE",
  maxPerOrder: 4,
  tiers: [{ id: "t1", name: "Vé", price: 100000, currency: "VND", availability: "AVAILABLE" }],
};

beforeEach(() => {
  setupEnv();
  seedSession("acc", "r1");
  sessionStorage.setItem("ef:queue:token:evt-f", "tok");
  sessionStorage.setItem(
    "evenflow_hold_evt-f",
    JSON.stringify({ orderId: "o1", holdId: "h1", ticketTypeId: "t1", quantity: 1, expiresAt: new Date(Date.now() + 300_000).toISOString() }),
  );
  vi.useFakeTimers();
  installFetch((c) => (c.path === "/api/time" ? json({}, 500) : c.method === "DELETE" ? noContent() : json({})));
});
afterEach(() => {
  vi.useRealTimers();
  resetEnv();
});

const tick = async (ms = 0) => {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
};

async function openFromTrigger() {
  await act(async () => {
    render(<CheckoutView event={event} />);
  });
  await tick(10);
  const trigger = screen.getByRole("button", { name: "Huỷ giữ vé" });
  await act(async () => {
    fireEvent.click(trigger);
  });
  return trigger;
}

describe("AC-11 focus", () => {
  it("focus moves into the dialog, and back to the trigger after 'Giữ lại'", async () => {
    const trigger = await openFromTrigger();
    expect(screen.getByRole("alertdialog").contains(document.activeElement)).toBe(true);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Giữ lại" }));
    });
    expect(document.activeElement).toBe(trigger);
  });

  it("after a confirmed release focus is on the primary button, not body", async () => {
    await openFromTrigger();
    const confirm = screen.getAllByRole("button", { name: "Huỷ giữ vé" }).find((b) => b.closest("dialog"));
    await act(async () => {
      fireEvent.click(confirm as HTMLElement);
    });
    await tick(50);
    expect(screen.queryByRole("alertdialog")).toBeNull();
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Giữ vé và thanh toán" }));
  });
});
