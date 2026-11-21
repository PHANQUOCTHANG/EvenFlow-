// Thin k6 HTTP wrappers. Everything goes through the gateway with a Bearer access token (spec scope 2).
// k6-only module: imports k6/http, never imported by node tests.
import http from "k6/http";
import { header } from "./flow.js";

// A desktop browser string: k6's default UA is not what a buyer sends (anti-bot reads the UA, contracts.md section 1).
export const USER_AGENT =
  "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36";

export function json(res) {
  if (!res || !res.body || res.status === 304) return null;
  try {
    return JSON.parse(res.body);
  } catch (_) {
    return null;
  }
}

// Plain object view used by the pure rules in flow.js.
export function view(res) {
  return { status: res.status, headers: res.headers, body: json(res) };
}

function headers(token, extra) {
  return Object.assign(
    { Authorization: `Bearer ${token}`, "User-Agent": USER_AGENT, Accept: "application/json" },
    extra || {},
  );
}

export function makeApi(base, event) {
  const ev = `${base}/v1/events/${event}`;
  return {
    healthz: () => http.get(`${base}/healthz`, { tags: { phase: "setup" } }),
    availability: () => http.get(`${ev}/availability`, { tags: { phase: "setup" } }),

    join: (token, signals) =>
      http.post(`${ev}/queue/join`, JSON.stringify({ signals }), {
        headers: headers(token, { "Content-Type": "application/json" }),
        tags: { phase: "join" },
      }),

    status: (token, queueToken, etag) =>
      http.get(`${ev}/queue/status`, {
        headers: headers(token, etag ? { "X-Queue-Token": queueToken, "If-None-Match": etag } : { "X-Queue-Token": queueToken }),
        tags: { phase: "status" },
      }),

    hold: (token, queueToken, idemKey, ticketTypeId) =>
      http.post(`${ev}/holds`, JSON.stringify({ ticket_type_id: ticketTypeId, quantity: 1 }), {
        headers: headers(token, {
          "Content-Type": "application/json",
          "X-Queue-Token": queueToken,
          "Idempotency-Key": idemKey,
        }),
        tags: { phase: "hold" },
      }),

    getOrder: (token, orderId) =>
      http.get(`${base}/v1/orders/${orderId}`, { headers: headers(token), tags: { phase: "order" } }),

    pay: (token, idemKey, body) =>
      http.post(`${ev}/payments`, JSON.stringify(body), {
        headers: headers(token, { "Content-Type": "application/json", "Idempotency-Key": idemKey }),
        tags: { phase: "payment" },
      }),

    // Public route, rate limited per IP: paced by the caller (spec D4).
    webhook: (q) =>
      http.post(
        `${base}/v1/payments/webhook/sandbox?provider_txn_id=${encodeURIComponent(q.txn)}&order_id=${q.orderId}` +
          `&payment_id=${q.paymentId}&status=SUCCESS&amount=${q.amount}`,
        null,
        { headers: { "User-Agent": USER_AGENT }, tags: { phase: "webhook" } },
      ),
  };
}

export { header };
