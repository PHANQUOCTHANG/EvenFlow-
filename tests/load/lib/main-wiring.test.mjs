// main.js wiring under stubbed k6 modules (no k6 binary needed): every profile loads, init refuses bad input,
// a smoke iteration walks join -> status (304 + ETag) -> hold -> payment -> webhook -> ISSUED through the gateway
// with a minted Bearer token, idempotent replays, and zero /api/v1/auth/* calls (AC-1 flow, AC-4, AC-5, AC-6).
import test from "node:test";
import assert from "node:assert/strict";
import { register } from "node:module";
import { createHmac } from "node:crypto";
import { readFileSync } from "node:fs";

register("./fixtures/k6-stub-hooks.mjs", import.meta.url);
const MAIN = new URL("../main.js", import.meta.url).href;
const SECRET = "dev-jwt-secret-change-me-0123456789";
const ID1 = "00000000-0000-4000-9000-000000000001";

const res = (status, body, headers = {}) => ({
  status,
  body: body === null ? "" : JSON.stringify(body),
  headers,
  timings: { duration: 5 },
});

function fakeGateway() {
  const calls = [];
  let polls = 0;
  let orderReads = 0;
  const path = (u) => u.replace(/^http:\/\/localhost:8080/, "");
  const gw = {
    calls,
    overridePost: null,
    setResponseCallback() {},
    expectedStatuses(o) {
      return o;
    },
    get(url, p) {
      calls.push({ m: "GET", u: path(url), h: (p && p.headers) || {} });
      if (url.endsWith("/healthz")) return res(200, { status: "ok" });
      if (url.endsWith("/availability")) return res(200, { tiers: [{ ticket_type_id: "tt-1", availability: "AVAILABLE" }] });
      if (url.includes("/queue/status")) {
        polls++;
        if (polls === 1) return res(304, null, { Etag: "W/\"q-0\"", "X-Poll-After-Ms": "1500" });
        return res(200, { state: "ADMITTED", poll_after_ms: 1000 }, { Etag: "W/\"a-0\"" });
      }
      if (url.includes("/v1/orders/")) {
        orderReads++;
        return res(200, { order_status: orderReads >= 3 ? "ISSUED" : "PAID", items: [{ unit_cents: 50000000, quantity: 1 }] });
      }
      return res(404, {});
    },
    post(url, body, p) {
      calls.push({ m: "POST", u: path(url), h: (p && p.headers) || {}, b: body });
      if (gw.overridePost) {
        const o = gw.overridePost(url);
        if (o) return o;
      }
      if (url.includes("/queue/join")) return res(200, { state: "QUEUED", queue_token: "qt-1", poll_after_ms: 2000 });
      if (url.endsWith("/holds")) return res(200, { hold_id: "h-1", order_id: "o-1" });
      if (url.endsWith("/payments")) return res(200, { payment_id: "p-1", provider_txn_id: "t-1" });
      if (url.includes("/webhook/sandbox")) return res(200, {});
      return res(404, {});
    },
  };
  return gw;
}

let seq = 0;
async function loadMain(env) {
  globalThis.__ENV = env;
  for (const v of Object.values(globalThis.__m || {})) v.length = 0; // metric modules are cached: reset samples
  globalThis.__http = fakeGateway();
  globalThis.__exec = { scenario: { iterationInTest: 0, startTime: Date.now() }, vu: { idInTest: 1 } };
  return import(`${MAIN}?v=${++seq}`); // fresh module instance per env
}

const sum = (k) => globalThis.__m[k].reduce((a, b) => a + b, 0);

test("every profile loads and exports its scenario functions", async () => {
  const want = {
    smoke: ["smoke"],
    spike: ["joinBurst"],
    wait: ["waitAndPoll"],
    checkout: ["checkoutWave"],
    full: ["joinBurst", "waitAndPoll", "checkoutWave"],
  };
  for (const [PROFILE, names] of Object.entries(want)) {
    const mod = await loadMain({ PROFILE, JOIN_TARGET: "1000", WAIT_VUS: "10", CHECKOUT_RATE: "2" });
    assert.deepEqual(Object.keys(mod.options.scenarios), names, PROFILE);
    for (const s of Object.values(mod.options.scenarios)) assert.equal(typeof mod[s.exec], "function", s.exec);
    assert.equal(typeof mod.handleSummary, "function");
  }
});

test("init refuses an unknown profile, a malformed value and a remote BASE", async () => {
  await assert.rejects(loadMain({ PROFILE: "soak" }), /PROFILE/);
  await assert.rejects(loadMain({ PROFILE: "spike", JOIN_RAMP: "5" }), /JOIN_RAMP/);
  await assert.rejects(loadMain({ BASE: "http://staging.example.com" }), /ALLOW_REMOTE/);
});

test("a smoke iteration buys a ticket through the gateway with a minted token and no auth call", async () => {
  const mod = await loadMain({ PROFILE: "smoke", REPLAY_RATIO: "1", RUN_ID: "rwire" });
  const data = mod.setup();
  assert.equal(data.ticketType, "tt-1");
  mod.smokeFlow(data);
  const calls = globalThis.__http.calls;

  assert.equal(calls.filter((c) => c.u.includes("/api/v1/auth/")).length, 0, "no /api/v1/auth/* call");
  for (const c of calls) {
    for (const h of Object.keys(c.h)) assert.ok(!/^x-identity-/i.test(h), `no forged identity header in ${c.u}`);
  }
  const authed = calls.filter((c) => !/healthz|availability|webhook/.test(c.u));
  assert.ok(authed.length >= 8);
  const token = authed[0].h.Authorization.replace(/^Bearer /, "");
  for (const c of authed) assert.equal(c.h.Authorization, `Bearer ${token}`, c.u);
  const [h, p, s] = token.split(".");
  assert.equal(createHmac("sha256", SECRET).update(`${h}.${p}`).digest("base64url"), s);
  const claims = JSON.parse(Buffer.from(p, "base64url").toString());
  assert.equal(claims.sub, ID1);
  assert.equal(claims.did, "ltd-000000000001");
  assert.ok(claims.exp >= Math.floor(Date.now() / 1000) + 100 + 300, "exp >= run end + 5 min");

  const join = calls.find((c) => c.u.endsWith("/queue/join"));
  const signals = JSON.parse(join.b).signals;
  assert.equal(signals.v, 1);
  assert.equal(signals.fp, "ltd-000000000001");
  assert.ok(!/k6/i.test(join.h["User-Agent"]));

  const status = calls.filter((c) => c.u.includes("/queue/status"));
  assert.equal(status.length, 2);
  assert.equal(status[0].h["X-Queue-Token"], "qt-1");
  assert.equal(status[0].h["If-None-Match"], undefined);
  assert.equal(status[1].h["If-None-Match"], "W/\"q-0\"", "sends the last ETag");

  const holds = calls.filter((c) => c.u.endsWith("/holds"));
  const pays = calls.filter((c) => c.u.endsWith("/payments"));
  assert.equal(holds.length, 2, "hold + replay");
  assert.equal(pays.length, 2, "payment + replay");
  for (const c of holds) assert.equal(c.h["Idempotency-Key"], `lt-rwire-${ID1}-hold`);
  for (const c of pays) assert.equal(c.h["Idempotency-Key"], `lt-rwire-${ID1}-pay`);
  assert.equal(holds[0].b, holds[1].b);
  assert.equal(JSON.parse(pays[0].b).amount, 50000000);
  const wh = calls.find((c) => c.u.includes("/webhook/sandbox"));
  assert.match(wh.u, /order_id=o-1&payment_id=p-1&status=SUCCESS&amount=50000000/);

  assert.equal(sum("wr_admitted"), 1);
  assert.equal(sum("checkout_holds_ok"), 1);
  assert.equal(sum("checkout_issued"), 1);
  assert.equal(sum("idem_mismatch"), 0);
  assert.deepEqual(globalThis.__m.wr_status_304_ratio, [true, false]);
  assert.deepEqual(globalThis.__m.wr_join_success, [true]);
});

test("a replay that returns another hold_id counts idem_mismatch", async () => {
  const mod = await loadMain({ PROFILE: "smoke", REPLAY_RATIO: "1", RUN_ID: "rwire" });
  let n = 0;
  globalThis.__http.overridePost = (url) => (url.endsWith("/holds") ? res(200, { hold_id: `h-${++n}`, order_id: "o-1" }) : null);
  mod.smokeFlow(mod.setup());
  assert.equal(sum("idem_mismatch"), 1);
});

test("hold 409 is sold out, not a failure", async () => {
  const mod = await loadMain({ PROFILE: "smoke", REPLAY_RATIO: "0" });
  globalThis.__http.overridePost = (url) => (url.endsWith("/holds") ? res(409, {}) : null);
  mod.smokeFlow(mod.setup());
  assert.equal(sum("checkout_sold_out"), 1);
  assert.equal(sum("checkout_holds_ok"), 0);
  assert.equal(globalThis.__http.calls.filter((c) => c.u.endsWith("/payments")).length, 0);
});

test("handleSummary writes results/{profile}-{RUN_ID}.json and a text table", async () => {
  const mod = await loadMain({ PROFILE: "smoke" });
  const out = mod.handleSummary({ metrics: {}, setup_data: { runId: "rsum" } });
  assert.deepEqual(Object.keys(out).sort(), [
    "stdout",
    "tests/load/results/smoke-rsum.json",
    "tests/load/results/smoke-rsum.txt",
  ]);
  assert.match(out.stdout, /OVERALL\s+PASS/);
  assert.equal(JSON.parse(out["tests/load/results/smoke-rsum.json"]).runId, "rsum");
});

test("pure helpers do not import k6/*", () => {
  for (const f of ["profiles", "jwt", "signals", "flow", "summary", "rng"]) {
    const src = readFileSync(new URL(`./${f}.js`, import.meta.url), "utf8");
    assert.ok(!/from\s+["']k6/.test(src), `${f}.js imports k6`);
  }
});
