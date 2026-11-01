/** AC-10 (S-4 client part): initiatePayment keeps redirect_url only if it is relative or absolute http(s); anything else
 *  (javascript:, data:, vbscript:, file:, unparsable) -> the default result URL (or the caller's returnUrl). */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { initiatePayment } from "@/lib/payment-client";
import { installFetch, json, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession("acc-1", "ref-1");
});
afterEach(resetEnv);

const PAY = {
  eventId: "evt-1",
  orderId: "ord 1",
  holdId: "hld-1",
  amount: 500000,
  method: "vnpay" as const,
  buyerName: "Nguyen Van A",
  buyerEmail: "a@example.com",
  buyerPhone: "0901234567",
  idempotencyKey: "idem-1",
};
const okWith = (redirect_url: unknown) =>
  json({ payment_id: "pay-1", order_id: "ord 1", redirect_url, status: "PENDING" });
const DEFAULT_URL = "/checkout/evt-1/result?orderId=ord%201";

describe("AC-10 redirect_url scheme check", () => {
  it.each([
    "javascript:alert(document.domain)",
    "JaVaScRiPt:alert(1)",
    " javascript:alert(1)",
    "\tjava\nscript:alert(1)",
    "data:text/html,<script>alert(1)</script>",
    "vbscript:msgbox(1)",
    "file:///etc/passwd",
    "blob:https://evil.example/abc",
    "http://[::1",
    "https://",
  ])("rejected %j -> default result URL", async (bad) => {
    installFetch(() => okWith(bad));
    const r = await initiatePayment(PAY);
    expect(r.redirectUrl).toBe(DEFAULT_URL);
  });

  it("rejected value -> caller's returnUrl when one was given", async () => {
    installFetch(() => okWith("javascript:alert(1)"));
    const r = await initiatePayment({ ...PAY, returnUrl: "https://web.test/checkout/evt-1/result?orderId=ord-1" });
    expect(r.redirectUrl).toBe("https://web.test/checkout/evt-1/result?orderId=ord-1");
  });

  it.each([
    "https://pay.test/redirect",
    "http://pay.test/redirect?a=1&b=2",
    "HTTPS://PAY.TEST/x",
    "/checkout/evt-1/result?orderId=ord-1",
    "result?orderId=1",
  ])("GUARD: %s kept unchanged", async (ok) => {
    installFetch(() => okWith(ok));
    expect((await initiatePayment(PAY)).redirectUrl).toBe(ok);
  });

  it.each([undefined, null, "", 5, {}])("GUARD: missing/non-string redirect_url %j -> default", async (v) => {
    installFetch(() => okWith(v));
    expect((await initiatePayment(PAY)).redirectUrl).toBe(DEFAULT_URL);
  });
});
