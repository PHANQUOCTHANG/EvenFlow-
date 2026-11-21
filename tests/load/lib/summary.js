// End-of-run summary (AC-8 arrivals, AC-10 oversell suspicion, AC-11 results). Pure: works on k6's handleSummary data.

export function resultPath(profile, runId) {
  if (!/^[A-Za-z0-9_]{1,64}$/.test(String(runId))) throw new Error(`RUN_ID must match [A-Za-z0-9_]{1,64}: ${runId}`);
  if (!/^[a-z]+$/.test(String(profile))) throw new Error(`bad profile: ${profile}`);
  return `tests/load/results/${profile}-${runId}.json`;
}

const metric = (data, name) => (data && data.metrics && data.metrics[name]) || null;
const count = (data, name) => {
  const m = metric(data, name);
  return m && m.values && typeof m.values.count === "number" ? m.values.count : 0;
};

// k6 sees HTTP only: this is a suspicion, the authoritative check is `seed-load.sh verify` (D7).
export function oversellSuspected(data, quota) {
  return Math.max(0, count(data, "checkout_holds_ok") - quota);
}

function actualFor(m, expr) {
  const v = m.values || {};
  const agg = /^\s*([a-z]+(?:\([0-9.]+\))?)/.exec(expr);
  const key = agg ? agg[1] : "";
  const x = v[key];
  if (typeof x !== "number") return "n/a";
  return Number.isInteger(x) ? String(x) : x.toFixed(key === "rate" ? 4 : 2);
}

const row = (a, b, c, d) => `${String(a).padEnd(40)} ${String(b).padEnd(18)} ${String(c).padEnd(14)} ${d}`;

export function sloRows(data) {
  const rows = [];
  for (const name of Object.keys((data && data.metrics) || {}).sort()) {
    const m = data.metrics[name];
    for (const expr of Object.keys(m.thresholds || {})) {
      rows.push({ name, target: expr, actual: actualFor(m, expr), ok: m.thresholds[expr].ok !== false });
    }
  }
  return rows;
}

export function renderSummary(data, { profile, quota, runId, requestedJoinRate = 0 }) {
  const rows = sloRows(data);
  const oversell = oversellSuspected(data, quota);
  const durS = data && data.state && data.state.testRunDurationMs ? data.state.testRunDurationMs / 1000 : 0;
  const arrivals = count(data, "wr_join_arrivals");
  const lines = [
    `k6 load suite  profile=${profile}  run=${runId}  quota=${quota}`,
    row("SLO / invariant", "target", "actual", "result"),
    ...rows.map((r) => row(r.name, r.target, r.actual, r.ok ? "PASS" : "FAIL")),
    "",
    row("counter", "value", "note", "result"),
    row("oversell_suspected", oversell, `must be 0 (QUOTA=${quota})`, oversell === 0 ? "PASS" : "FAIL"),
    row("wr_join_arrivals", arrivals, "join attempts", ""),
    row("join arrival rate (/s)", durS > 0 ? (arrivals / durS).toFixed(1) : "n/a", "achieved, whole run", ""),
    row("join arrival rate requested (/s)", requestedJoinRate || "n/a", "ramp only: JOIN_TARGET / JOIN_RAMP", ""),
    row("", "", "spike arrivals = JOIN_TARGET + late tail (2% rate, 25 s)", ""),
    row("dropped_iterations", count(data, "dropped_iterations"), "arrivals k6 could not start", ""),
  ];
  const ok = rows.every((r) => r.ok) && oversell === 0;
  lines.push(row("OVERALL", "", "", ok ? "PASS" : "FAIL"));
  lines.push("authoritative oversell check: tests/load/seed-load.sh verify");
  return lines.join("\n") + "\n";
}
