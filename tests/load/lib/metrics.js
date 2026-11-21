// k6 custom metrics (plan.md section 4). k6-only module: imports k6/metrics, never imported by node tests.
import { Counter, Rate, Trend } from "k6/metrics";

export const m = {
  joinLatency: new Trend("wr_join_latency", true),
  statusLatency: new Trend("wr_status_latency", true),
  holdLatency: new Trend("co_hold_latency", true),
  paymentLatency: new Trend("co_payment_latency", true),
  issueWait: new Trend("co_issue_wait", true),

  joinSuccess: new Rate("wr_join_success"),
  status304: new Rate("wr_status_304_ratio"),
  status429: new Rate("wr_status_429"),

  joinArrivals: new Counter("wr_join_arrivals"),
  admitted: new Counter("wr_admitted"),
  expired: new Counter("wr_expired"),
  soldOut: new Counter("wr_sold_out"),
  holdsOk: new Counter("checkout_holds_ok"),
  checkoutSoldOut: new Counter("checkout_sold_out"),
  issued: new Counter("checkout_issued"),
  issueTimeout: new Counter("checkout_issue_timeout"),
  idemMismatch: new Counter("idem_mismatch"),
  identityExhausted: new Counter("identity_exhausted"),
};

// A counter threshold is only evaluated once the metric has a sample: add 0 in setup() so that
// "count>=1" fails (instead of being skipped) when nothing happened.
export function primeCounters() {
  const counters = ["joinArrivals", "admitted", "expired", "soldOut", "holdsOk", "checkoutSoldOut", "issued",
    "issueTimeout", "idemMismatch", "identityExhausted"];
  for (const k of counters) m[k].add(0);
}
