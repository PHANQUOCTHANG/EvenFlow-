/** AC-11 (C-6): "Huỷ giữ vé" with a confirmation dialog (human decision) -> DELETE /v1/holds/{holdId}.
 *  Assumed interface: the confirmation is an element with role="alertdialog" (or an open <dialog>, role "dialog") containing a
 *  confirm button (name matches /huỷ giữ vé|xác nhận|đồng ý|confirm|release|yes/i) and at least one other button that
 *  dismisses; Escape dismisses. i18n keys checkout.cancelHold / checkout.cancelHoldFailed in MESSAGES.vi. */
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as holdClient from "@/lib/hold-client";
import { MESSAGES } from "@/lib/i18n/messages";
import type { EventDetailSnapshot } from "@/lib/types/event";
import { type Call, installFetch, json, noContent, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

import { CheckoutView } from "./checkout-view";

const event: EventDetailSnapshot = {
  id: "evt-d5",
  slug: "evt-d5",
  title: "D5",
  description: "",
  venue: "Mỹ Đình",
  startsAt: "2026-11-20T19:00:00+07:00",
  saleStartAt: "2026-10-15T10:00:00+07:00",
  state: "ON_SALE",
  maxPerOrder: 4,
  tiers: [{ id: "t1", name: "Vé", price: 100000, currency: "VND", availability: "AVAILABLE" }],
};
const CANCEL = /^Huỷ giữ vé$/;
const CONFIRM_RE = /huỷ giữ vé|xác nhận|đồng ý|confirm|release|yes/i;
const AUTH_TEXT = "Phiên đăng nhập đã hết hạn. Vui lòng đăng nhập lại.";

beforeEach(() => {
  setupEnv();
  seedSession("acc", "r1");
  sessionStorage.setItem("ef:queue:token:evt-d5", "tok");
  vi.useFakeTimers();
});
afterEach(() => {
  vi.restoreAllMocks();
  vi.useRealTimers();
  resetEnv();
});

function seedHold(msFromNow = 5 * 60_000) {
  sessionStorage.setItem(
    "evenflow_hold_evt-d5",
    JSON.stringify({ orderId: "o1", holdId: "h1", ticketTypeId: "t1", quantity: 1, expiresAt: new Date(Date.now() + msFromNow).toISOString() }),
  );
}
const tick = async (ms = 0) => {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
};
const deletes = (calls: Call[]) => calls.filter((c) => c.method === "DELETE" && c.path === "/v1/holds/h1");
const summaryState = () => document.querySelector("[data-hold-state]")?.getAttribute("data-hold-state");
const dialog = () => screen.queryByRole("alertdialog") ?? screen.queryByRole("dialog");
const confirmBtn = () => within(dialog() as HTMLElement).getAllByRole("button").find((b) => CONFIRM_RE.test(b.textContent ?? "")) as HTMLElement;
const dismissBtn = () => within(dialog() as HTMLElement).getAllByRole("button").find((b) => !CONFIRM_RE.test(b.textContent ?? "")) as HTMLElement;
const openDialog = async () => {
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: CANCEL }));
  });
  expect(dialog()).not.toBeNull();
};
const cancelHoldFailedText = () => (MESSAGES.vi as Record<string, string>)["checkout.cancelHoldFailed"];

/** Fetch mock: /api/time fails (client clock), DELETE answered by `del`. */
function server(del: () => Response | Promise<Response> = () => noContent()) {
  return installFetch((c) => {
    if (c.path === "/api/time") return json({}, 500);
    if (c.method === "DELETE") return del();
    return json({});
  });
}
async function renderActive(del?: () => Response | Promise<Response>, ms?: number) {
  const s = server(del);
  seedHold(ms);
  await act(async () => {
    render(<CheckoutView event={event} />);
  });
  await tick(10);
  return s;
}

describe("AC-11 button visibility", () => {
  it("active hold: secondary 'Huỷ giữ vé' button is shown, i18n key exists in vi and en", async () => {
    await renderActive();
    expect(summaryState()).toBe("active");
    expect(screen.queryByRole("button", { name: CANCEL })).not.toBeNull();
    expect((MESSAGES.vi as Record<string, string>)["checkout.cancelHold"]).toBe("Huỷ giữ vé");
    expect((MESSAGES.en as Record<string, string>)["checkout.cancelHold"]).toBe("Release tickets");
    expect(typeof cancelHoldFailedText()).toBe("string");
  });

  it("idle: no button", async () => {
    server();
    await act(async () => {
      render(<CheckoutView event={event} />);
    });
    expect(screen.queryByRole("button", { name: CANCEL })).toBeNull();
  });

  it("creating: no button", async () => {
    server();
    vi.spyOn(holdClient, "requestHold").mockReturnValue(new Promise(() => {}));
    await act(async () => {
      render(<CheckoutView event={event} />);
    });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Giữ vé và thanh toán" }));
    });
    expect(summaryState()).toBe("creating");
    expect(screen.queryByRole("button", { name: CANCEL })).toBeNull();
  });

  it.each([
    ["sold_out", "SOLD_OUT", 409],
    ["ambiguous", "AMBIGUOUS", 0],
  ] as const)("%s: no button", async (state, type, status) => {
    server();
    vi.spyOn(holdClient, "requestHold").mockRejectedValue(new holdClient.HoldClientError("x", type, status));
    await act(async () => {
      render(<CheckoutView event={event} />);
    });
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Giữ vé và thanh toán" }));
    });
    expect(summaryState()).toBe(state);
    expect(screen.queryByRole("button", { name: CANCEL })).toBeNull();
  });

  it("expired: no button", async () => {
    await renderActive(undefined, 3_000);
    for (let i = 0; i < 5; i++) await tick(1000);
    expect(summaryState()).toBe("expired");
    expect(screen.queryByRole("button", { name: CANCEL })).toBeNull();
  });
});

describe("AC-11 confirmation dialog", () => {
  it("clicking 'Huỷ giữ vé' opens an accessible dialog and sends NO request yet", async () => {
    const s = await renderActive();
    await openDialog();
    const d = dialog() as HTMLElement;
    expect(["alertdialog", "dialog"]).toContain(d.getAttribute("role") ?? "dialog");
    expect(d.getAttribute("aria-label") ?? d.getAttribute("aria-labelledby")).toBeTruthy();
    expect(deletes(s.calls)).toHaveLength(0);
  });

  it("dismiss button closes it: 0 DELETE, hold and countdown stay", async () => {
    const s = await renderActive();
    await openDialog();
    await act(async () => {
      fireEvent.click(dismissBtn());
    });
    await tick(50);
    expect(dialog()).toBeNull();
    expect(deletes(s.calls)).toHaveLength(0);
    expect(holdClient.getActiveHold("evt-d5")).not.toBeNull();
    expect(summaryState()).toBe("active");
  });

  it("Escape closes it: 0 DELETE, hold stays", async () => {
    const s = await renderActive();
    await openDialog();
    const d = dialog() as HTMLElement;
    await act(async () => {
      fireEvent.keyDown(d, { key: "Escape", code: "Escape" });
      if (d.tagName === "DIALOG") fireEvent(d, new Event("cancel", { cancelable: true }));
    });
    await tick(50);
    expect(dialog()).toBeNull();
    expect(deletes(s.calls)).toHaveLength(0);
    expect(summaryState()).toBe("active");
  });

  it("confirm -> exactly 1 DELETE /v1/holds/h1 with an Idempotency-Key and the bearer; hold removed; back to idle; dialog closed", async () => {
    const s = await renderActive();
    await openDialog();
    await act(async () => {
      fireEvent.click(confirmBtn());
    });
    await tick(50);
    const d = deletes(s.calls);
    expect(d).toHaveLength(1);
    expect(d[0].headers.get("Idempotency-Key")).toBeTruthy();
    expect(d[0].headers.get("Authorization")).toBe("Bearer acc");
    expect(holdClient.getActiveHold("evt-d5")).toBeNull();
    expect(summaryState()).toBe("idle");
    expect(screen.queryByRole("button", { name: "Giữ vé và thanh toán" })).not.toBeNull();
    expect(screen.queryByRole("button", { name: CANCEL })).toBeNull();
    expect(dialog()).toBeNull();
  });

  it("double click on confirm while the DELETE is pending -> still 1 DELETE; primary button disabled while cancelling", async () => {
    let release: (r: Response) => void = () => {};
    const s = await renderActive(() => new Promise<Response>((r) => (release = r)));
    await openDialog();
    const c = confirmBtn();
    await act(async () => {
      fireEvent.click(c);
      fireEvent.click(c);
    });
    await tick(20);
    expect(deletes(s.calls)).toHaveLength(1);
    const primary = screen.getByRole("button", { name: "Tiếp tục thanh toán" });
    expect((primary as HTMLButtonElement).disabled || primary.getAttribute("aria-disabled") === "true").toBe(true);
    await act(async () => {
      release(noContent());
    });
    await tick(50);
    expect(deletes(s.calls)).toHaveLength(1);
    expect(summaryState()).toBe("idle");
  });

  it("GUARD: without any cancel the primary button stays enabled", async () => {
    await renderActive();
    const primary = screen.getByRole("button", { name: "Tiếp tục thanh toán" }) as HTMLButtonElement;
    expect(primary.disabled).toBe(false);
  });
});

describe("AC-11 outcomes", () => {
  it.each([404, 200])("DELETE %i is treated as released", async (status) => {
    await renderActive(() => (status === 404 ? problem(404, "/problems/not-found") : json({})));
    await openDialog();
    await act(async () => {
      fireEvent.click(confirmBtn());
    });
    await tick(50);
    expect(summaryState()).toBe("idle");
    expect(holdClient.getActiveHold("evt-d5")).toBeNull();
  });

  it("AUTH_REQUIRED (401): existing sign-in alert, hold stays, no 'could not confirm' alert", async () => {
    await renderActive(() => problem(401, "/problems/unauthorized"));
    await openDialog();
    await act(async () => {
      fireEvent.click(confirmBtn());
    });
    await tick(50);
    expect(screen.queryByText(AUTH_TEXT)).not.toBeNull();
    expect(screen.getByRole("link", { name: "Đăng nhập" }).getAttribute("href")).toBe("/auth/sign-in");
    expect(holdClient.getActiveHold("evt-d5")).not.toBeNull();
    expect(summaryState()).toBe("active");
    expect(screen.queryByText(cancelHoldFailedText())).toBeNull();
  });

  it.each([
    ["HTTP 500", () => problem(500, "about:blank")],
    ["HTTP 403", () => problem(403, "/problems/forbidden")],
    ["network", () => Promise.reject(new TypeError("Failed to fetch")) as Promise<Response>],
  ] as Array<[string, () => Response | Promise<Response>]>)("%s: hold stays, error alert says release could not be confirmed, no sign-in prompt", async (_n, del) => {
    await renderActive(del);
    await openDialog();
    await act(async () => {
      fireEvent.click(confirmBtn());
    });
    await tick(50);
    expect(holdClient.getActiveHold("evt-d5")).not.toBeNull();
    expect(summaryState()).toBe("active");
    expect(screen.queryByText(cancelHoldFailedText())).not.toBeNull();
    expect(screen.queryByRole("link", { name: "Đăng nhập" })).toBeNull();
  });

  it("failure then a second attempt: user can confirm again (a new DELETE) and succeed", async () => {
    let n = 0;
    const s = await renderActive(() => (n++ === 0 ? problem(500, "about:blank") : noContent()));
    await openDialog();
    await act(async () => {
      fireEvent.click(confirmBtn());
    });
    await tick(50);
    await openDialog();
    await act(async () => {
      fireEvent.click(confirmBtn());
    });
    await tick(50);
    expect(deletes(s.calls).length).toBe(2);
    expect(summaryState()).toBe("idle");
  });
});

describe("AC-11 expiry", () => {
  it("hold expires while the dialog is open -> dialog closes, state expired, 0 DELETE (stepped per second)", async () => {
    const s = await renderActive(undefined, 4_000);
    await openDialog();
    for (let i = 0; i < 6; i++) await tick(1000);
    expect(dialog()).toBeNull();
    expect(summaryState()).toBe("expired");
    expect(deletes(s.calls)).toHaveLength(0);
  });

  it("hold expires while the DELETE is in flight -> ends expired, not idle", async () => {
    let release: (r: Response) => void = () => {};
    const s = await renderActive(() => new Promise<Response>((r) => (release = r)), 4_000);
    await openDialog();
    await act(async () => {
      fireEvent.click(confirmBtn());
    });
    for (let i = 0; i < 6; i++) await tick(1000);
    await act(async () => {
      release(noContent());
    });
    await tick(50);
    expect(deletes(s.calls)).toHaveLength(1);
    expect(summaryState()).toBe("expired");
  });
});
