// AC-2 (profiles + env overrides + init errors), AC-3 (thresholds), AC-8/AC-9 (shapes) / AC-12.
import test from "node:test";
import assert from "node:assert/strict";
import { PROFILES, buildOptions, resolveConfig, parseDurationS } from "./profiles.js";

const thr = (opts, metric) => {
  const v = opts.thresholds[metric];
  assert.ok(v, `threshold for ${metric}`);
  return v.map((t) => (typeof t === "string" ? t : t.threshold));
};

test("five profiles", () => {
  assert.deepEqual([...PROFILES], ["smoke", "spike", "wait", "checkout", "full"]);
});

test("parseDurationS accepts k6-style durations", () => {
  assert.equal(parseDurationS("5s", "X"), 5);
  assert.equal(parseDurationS("30m", "X"), 1800);
  assert.equal(parseDurationS("1h", "X"), 3600);
  assert.equal(parseDurationS("1m30s", "X"), 90);
});

test("parseDurationS rejects bad durations", () => {
  for (const bad of ["", "5", "-5s", "0s", "5x", "s", "1.5.0s", "5 s"]) {
    assert.throws(() => parseDurationS(bad, "WAIT_DURATION"), /WAIT_DURATION/, bad);
  }
});

test("default profile is smoke", () => {
  assert.equal(resolveConfig({}).profile, "smoke");
});

test("unknown profile throws at init", () => {
  assert.throws(() => buildOptions({ PROFILE: "soak" }), /PROFILE/);
});

test("malformed values throw at init", () => {
  const cases = [
    { JOIN_TARGET: "0" }, { JOIN_TARGET: "-3" }, { JOIN_TARGET: "abc" }, { JOIN_TARGET: "1.5" },
    { WAIT_VUS: "0" }, { CHECKOUT_RATE: "x" }, { USERS: "0" }, { QUOTA: "-1" },
    { JOIN_RAMP: "5" }, { WAIT_DURATION: "forever" }, { CHECKOUT_DURATION: "0s" },
    { SLO_SCALE: "0" }, { SLO_SCALE: "-2" }, { SLO_SCALE: "fast" },
    { REPLAY_RATIO: "1.5" }, { REPLAY_RATIO: "-0.1" }, { JITTER_MS: "-1" }, { WEBHOOK_RATE: "0" },
    { SIGNALS: "lots" },
  ];
  for (const env of cases) {
    const name = Object.keys(env)[0];
    for (const PROFILE of ["smoke", "full"]) {
      assert.throws(() => buildOptions({ PROFILE, ...env }), new RegExp(name), JSON.stringify(env));
    }
  }
});

test("smoke defaults: 20 VUs x 1 join, USERS=50, QUOTA=200, SLO_SCALE=4", () => {
  const cfg = resolveConfig({ PROFILE: "smoke" });
  assert.equal(cfg.users, 50);
  assert.equal(cfg.quota, 200);
  assert.equal(cfg.sloScale, 4);
  const o = buildOptions({ PROFILE: "smoke" });
  const s = o.scenarios.smoke;
  assert.equal(s.executor, "shared-iterations");
  assert.equal(s.vus, 20);
  assert.equal(s.iterations, 20);
  assert.equal(s.exec, "smokeFlow");
  assert.deepEqual(Object.keys(o.scenarios), ["smoke"]);
  assert.ok(cfg.runSeconds <= 120, "smoke must fit in < 2 minutes");
});

test("spike defaults reach JOIN_TARGET=100000 arrivals within JOIN_RAMP=5s", () => {
  const cfg = resolveConfig({ PROFILE: "spike" });
  assert.equal(cfg.joinTarget, 100000);
  assert.equal(cfg.joinRampS, 5);
  assert.equal(cfg.jitterMs, 5000);
  const s = buildOptions({ PROFILE: "spike" }).scenarios.joinBurst;
  assert.equal(s.executor, "ramping-arrival-rate");
  assert.equal(s.exec, "joinBurst");
  assert.equal(s.timeUnit, "1s");
  // arrivals of the first JOIN_RAMP seconds: startRate held for the first stage
  assert.equal(s.startRate, 20000);
  assert.deepEqual(s.stages[0], { target: 20000, duration: "5s" });
  assert.ok(s.startRate * 5 >= 100000);
  assert.ok(s.maxVUs >= s.preAllocatedVUs);
});

test("spike honours JOIN_TARGET, JOIN_RAMP, JITTER_MS overrides", () => {
  const cfg = resolveConfig({ PROFILE: "spike", JOIN_TARGET: "5000", JOIN_RAMP: "10s", JITTER_MS: "0" });
  assert.equal(cfg.jitterMs, 0);
  const s = buildOptions({ PROFILE: "spike", JOIN_TARGET: "5000", JOIN_RAMP: "10s" }).scenarios.joinBurst;
  assert.equal(s.startRate, 500);
  assert.deepEqual(s.stages[0], { target: 500, duration: "10s" });
});

test("wait defaults 20000 VUs for 30m and honours overrides", () => {
  const s = buildOptions({ PROFILE: "wait" }).scenarios.waitAndPoll;
  assert.equal(s.executor, "constant-vus");
  assert.equal(s.vus, 20000);
  assert.equal(s.duration, "1800s");
  assert.equal(s.exec, "waitAndPoll");
  const o = buildOptions({ PROFILE: "wait", WAIT_VUS: "300", WAIT_DURATION: "2m" }).scenarios.waitAndPoll;
  assert.equal(o.vus, 300);
  assert.equal(o.duration, "120s");
});

test("checkout defaults 200/s for 5m and honours overrides", () => {
  const s = buildOptions({ PROFILE: "checkout" }).scenarios.checkoutWave;
  assert.equal(s.executor, "constant-arrival-rate");
  assert.equal(s.rate, 200);
  assert.equal(s.timeUnit, "1s");
  assert.equal(s.duration, "300s");
  assert.equal(s.exec, "checkoutWave");
  const o = buildOptions({ PROFILE: "checkout", CHECKOUT_RATE: "7", CHECKOUT_DURATION: "30s" }).scenarios.checkoutWave;
  assert.equal(o.rate, 7);
  assert.equal(o.duration, "30s");
});

test("full runs spike at 0s, wait from 10s, checkout from 60s", () => {
  const sc = buildOptions({ PROFILE: "full" }).scenarios;
  assert.deepEqual(Object.keys(sc).sort(), ["checkoutWave", "joinBurst", "waitAndPoll"]);
  assert.equal(sc.joinBurst.startTime, "0s");
  assert.equal(sc.waitAndPoll.startTime, "10s");
  assert.equal(sc.checkoutWave.startTime, "60s");
});

test("identity ranges never overlap and fit USERS", () => {
  for (const PROFILE of PROFILES) {
    const cfg = resolveConfig({ PROFILE, JOIN_TARGET: "1000", WAIT_VUS: "50", CHECKOUT_RATE: "5", CHECKOUT_DURATION: "10s" });
    const rs = Object.values(cfg.ranges).filter((r) => r.size > 0).sort((a, b) => a.start - b.start);
    assert.ok(rs.length >= 1, PROFILE);
    assert.ok(rs[0].start >= 1);
    for (let i = 1; i < rs.length; i++) assert.ok(rs[i].start >= rs[i - 1].start + rs[i - 1].size, PROFILE);
    const last = rs[rs.length - 1];
    assert.ok(last.start + last.size - 1 <= cfg.users, `${PROFILE} needs ${last.start + last.size - 1} > USERS ${cfg.users}`);
  }
});

test("USERS smaller than the identities a profile needs throws", () => {
  assert.throws(() => buildOptions({ PROFILE: "spike", JOIN_TARGET: "1000", USERS: "10" }), /USERS/);
  assert.throws(() => buildOptions({ PROFILE: "smoke", JOIN_TARGET: "60", USERS: "50" }), /USERS/);
});

test("thresholds encode the SLOs at SLO_SCALE=1", () => {
  for (const PROFILE of ["spike", "wait", "checkout", "full"]) {
    const o = buildOptions({ PROFILE, QUOTA: "1234" });
    assert.deepEqual(thr(o, "wr_join_latency"), ["p(99)<300"]);
    assert.deepEqual(thr(o, "wr_status_latency"), ["p(99)<150"]);
    assert.deepEqual(thr(o, "co_hold_latency"), ["p(99)<800"]);
    assert.deepEqual(thr(o, "co_payment_latency"), ["p(99)<800"]);
    assert.deepEqual(thr(o, "http_req_duration{phase:join}"), ["p(99)<300"]);
    assert.deepEqual(thr(o, "http_req_duration{phase:status}"), ["p(99)<150"]);
    assert.deepEqual(thr(o, "http_req_duration{phase:hold}"), ["p(99)<800"]);
    assert.deepEqual(thr(o, "http_req_duration{phase:payment}"), ["p(99)<800"]);
    assert.deepEqual(thr(o, "wr_join_success"), ["rate>0.995"]);
    assert.deepEqual(thr(o, "http_req_failed"), ["rate<0.01"]);
    assert.deepEqual(thr(o, "wr_status_429"), ["rate<0.001"]);
    assert.deepEqual(thr(o, "checkout_holds_ok"), ["count<=1234"]);
    assert.deepEqual(thr(o, "idem_mismatch"), ["count==0"]);
  }
});

test("count invariants abort on fail; latency thresholds do not", () => {
  const o = buildOptions({ PROFILE: "full" });
  for (const m of ["checkout_holds_ok", "idem_mismatch"]) {
    assert.equal(o.thresholds[m][0].abortOnFail, true, m);
  }
  assert.equal(typeof o.thresholds.wr_join_latency[0], "string");
});

test("SLO_SCALE scales latency only, never the count invariants", () => {
  const o = buildOptions({ PROFILE: "smoke" }); // SLO_SCALE=4 by default
  assert.deepEqual(thr(o, "wr_join_latency"), ["p(99)<1200"]);
  assert.deepEqual(thr(o, "wr_status_latency"), ["p(99)<600"]);
  assert.deepEqual(thr(o, "co_hold_latency"), ["p(99)<3200"]);
  assert.deepEqual(thr(o, "checkout_holds_ok"), ["count<=200"]);
  assert.deepEqual(thr(o, "idem_mismatch"), ["count==0"]);
  assert.deepEqual(thr(o, "wr_join_success"), ["rate>0.995"]);
  assert.deepEqual(thr(o, "http_req_failed"), ["rate<0.01"]);
  const s2 = buildOptions({ PROFILE: "spike", SLO_SCALE: "2", QUOTA: "10" });
  assert.deepEqual(thr(s2, "wr_join_latency"), ["p(99)<600"]);
  assert.deepEqual(thr(s2, "checkout_holds_ok"), ["count<=10"]);
});

test("smoke requires the end-to-end evidence of AC-1", () => {
  const o = buildOptions({ PROFILE: "smoke" });
  assert.deepEqual(thr(o, "wr_admitted"), ["count>=1"]);
  assert.deepEqual(thr(o, "checkout_issued"), ["count>=1"]);
  assert.deepEqual(thr(o, "wr_status_304_ratio"), ["rate>0"]);
  assert.equal(buildOptions({ PROFILE: "spike" }).thresholds.checkout_issued, undefined);
});

test("summary exposes p(99) and 304/409 are not failures", () => {
  const o = buildOptions({ PROFILE: "smoke" });
  assert.ok(o.summaryTrendStats.includes("p(99)"));
  assert.deepEqual(resolveConfig({}).expectedStatuses, [200, 201, 204, 304, 409]);
});
