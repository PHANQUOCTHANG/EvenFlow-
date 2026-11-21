/**
 * k6 load suite (EV-194 / EVF-124): opening spike, 30-minute waiting room, checkout wave.
 * Spec and plan: docs/workflow/k6-load-suite/{spec.md, plan.md}. How to run: tests/load/README.md.
 *
 *   k6 run -e PROFILE=smoke tests/load/main.js            # smoke | spike | wait | checkout | full
 *
 * Every request goes through the gateway with an HS256 access token minted here from JWT_SECRET (spec D1):
 * no /api/v1/auth/* call, no forged X-Identity-* header. Identities are pre-seeded by tests/load/seed-load.sh.
 * Load runs need ANTIBOT_MODE=shadow (or off) on the target (spec D5): one k6 host looks like an account farm.
 */
import { check, sleep } from "k6";
import crypto from "k6/crypto";
import exec from "k6/execution";
import http from "k6/http";

import { makeApi, view, json, header } from "./lib/api.js";
import { m, primeCounters } from "./lib/metrics.js";
import { buildOptions, resolveConfig } from "./lib/profiles.js";
import { identityFor, deviceFor, idemKey, nextPollDelayMs, shouldReplay, webhookDelayMs } from "./lib/flow.js";
import { accessClaims, encodeJwt, accessTokenExpS } from "./lib/jwt.js";
import { signalsFor } from "./lib/signals.js";
import { mulberry32, randInt } from "./lib/rng.js";
import { renderSummary, resultPath } from "./lib/summary.js";

// Init: throws (k6 exits non-zero before any request) on unknown PROFILE, malformed values or a remote BASE
// without ALLOW_REMOTE=1 (AC-2, AC-4).
const cfg = resolveConfig(__ENV);
export const options = buildOptions(__ENV);

// http_req_failed counts 5xx and network errors only (AC-3): 304, 409 sold out, 429 are outcomes, measured apart.
http.setResponseCallback(http.expectedStatuses({ min: 200, max: 499 }));

const api = makeApi(cfg.base, cfg.event);
// Captured at init (R-7): result files never fall back to a shared name, even if handleSummary gets no setup_data.
const INIT_RUN_ID = cfg.runId || `r${Date.now().toString(36)}`;
const SELLABLE = { AVAILABLE: true, FEW_LEFT: true }; // R-6
const POLL_JITTER_MS = 250;
const ISSUE_WAIT_S = 20;
const QUEUE_DEADLINE_S = 60; // smoke / checkout wave: give up waiting for ADMITTED after this
const TERMINAL = { ADMITTED: true, EXPIRED: true, SOLD_OUT: true };

export function setup() {
  if (cfg.remote) {
    console.warn(`
[load] ******** REMOTE TARGET ${cfg.base} (ALLOW_REMOTE=1, REMOTE_CONFIRM set) ********
`);
  }
  const h = api.healthz();
  if (h.status !== 200) throw new Error(`gateway ${cfg.base}/healthz returned ${h.status}`);
  // ticketing-worker loads Redis stock every 5 s; before that availability says SOLD_OUT on purpose.
  let ticketType = cfg.ticketType;
  let state = "";
  for (let i = 0; i < 30; i++) {
    const body = json(api.availability()) || {};
    const tiers = body.tiers || [];
    const tier = ticketType ? tiers.find((t) => t.ticket_type_id === ticketType) : tiers[0];
    state = tier ? tier.availability : "no tier";
    if (tier && SELLABLE[state]) {
      ticketType = tier.ticket_type_id;
      break;
    }
    sleep(1);
  }
  if (!SELLABLE[state]) {
    throw new Error(`event ${cfg.event} not sellable after 30 s (${state}): run tests/load/seed-load.sh first`);
  }
  primeCounters();
  const runId = INIT_RUN_ID;
  console.log(
    `[load] profile=${cfg.profile} run=${runId} event=${cfg.event} ticket_type=${ticketType} users=${cfg.users} ` +
      `quota=${cfg.quota} (must equal the seeded QUOTA) hold_ttl=${cfg.holdTtlS}s webhook_rate=${cfg.webhookRate}/s`,
  );
  return { runId, ticketType, startMs: Date.now() };
}

// ---------------------------------------------------------------------------------------------------------------
// per-identity state

function identity(range) {
  const i = exec.scenario.iterationInTest;
  if (i >= range.size) {
    m.identityExhausted.add(1);
    return null;
  }
  const n = range.start + i;
  return { n, id: identityFor(n), did: deviceFor(n), rng: mulberry32(Math.imul(n, 2654435761)) };
}

function mint(who) {
  const now = Math.floor(Date.now() / 1000);
  const claims = accessClaims({
    sub: who.id,
    did: who.did,
    jti: `lt-${who.n}-${now}`,
    iatS: now,
    expS: accessTokenExpS(now, cfg.runSeconds),
  });
  return encodeJwt(claims, (s) => crypto.hmac("sha256", cfg.jwtSecret, s, "base64rawurl"));
}

function join(who, token) {
  m.joinArrivals.add(1);
  const res = api.join(token, signalsFor(cfg.signals, { identityIdx: who.n, deviceId: who.did, rng: who.rng }));
  m.joinLatency.add(res.timings.duration);
  const ok = res.status === 200;
  m.joinSuccess.add(ok);
  const body = json(res) || {};
  check(res, { "join 200": () => ok, "join has queue_token or SOLD_OUT": () => !!body.queue_token || body.state === "SOLD_OUT" });
  if (!ok) return null;
  if (body.state === "SOLD_OUT") {
    m.soldOut.add(1);
    return null;
  }
  return { queueToken: body.queue_token, state: body.state, pollAfter: nextPollDelayMs(view(res), { rng: who.rng }) };
}

// Poll until a terminal state or deadlineMs. Honours poll_after_ms / X-Poll-After-Ms / Retry-After (AC-5).
function poll(who, token, q, deadlineMs) {
  let etag = null;
  let state = q.state;
  let delay = q.pollAfter;
  while (!TERMINAL[state]) {
    if (Date.now() + delay > deadlineMs) return state;
    sleep(delay / 1000);
    const res = api.status(token, q.queueToken, etag);
    m.statusLatency.add(res.timings.duration);
    m.status429.add(res.status === 429);
    if (res.status === 200 || res.status === 304) m.status304.add(res.status === 304);
    const e = header(res.headers, "ETag");
    if (e) etag = e;
    const v = view(res);
    if (res.status === 200 && v.body && v.body.state) state = v.body.state;
    check(res, { "status 200/304/429": (r) => r.status === 200 || r.status === 304 || r.status === 429 });
    delay = nextPollDelayMs(v, { jitterMaxMs: POLL_JITTER_MS, rng: who.rng });
  }
  if (state === "ADMITTED") m.admitted.add(1);
  else if (state === "EXPIRED") m.expired.add(1);
  else if (state === "SOLD_OUT") m.soldOut.add(1);
  return state;
}

// Webhook pacing (D4, R-1): in a paced wave (checkout scenario) the k-th buyer posts at start + k / WEBHOOK_RATE,
// and WEBHOOK_RATE >= CHECKOUT_RATE is enforced at init, so the delay stays near 0. Buyers of the wait/smoke flows
// are admitted by the server at its own rate: no slot, 429 is retried with Retry-After. Never sleep past the hold.
function webhookWait(who, range, data, holdStartMs, paced) {
  const nowMs = Date.now();
  const d = webhookDelayMs({
    k: paced ? who.n - range.start : 0,
    startMs: paced ? data.startMs : nowMs,
    nowMs,
    rate: cfg.webhookRate,
    holdStartMs,
    holdTtlS: cfg.holdTtlS,
  });
  if (d > 0) sleep(d / 1000);
  return d >= 0;
}

function postWebhook(q) {
  for (let attempt = 0; attempt < 5; attempt++) {
    const res = api.webhook(q);
    if (res.status !== 429) return res;
    sleep(nextPollDelayMs(view(res), { rng: Math.random }) / 1000);
  }
  return null;
}

// hold -> order -> payment -> sandbox webhook -> ISSUED, with idempotent replays (AC-6).
function checkout(who, token, q, data, range, paced) {
  const holdStartMs = Date.now();
  const holdKey = idemKey(data.runId, who.id, "hold");
  let res = api.hold(token, q.queueToken, holdKey, data.ticketType);
  m.holdLatency.add(res.timings.duration);
  if (res.status === 409) {
    m.checkoutSoldOut.add(1);
    return;
  }
  const hold = json(res) || {};
  if (!step(check(res, { "hold 200": (r) => r.status === 200 && !!hold.hold_id }))) return;
  m.holdsOk.add(1);
  const replay = shouldReplay(who.rng, cfg.replayRatio);
  if (replay) {
    const again = json(api.hold(token, q.queueToken, holdKey, data.ticketType)) || {};
    if (again.hold_id !== hold.hold_id) m.idemMismatch.add(1);
  }

  const order = json(api.getOrder(token, hold.order_id)) || {};
  const item = (order.items || [])[0];
  if (!step(check(order, { "order has an item": () => !!item }))) return;
  const amount = item.unit_cents * item.quantity;
  const payKey = idemKey(data.runId, who.id, "pay");
  const payBody = {
    order_id: hold.order_id,
    hold_id: hold.hold_id,
    amount,
    method: "sandbox",
    buyer_name: `Load ${who.n}`,
    buyer_email: `load${who.n}@eventflow.local`,
    return_url: "http://localhost:3000/orders",
  };
  res = api.pay(token, payKey, payBody);
  m.paymentLatency.add(res.timings.duration);
  const pay = json(res) || {};
  if (!step(check(res, { "payment 200": (r) => r.status === 200 && !!pay.payment_id }))) return;
  if (replay) {
    const again = json(api.pay(token, payKey, payBody)) || {};
    if (again.payment_id !== pay.payment_id) m.idemMismatch.add(1);
  }

  if (!webhookWait(who, range, data, holdStartMs, paced)) {
    m.webhookDelayed.add(1);
    step(false);
    return;
  }
  const wh = postWebhook({ txn: pay.provider_txn_id, orderId: hold.order_id, paymentId: pay.payment_id, amount });
  if (!step(check(wh, { "webhook 200": (r) => !!r && r.status === 200 }))) return;

  const t0 = Date.now();
  for (let i = 0; i < ISSUE_WAIT_S; i++) {
    const o = json(api.getOrder(token, hold.order_id)) || {};
    if (o.order_status === "ISSUED") {
      m.issued.add(1);
      m.issueWait.add(Date.now() - t0);
      step(true);
      return;
    }
    sleep(1);
  }
  m.issueTimeout.add(1);
  step(false);
}

// Every checkout step after a successful hold (order, payment, webhook, ISSUED) is one sample of checkout_step_ok (R-4).
function step(ok) {
  m.checkoutStepOk.add(!!ok);
  return ok;
}

function jitter(rng, maxMs) {
  if (maxMs > 0) sleep(randInt(rng, 0, maxMs) / 1000);
}

// ---------------------------------------------------------------------------------------------------------------
// scenarios

export function smokeFlow(data) {
  const who = identity(cfg.ranges.join);
  if (!who) return;
  jitter(who.rng, cfg.jitterMs);
  const token = mint(who);
  const q = join(who, token);
  if (!q) return;
  if (poll(who, token, q, data.startMs + QUEUE_DEADLINE_S * 1000) === "ADMITTED") {
    checkout(who, token, q, data, cfg.ranges.join, false);
  }
}

// Opening spike (AC-8): join only, client jitter 0..JITTER_MS (EVF-61).
export function joinBurst() {
  const who = identity(cfg.ranges.join);
  if (!who) return;
  jitter(who.rng, cfg.jitterMs);
  join(who, mint(who));
}

// 30-minute wait (AC-9): join once, poll until the scenario ends; ADMITTED hands over to checkout.
export function waitAndPoll(data) {
  const endMs = exec.scenario.startTime + cfg.waitDurationS * 1000;
  const who = identity(cfg.ranges.wait);
  if (who) {
    jitter(who.rng, Math.min(cfg.jitterMs, 5000));
    const token = mint(who);
    const q = join(who, token);
    if (q && poll(who, token, q, endMs) === "ADMITTED") checkout(who, token, q, data, cfg.ranges.wait, false);
  }
  // one identity per VU: idle until the scenario ends instead of taking a new identity
  const rest = endMs - Date.now();
  if (rest > 0) sleep(rest / 1000);
}

// Checkout wave: new identities join, wait for ADMITTED, buy.
export function checkoutWave(data) {
  const who = identity(cfg.ranges.checkout);
  if (!who) return;
  const token = mint(who);
  const q = join(who, token);
  if (!q) return;
  if (poll(who, token, q, Date.now() + QUEUE_DEADLINE_S * 1000) === "ADMITTED") {
    checkout(who, token, q, data, cfg.ranges.checkout, true);
  }
}

export default smokeFlow;

export function handleSummary(data) {
  const runId = (data.setup_data && data.setup_data.runId) || INIT_RUN_ID;
  const requestedJoinRate = cfg.profile === "spike" || cfg.profile === "full" ? Math.ceil(cfg.joinTarget / cfg.joinRampS) : 0;
  const text = renderSummary(data, { profile: cfg.profile, quota: cfg.quota, runId, requestedJoinRate });
  return {
    stdout: text,
    [resultPath(cfg.profile, runId)]: JSON.stringify({ profile: cfg.profile, runId, quota: cfg.quota, summary: text, data }, null, 2),
    [resultPath(cfg.profile, runId).replace(/\.json$/, ".txt")]: text,
  };
}
