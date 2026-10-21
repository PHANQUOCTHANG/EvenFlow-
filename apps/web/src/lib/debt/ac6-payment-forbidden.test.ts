/** AC-6 (W-5): 403 /problems/forbidden in initiatePayment / getOrderDetails / issueTickets -> PaymentClientError FORBIDDEN
 *  (type and code, status 403), exactly one HTTP call (no refresh, no retry). Other 403s keep their current mapping (UNKNOWN). */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { getOrderDetails, initiatePayment, issueTickets } from "@/lib/payment-client";
import { installFetch, json, problem, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession("acc", "r1");
});
afterEach(resetEnv);

const pay = () =>
  initiatePayment({
    eventId: "e1",
    orderId: "o1",
    holdId: "h1",
    method: "momo",
    buyerName: "a",
    buyerEmail: "a@b.co",
    buyerPhone: "0900000000",
    amount: 100,
  });

const ops: Array<[string, () => Promise<unknown>]> = [
  ["initiatePayment", pay],
  ["getOrderDetails", () => getOrderDetails("o1", "e1")],
  ["issueTickets", () => issueTickets("o1")],
];

const fail = (p: Promise<unknown>) =>
  p.then(
    () => null,
    (e: unknown) => e as { type: string; code: string; status?: number; message: string },
  );

describe("AC-6 403 /problems/forbidden", () => {
  it.each(ops)("%s -> FORBIDDEN (type + code + status 403), one call only", async (_n, op) => {
    const { calls } = installFetch(() => problem(403, "/problems/forbidden", "role mismatch"));
    const e = await fail(op());
    expect(e?.type).toBe("FORBIDDEN");
    expect(e?.code).toBe("FORBIDDEN");
    expect(e?.status).toBe(403);
    expect(calls).toHaveLength(1);
  });

  it.each(ops)("GUARD: %s with 403 about:blank keeps the generic mapping (UNKNOWN)", async (_n, op) => {
    installFetch(() => problem(403, "about:blank"));
    expect((await fail(op()))?.type).toBe("UNKNOWN");
  });

  it.each(ops)("GUARD: %s with 403 and an empty body keeps the generic mapping (UNKNOWN)", async (_n, op) => {
    installFetch(() => new Response(null, { status: 403 }));
    expect((await fail(op()))?.type).toBe("UNKNOWN");
  });

  it("GUARD: getOrderDetails 404 is still NOT_FOUND and initiatePayment 422 still INVALID_INPUT", async () => {
    installFetch(() => problem(404, "/problems/not-found"));
    expect((await fail(getOrderDetails("o1", "e1")))?.type).toBe("NOT_FOUND");
    installFetch(() => json({ type: "/problems/validation", title: "t", status: 422, detail: "bad" }, 422));
    expect((await fail(pay()))?.type).toBe("INVALID_INPUT");
  });
});
