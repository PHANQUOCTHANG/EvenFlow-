/**
 * Contract test: lib/payment-client.ts (AC-3, AC-12, AC-13, AC-14, AC-15).
 *
 * Gia dinh ten truong (contracts.md chi dong bang wire, khong dong bang kieu TS):
 *  - OrderDetails them: expiresAt, holdId, failureReason, items: [{ticketTypeId, tierName, quantity, unitCents}].
 *  - Ma loi doc tu `error.type ?? error.code`; 404 don hang -> "NOT_FOUND" (thay cho ORDER_NOT_FOUND cu).
 *  - issueTickets doc wire `{order_id, tickets:[{id, ticket_type_id, status, qr_payload, ...}]}`
 *    (services/ticketing/internal/port/ticket.go IssueTicketsResult, domain.TicketWithQR).
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  getOrderDetails,
  initiatePayment,
  issueTickets,
  listPaymentMethods,
  PaymentClientError,
  saveRecentOrder,
} from "@/lib/payment-client";
import {
  BASE,
  codeOf,
  expectNoIdentityOrCredentials,
  installFetch,
  json,
  problem,
  rejection,
  resetEnv,
  seedSession,
  setupEnv,
} from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession("acc-1", "ref-1");
});
afterEach(resetEnv);

const PAY = {
  eventId: "evt-1",
  orderId: "ord-1",
  holdId: "hld-1",
  amount: 500000,
  method: "vnpay" as const,
  buyerName: "Nguyen Van A",
  buyerEmail: "a@example.com",
  buyerPhone: "0901234567",
  idempotencyKey: "idem-pay-1",
  returnUrl: "https://web.test/checkout/evt-1/result?orderId=ord-1",
};
const PAY_OK = {
  payment_id: "pay-1",
  order_id: "ord-1",
  redirect_url: "https://pay.test/redirect",
  status: "PENDING",
  provider_txn_id: "txn-1",
};

describe("initiatePayment: request (AC-3, AC-12)", () => {
  it("POST {base}/v1/events/{id}/payments dung header va body theo contract", async () => {
    const { calls } = installFetch(() => json(PAY_OK));
    const r = await initiatePayment(PAY);
    const c = calls[0];
    expect(c.url).toBe(`${BASE}/v1/events/evt-1/payments`);
    expect(c.method).toBe("POST");
    expect(c.headers.get("Idempotency-Key")).toBe("idem-pay-1");
    expect(c.headers.get("Authorization")).toBe("Bearer acc-1");
    expect(c.body).toEqual({
      order_id: "ord-1",
      hold_id: "hld-1",
      amount: 500000,
      method: "vnpay",
      buyer_name: "Nguyen Van A",
      buyer_email: "a@example.com",
      buyer_phone: "0901234567",
      return_url: "https://web.test/checkout/evt-1/result?orderId=ord-1",
    });
    expectNoIdentityOrCredentials(c);
    expect(r).toMatchObject({ paymentId: "pay-1", orderId: "ord-1", redirectUrl: "https://pay.test/redirect" });
  });

  it.each(["sandbox", "momo", "stripe"] as const)("chap nhan method %s", async (method) => {
    const { calls } = installFetch(() => json(PAY_OK));
    await initiatePayment({ ...PAY, method } as never);
    expect((calls[0].body as { method: string }).method).toBe(method);
  });
});

describe("initiatePayment: khong bao gio gia thanh cong (AC-12)", () => {
  it.each(["sandbox", "vnpay"] as const)("loi mang voi method %s -> PaymentClientError NETWORK_ERROR, khong tra paymentId", async (method) => {
    installFetch(() => {
      throw new TypeError("Failed to fetch");
    });
    const err = await rejection(initiatePayment({ ...PAY, method }));
    expect(err).toBeInstanceOf(PaymentClientError);
    expect(codeOf(err)).toBe("NETWORK_ERROR");
  });

  it("401 voi sandbox -> reject PaymentClientError (khong mock_pay_*)", async () => {
    installFetch(() => problem(401, "/problems/unauthorized"));
    const err = await rejection(initiatePayment({ ...PAY, method: "sandbox" }));
    expect(err).toBeInstanceOf(PaymentClientError);
    expect((err as PaymentClientError).status).toBe(401);
  });

  it.each([500, 502, 503])("HTTP %i -> reject PaymentClientError", async (status) => {
    installFetch(() => problem(status, "about:blank"));
    expect(await rejection(initiatePayment(PAY))).toBeInstanceOf(PaymentClientError);
  });

  it("2xx nhung thieu payment_id -> reject, KHONG tu sinh payment_id", async () => {
    installFetch(() => json({ order_id: "ord-1", redirect_url: "https://x", status: "PENDING" }));
    expect(await rejection(initiatePayment(PAY))).toBeInstanceOf(PaymentClientError);
  });

  it("apiBase() rong trong moi truong dev cung khong gia thanh cong", async () => {
    installFetch(() => {
      throw new TypeError("Failed to fetch");
    });
    const err = await rejection(initiatePayment({ ...PAY, method: "sandbox", apiBaseUrl: "" }));
    expect(err).toBeInstanceOf(PaymentClientError);
  });

  it("409 -> HOLD_EXPIRED, 422 -> INVALID_INPUT (giu hanh vi cu)", async () => {
    installFetch(() => problem(409, "about:blank"));
    expect(codeOf(await rejection(initiatePayment(PAY)))).toBe("HOLD_EXPIRED");
    installFetch(() => problem(422, "about:blank", "so tien khong khop"));
    const err = await rejection(initiatePayment(PAY));
    expect(codeOf(err)).toBe("INVALID_INPUT");
    expect((err as Error).message).toContain("so tien khong khop");
  });
});

const ORDER_WIRE = {
  order_id: "ord-1",
  event_id: "evt-1",
  event_title: "Rock Fest",
  venue: "Mỹ Đình",
  tier_name: "VIP",
  quantity: 2,
  total_amount: 1000000,
  total_cents: 1000000,
  currency: "VND",
  status: "PAID",
  order_status: "PAID",
  expires_at: "2026-11-01T10:10:00Z",
  created_at: "2026-11-01T10:00:00Z",
  items: [{ ticket_type_id: "tier-1", tier_name: "VIP", quantity: 2, unit_cents: 500000 }],
  issued_tickets: [],
  hold_id: "hld-1",
  paid_at: "2026-11-01T10:05:00Z",
  payment_method: "vnpay",
};

describe("getOrderDetails (AC-14)", () => {
  it("GET {base}/v1/orders/{id} voi Bearer", async () => {
    const { calls } = installFetch(() => json(ORDER_WIRE));
    await getOrderDetails("ord-1", "evt-1");
    expect(calls[0].url).toBe(`${BASE}/v1/orders/ord-1`);
    expect(calls[0].method).toBe("GET");
    expect(calls[0].headers.get("Authorization")).toBe("Bearer acc-1");
    expectNoIdentityOrCredentials(calls[0]);
  });

  it("anh xa day du truong that: items, expiresAt, holdId, status, tong tien", async () => {
    installFetch(() => json(ORDER_WIRE));
    const o = await getOrderDetails("ord-1", "evt-1");
    expect(o).toMatchObject({
      orderId: "ord-1",
      eventId: "evt-1",
      eventTitle: "Rock Fest",
      venue: "Mỹ Đình",
      tierName: "VIP",
      quantity: 2,
      totalAmount: 1000000,
      status: "PAID",
      expiresAt: "2026-11-01T10:10:00Z",
      holdId: "hld-1",
      paidAt: "2026-11-01T10:05:00Z",
    });
    expect((o as unknown as { items: unknown[] }).items).toEqual([
      { ticketTypeId: "tier-1", tierName: "VIP", quantity: 2, unitCents: 500000 },
    ]);
  });

  it("anh xa failure_reason va cac trang thai EXPIRED/FAILED/CANCELLED/PENDING", async () => {
    for (const status of ["PENDING", "FAILED", "EXPIRED", "CANCELLED"]) {
      installFetch(() => json({ ...ORDER_WIRE, status, failure_reason: "card_declined" }));
      const o = await getOrderDetails("ord-1", "evt-1");
      expect(o.status).toBe(status);
      expect(o.failureReason).toBe("card_declined");
    }
  });

  it("KHONG bia quantity / tier_name / event_title khi server khong tra", async () => {
    installFetch(() => json({ order_id: "ord-1", status: "PENDING" }));
    const o = await getOrderDetails("ord-1", "evt-1");
    expect(o.quantity).not.toBe(1);
    expect(o.tierName).not.toBe("Vé Tiêu Chuẩn");
    expect(o.eventTitle).not.toBe("Sự kiện EvenFlow");
    expect(o.venue).not.toBe("Địa điểm tổ chức");
  });

  it("404 -> reject NOT_FOUND, KHONG phuc vu cache sessionStorage nhu du lieu song", async () => {
    saveRecentOrder({
      orderId: "ord-1",
      eventId: "evt-1",
      eventTitle: "Cached",
      venue: "v",
      tierName: "t",
      quantity: 1,
      totalAmount: 1,
      status: "PAID",
    });
    installFetch(() => problem(404, "/problems/not-found"));
    const err = await rejection(getOrderDetails("ord-1", "evt-1"));
    expect(err).toBeInstanceOf(PaymentClientError);
    expect(codeOf(err)).toBe("NOT_FOUND");
  });

  it("loi mang -> reject NETWORK_ERROR, khong tra cache", async () => {
    saveRecentOrder({
      orderId: "ord-1",
      eventId: "evt-1",
      eventTitle: "Cached",
      venue: "v",
      tierName: "t",
      quantity: 1,
      totalAmount: 1,
      status: "PAID",
    });
    installFetch(() => {
      throw new TypeError("Failed to fetch");
    });
    const err = await rejection(getOrderDetails("ord-1", "evt-1"));
    expect(err).toBeInstanceOf(PaymentClientError);
    expect(codeOf(err)).toBe("NETWORK_ERROR");
  });

  it("401 token-expired -> refresh roi retry GET don hang", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        return json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2" });
      }
      return c.headers.get("Authorization") === "Bearer new" ? json(ORDER_WIRE) : problem(401, "/problems/token-expired");
    });
    const o = await getOrderDetails("ord-1", "evt-1");
    expect(o.status).toBe("PAID");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
  });
});

describe("listPaymentMethods (AC-13)", () => {
  const METHODS = {
    methods: [
      { code: "sandbox", name: "Sandbox", description: "Test", enabled: true, icon: "flask" },
      { code: "stripe", name: "Stripe", description: "Card", enabled: true, icon: "card" },
      { code: "momo", name: "MoMo", description: "Vi", enabled: false, icon: "momo" },
    ],
  };

  it("GET {base}/v1/payments/methods, tra mang PaymentMethodInfo", async () => {
    const { calls } = installFetch(() => json(METHODS));
    const list = await listPaymentMethods();
    expect(calls[0].url).toBe(`${BASE}/v1/payments/methods`);
    expect(calls[0].method).toBe("GET");
    expect(Array.isArray(list)).toBe(true);
    expect(list.find((m) => m.code === ("stripe" as never))).toMatchObject({
      code: "stripe",
      name: "Stripe",
      enabled: true,
      icon: "card",
    });
    // Method bi tat (neu con trong danh sach) phai mang enabled=false de UI loc.
    expect(list.filter((m) => m.code === ("momo" as never)).every((m) => m.enabled === false)).toBe(true);
  });

  it("loi server -> reject (UI hien loi + thu lai), KHONG tra danh sach cung", async () => {
    installFetch(() => problem(500, "about:blank"));
    const err = await rejection(listPaymentMethods());
    expect(err).toBeInstanceOf(PaymentClientError);
  });

  it("loi mang -> reject", async () => {
    installFetch(() => {
      throw new TypeError("Failed to fetch");
    });
    expect(await rejection(listPaymentMethods())).toBeInstanceOf(PaymentClientError);
  });
});

describe("issueTickets (AC-15)", () => {
  const ISSUE_WIRE = {
    order_id: "ord-1",
    event_id: "evt-1",
    already_issued: false,
    tickets: [
      { id: "t-1", order_id: "ord-1", ticket_type_id: "tier-1", status: "ISSUED", qr_payload: "QR.AAA", jti: "j1" },
      { id: "t-2", order_id: "ord-1", ticket_type_id: "tier-1", status: "ISSUED", qr_payload: "QR.BBB", jti: "j2" },
    ],
  };

  it("POST {base}/v1/orders/{id}/issue-tickets voi Idempotency-Key va Bearer", async () => {
    const { calls } = installFetch(() => json(ISSUE_WIRE));
    await issueTickets("ord-1");
    expect(calls[0].url).toBe(`${BASE}/v1/orders/ord-1/issue-tickets`);
    expect(calls[0].method).toBe("POST");
    expect(calls[0].headers.get("Idempotency-Key")).toMatch(/^[0-9a-f-]{36}$/i);
    expect(calls[0].headers.get("Authorization")).toBe("Bearer acc-1");
    expectNoIdentityOrCredentials(calls[0]);
  });

  it("tra IssuedTicketQR[] {ticketId, ticketTypeId, status, qrPayload} tu qr_payload that", async () => {
    installFetch(() => json(ISSUE_WIRE));
    const tickets = await issueTickets("ord-1");
    expect(tickets).toEqual([
      { ticketId: "t-1", ticketTypeId: "tier-1", status: "ISSUED", qrPayload: "QR.AAA" },
      { ticketId: "t-2", ticketTypeId: "tier-1", status: "ISSUED", qrPayload: "QR.BBB" },
    ]);
  });

  it("luu key o sessionStorage `ef:issue:{orderId}` va dung lai CUNG key khi goi lai (reload)", async () => {
    const { calls } = installFetch(() => json(ISSUE_WIRE));
    await issueTickets("ord-1");
    const stored = sessionStorage.getItem("ef:issue:ord-1");
    expect(stored).toBeTruthy();
    expect(calls[0].headers.get("Idempotency-Key")).toBe(stored);
    await issueTickets("ord-1");
    expect(calls[1].headers.get("Idempotency-Key")).toBe(stored);
  });

  it("don khac -> key khac", async () => {
    const { calls } = installFetch(() => json(ISSUE_WIRE));
    await issueTickets("ord-1");
    await issueTickets("ord-2");
    expect(calls[0].headers.get("Idempotency-Key")).not.toBe(calls[1].headers.get("Idempotency-Key"));
    expect(sessionStorage.getItem("ef:issue:ord-2")).toBeTruthy();
  });

  it("422 order-not-paid -> reject (khong tra ve QR gia)", async () => {
    installFetch(() => problem(422, "/problems/order-not-paid"));
    const err = await rejection(issueTickets("ord-1"));
    expect(err).toBeInstanceOf(Error);
  });

  it("khong bao gio tra QR demo khi server khong tra qr_payload", async () => {
    installFetch(() => json({ ...ISSUE_WIRE, tickets: [] }));
    const tickets = await issueTickets("ord-1");
    expect(tickets).toEqual([]);
  });
});
