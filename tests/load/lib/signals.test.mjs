// AC-7 / AC-12: join signals follow the anti-bot v1 contract
// (origin/feature/antibot-phase1: docs/workflow/antibot-phase1/contracts.md section 1).
import test from "node:test";
import assert from "node:assert/strict";
import { signalsFor, fnv1a64Hex, SIGNAL_MODES } from "./signals.js";
import { deviceFor } from "./flow.js";
import { mulberry32 } from "./rng.js";

const intIn = (v, lo, hi) => Number.isInteger(v) && v >= lo && v <= hi;

// Every range of contracts.md section 1. Returns the list of violations.
function contractViolations(s) {
  const bad = [];
  if (s.v !== 1) bad.push("v");
  if (!intIn(s.rt, 0, 600000)) bad.push("rt");
  for (const k of ["pe", "se", "ke"]) if (!intIn(s[k], 0, 10000)) bad.push(k);
  if (!Array.isArray(s.iv) || s.iv.length > 32 || !s.iv.every((x) => intIn(x, 0, 60000))) bad.push("iv");
  if (typeof s.sq !== "string" || !/^[0-9a-f]{16}$/.test(s.sq)) bad.push("sq");
  if (!intIn(s.sn, 0, 32)) bad.push("sn");
  if (typeof s.fp !== "string" || !/^[A-Za-z0-9-]{8,64}$/.test(s.fp)) bad.push("fp");
  if (s.wd !== false) bad.push("wd");
  if (s.gl !== true) bad.push("gl");
  if (!intIn(s.fc, 0, 20)) bad.push("fc");
  if (typeof s.tz !== "string" || !/^[A-Za-z_+\-/0-9]{1,64}$/.test(s.tz)) bad.push("tz");
  if (!intIn(s.tzo, -840, 840)) bad.push("tzo");
  return bad;
}

function cv(xs) {
  const m = xs.reduce((a, b) => a + b, 0) / xs.length;
  const sd = Math.sqrt(xs.reduce((a, b) => a + (b - m) ** 2, 0) / xs.length);
  return m === 0 ? 0 : sd / m;
}

const gen = (i, mode = "realistic") =>
  signalsFor(mode, { identityIdx: i, deviceId: deviceFor(i), rng: mulberry32(1000 + i) });

test("modes are realistic, min, none", () => {
  assert.deepEqual([...SIGNAL_MODES].sort(), ["min", "none", "realistic"]);
  assert.throws(() => gen(1, "bogus"), /SIGNALS/);
});

test("realistic payload satisfies every v1 range", () => {
  for (let i = 1; i <= 500; i++) {
    const s = gen(i);
    assert.deepEqual(contractViolations(s), [], `identity ${i}: ${JSON.stringify(s)}`);
  }
});

test("realistic payload looks human to R1/R2/R5/R5W/wd/gl", () => {
  for (let i = 1; i <= 500; i++) {
    const s = gen(i);
    assert.ok(s.iv.length >= 8, "at least 8 intervals (R1_CLIENT_MIN)");
    assert.ok(new Set(s.iv).size > 1, "intervals not constant");
    assert.ok(cv(s.iv) >= 0.2, `interval CV ${cv(s.iv)} well above R1_CLIENT_CV=0.10`);
    assert.ok(s.rt >= 1000, "time to click well above R2_MS=120");
    assert.ok(s.fc >= 3, "fonts >= R5_FONTS_MIN");
    assert.ok(s.pe >= 1, "the join click is a pointer event");
    assert.ok(s.sn >= 6 && s.sn <= 32, "sq only sent with >= 6 events");
    assert.equal(s.sn, Math.min(32, s.iv.length + 1));
  }
});

test("fp is the VU device id; tz and tzo are consistent", () => {
  const offsets = { "Asia/Ho_Chi_Minh": 420, "Asia/Bangkok": 420, "Asia/Jakarta": 420, "Asia/Singapore": 480 };
  for (let i = 1; i <= 50; i++) {
    const s = gen(i);
    assert.equal(s.fp, deviceFor(i));
    assert.ok(s.tz in offsets, s.tz);
    assert.equal(s.tzo, offsets[s.tz]);
  }
});

test("sq is unique per identity (R7 replay rule must not fire)", () => {
  const seen = new Set();
  for (let i = 1; i <= 5000; i++) seen.add(gen(i).sq);
  assert.equal(seen.size, 5000);
});

test("same identity and seed give the same payload (deterministic)", () => {
  assert.deepEqual(gen(17), gen(17));
});

test("serialised size <= 2 KiB", () => {
  for (let i = 1; i <= 500; i++) {
    const n = Buffer.byteLength(JSON.stringify(gen(i)), "utf8");
    assert.ok(n <= 2048, `size ${n}`);
  }
});

test("min sends {v:1} and none sends {}", () => {
  assert.deepEqual(gen(3, "min"), { v: 1 });
  assert.deepEqual(gen(3, "none"), {});
});

test("fnv1a64Hex matches the FNV-1a 64 reference vectors", () => {
  assert.equal(fnv1a64Hex(""), "cbf29ce484222325");
  assert.equal(fnv1a64Hex("a"), "af63dc4c8601ec8c");
  assert.equal(fnv1a64Hex("foobar"), "85944171f73967e8");
});
