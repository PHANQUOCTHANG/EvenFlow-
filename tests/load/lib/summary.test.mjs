// AC-10 (oversell suspicion) and AC-11 (results path + SLO table).
import test from "node:test";
import assert from "node:assert/strict";
import { renderSummary, oversellSuspected, resultPath } from "./summary.js";

const data = (holds, joinP99, joinOk = true) => ({
  metrics: {
    wr_join_latency: { type: "trend", values: { "p(99)": joinP99 }, thresholds: { "p(99)<300": { ok: joinOk } } },
    checkout_holds_ok: { type: "counter", values: { count: holds }, thresholds: { "count<=100": { ok: holds <= 100 } } },
    idem_mismatch: { type: "counter", values: { count: 0 }, thresholds: { "count==0": { ok: true } } },
    dropped_iterations: { type: "counter", values: { count: 3 } },
    wr_join_arrivals: { type: "counter", values: { count: 997 } },
  },
  state: { testRunDurationMs: 10000 },
});

test("oversell suspected only when distinct successful holds exceed QUOTA", () => {
  assert.equal(oversellSuspected(data(100, 10), 100), 0);
  assert.equal(oversellSuspected(data(103, 10), 100), 3);
  assert.equal(oversellSuspected({ metrics: {} }, 100), 0);
});

test("text summary lists each SLO with target, actual and PASS/FAIL", () => {
  const t = renderSummary(data(100, 250), { profile: "smoke", quota: 100, runId: "r1" });
  assert.match(t, /wr_join_latency\s+p\(99\)<300\s+250(\.0+)?\s+PASS/);
  assert.match(t, /checkout_holds_ok\s+count<=100\s+100\s+PASS/);
  assert.match(t, /oversell_suspected\s+0/);
  assert.match(t, /dropped_iterations\s+3/);
  assert.match(t, /OVERALL\s+PASS/);
});

test("a failed threshold or an oversell suspicion makes the summary FAIL", () => {
  const f = renderSummary(data(100, 900, false), { profile: "spike", quota: 100, runId: "r1" });
  assert.match(f, /wr_join_latency\s+p\(99\)<300\s+900(\.0+)?\s+FAIL/);
  assert.match(f, /OVERALL\s+FAIL/);
  const o = renderSummary(data(101, 10), { profile: "spike", quota: 100, runId: "r1" });
  assert.match(o, /oversell_suspected\s+1/);
  assert.match(o, /OVERALL\s+FAIL/);
});

test("results go to tests/load/results/{profile}-{RUN_ID}.json", () => {
  assert.equal(resultPath("smoke", "r20261122a"), "tests/load/results/smoke-r20261122a.json");
  assert.throws(() => resultPath("smoke", "../x"), /RUN_ID/);
});
