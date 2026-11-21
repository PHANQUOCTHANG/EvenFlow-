// Review round 1 (findings-review.json R-1..R-8). New file: the tests frozen at the red-check commit stay as they are.
import test from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, writeFileSync, readFileSync, chmodSync, existsSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, delimiter } from "node:path";
import { fileURLToPath } from "node:url";
import { buildOptions, resolveConfig, DEFAULT_QUOTA, DEFAULT_HOLD_TTL_S, PROFILES } from "./profiles.js";
import { webhookDelayMs, confirmRemote } from "./flow.js";

const thr = (o, k) => (o.thresholds[k] || []).map((t) => (typeof t === "string" ? t : t.threshold));

// ---- R-1: webhook pacing keeps up with the checkout wave and never sleeps past the hold TTL ----

test("R-1: checkout and full default WEBHOOK_RATE to CHECKOUT_RATE", () => {
  assert.equal(resolveConfig({ PROFILE: "checkout" }).webhookRate, 200);
  assert.equal(resolveConfig({ PROFILE: "full", CHECKOUT_RATE: "50" }).webhookRate, 50);
  assert.equal(resolveConfig({ PROFILE: "checkout", WEBHOOK_RATE: "300" }).webhookRate, 300);
});

test("R-1: WEBHOOK_RATE below CHECKOUT_RATE is refused at init", () => {
  for (const PROFILE of ["checkout", "full"]) {
    assert.throws(() => buildOptions({ PROFILE, WEBHOOK_RATE: "8" }), /WEBHOOK_RATE=8 is below CHECKOUT_RATE=200/);
  }
  assert.doesNotThrow(() => buildOptions({ PROFILE: "checkout", CHECKOUT_RATE: "8", WEBHOOK_RATE: "8" }));
});

test("R-1: the 4800th buyer of a default checkout wave posts its webhook without waiting", () => {
  const cfg = resolveConfig({ PROFILE: "checkout" });
  const k = 4799; // 4800th identity of the wave
  const startMs = 1_000_000;
  const arrivalMs = startMs + (k * 1000) / cfg.checkoutRate; // constant-arrival-rate start of its iteration
  const holdStartMs = arrivalMs + 3000; // join + poll to ADMITTED
  const nowMs = holdStartMs + 500; // hold + order + payment
  const d = webhookDelayMs({ k, startMs, nowMs, rate: cfg.webhookRate, holdStartMs, holdTtlS: cfg.holdTtlS });
  assert.equal(d, 0);
  // the old identity-index pacing at 8/s would have slept ~575 s, i.e. past the hold minus the 60 s margin
  assert.equal(webhookDelayMs({ k, startMs, nowMs, rate: 8, holdStartMs, holdTtlS: cfg.holdTtlS }), -1);
});

test("R-1: a webhook delay never ends inside the last 60 s of the hold", () => {
  for (let k = 0; k < 100000; k += 997) {
    for (const rate of [1, 8, 50, 200]) {
      const startMs = 0;
      const nowMs = (k * 1000) / 200 + 2000;
      const d = webhookDelayMs({ k, startMs, nowMs, rate, holdStartMs: nowMs - 1000, holdTtlS: 600 });
      if (d >= 0) assert.ok(nowMs + d <= nowMs - 1000 + 600000 - 60000, `k=${k} rate=${rate} d=${d}`);
    }
  }
});

test("R-1: HOLD_TTL defaults to the seed value and is validated", () => {
  assert.equal(resolveConfig({}).holdTtlS, DEFAULT_HOLD_TTL_S);
  assert.equal(resolveConfig({ HOLD_TTL: "120" }).holdTtlS, 120);
  assert.throws(() => resolveConfig({ HOLD_TTL: "0" }), /HOLD_TTL/);
});

// ---- R-3: one QUOTA default for seed and every profile ----

const SEED = fileURLToPath(new URL("../seed-load.sh", import.meta.url));

test("R-3: every profile uses DEFAULT_QUOTA and the oversell threshold follows it", () => {
  for (const PROFILE of PROFILES) {
    assert.equal(resolveConfig({ PROFILE }).quota, DEFAULT_QUOTA, PROFILE);
    assert.deepEqual(thr(buildOptions({ PROFILE }), "checkout_holds_ok"), [`count<=${DEFAULT_QUOTA}`]);
  }
});

test("R-3: seed-load.sh reads QUOTA and HOLD_TTL defaults from profiles.js (no second copy)", () => {
  const src = readFileSync(SEED, "utf8");
  assert.match(src, /QUOTA="\$\{QUOTA:-\$\(profile_default DEFAULT_QUOTA\)\}"/);
  assert.match(src, /HOLD_TTL="\$\{HOLD_TTL:-\$\(profile_default DEFAULT_HOLD_TTL_S\)\}"/);
  const prof = readFileSync(new URL("./profiles.js", import.meta.url), "utf8");
  assert.match(prof, new RegExp(`^export const DEFAULT_QUOTA = ${DEFAULT_QUOTA};`, "m"));
  assert.match(prof, new RegExp(`^export const DEFAULT_HOLD_TTL_S = ${DEFAULT_HOLD_TTL_S};`, "m"));
});

// ---- R-4: a run that buys nothing fails ----

test("R-4: profiles with a checkout phase require purchases, no issue timeout and successful steps", () => {
  for (const PROFILE of ["smoke", "wait", "checkout", "full"]) {
    const o = buildOptions({ PROFILE });
    assert.deepEqual(thr(o, "checkout_issued"), ["count>=1"], PROFILE);
    assert.deepEqual(thr(o, "checkout_issue_timeout"), ["count==0"], PROFILE);
    assert.deepEqual(thr(o, "checkout_step_ok"), ["rate>0.99"], PROFILE);
  }
  const spike = buildOptions({ PROFILE: "spike" });
  for (const k of ["checkout_issued", "checkout_issue_timeout", "checkout_step_ok"]) assert.equal(spike.thresholds[k], undefined);
});

// ---- R-8: ALLOW_REMOTE alone (maybe a leftover export) does not arm a remote run ----

test("R-8: a remote BASE needs ALLOW_REMOTE=1 and REMOTE_CONFIRM=<host>", () => {
  const BASE = "https://staging.example.com";
  assert.throws(() => resolveConfig({ BASE, ALLOW_REMOTE: "1" }), /REMOTE_CONFIRM=staging\.example\.com/);
  assert.throws(() => resolveConfig({ BASE, ALLOW_REMOTE: "1", REMOTE_CONFIRM: "other.example.com" }), /REMOTE_CONFIRM/);
  assert.equal(resolveConfig({ BASE, ALLOW_REMOTE: "1", REMOTE_CONFIRM: "staging.example.com" }).remote, true);
  assert.equal(resolveConfig({}).remote, false);
  assert.doesNotThrow(() => confirmRemote("localhost", {}));
});

// ---- R-2: seed-load.sh never wipes a non-load event and refuses a remote Docker engine ----

function fakeDocker(context) {
  const dir = mkdtempSync(join(tmpdir(), "k6seed-"));
  const log = join(dir, "calls.log");
  const f = join(dir, "docker");
  writeFileSync(f, `#!/usr/bin/env bash\necho "$*" >> "${log.replace(/\\/g, "/")}"\n[ "$1 $2" = "context show" ] && echo ${context}\nexit 0\n`);
  chmodSync(f, 0o755);
  return { dir, log };
}

function seed(env, docker) {
  const PATH = docker ? `${docker.dir}${delimiter}${process.env.PATH}` : process.env.PATH;
  const clean = { ...process.env, PATH };
  for (const k of ["LOAD_EVENT", "DOCKER_HOST", "ALLOW_REMOTE", "QUOTA", "USERS", "HOLD_TTL", "RESET"]) delete clean[k];
  return spawnSync("bash", [SEED, "seed"], { env: { ...clean, ...env }, encoding: "utf8" });
}

test("R-2: LOAD_EVENT outside the load range (e.g. the demo event) is refused before docker runs", () => {
  const d = fakeDocker("default");
  for (const LOAD_EVENT of ["11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "x"]) {
    const r = seed({ LOAD_EVENT }, d);
    assert.equal(r.status, 2, r.stderr);
    assert.match(r.stderr, /LOAD_EVENT must be a load event/);
  }
  assert.equal(existsSync(d.log), false, "docker was never called");
});

test("R-2: a remote docker context or DOCKER_HOST is refused unless ALLOW_REMOTE=1", () => {
  const remote = fakeDocker("prod-cluster");
  let r = seed({}, remote);
  assert.equal(r.status, 2, r.stderr);
  assert.match(r.stderr, /docker context 'prod-cluster' may be remote/);
  assert.ok(!readFileSync(remote.log, "utf8").includes("compose"), "no compose call");

  const local = fakeDocker("default");
  r = seed({ DOCKER_HOST: "tcp://10.0.0.5:2376" }, local);
  assert.equal(r.status, 2, r.stderr);
  assert.match(r.stderr, /DOCKER_HOST=tcp:\/\/10\.0\.0\.5:2376 is not a local engine/);
});
