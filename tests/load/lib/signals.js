// Join signals, anti-bot payload v1 (origin/feature/antibot-phase1: docs/workflow/antibot-phase1/contracts.md §1).
// The only place that builds the payload: if the contract is amended, change this module (and the range check in
// signals.test.mjs). Pure, deterministic for a given rng.
import { randInt } from "./rng.js";

export const SIGNAL_MODES = ["realistic", "min", "none"];

// FNV-1a 64 over the UTF-8 bytes of str, as 16 lowercase hex chars. 16-bit limbs: no BigInt needed in k6.
export function fnv1a64Hex(str) {
  // offset basis 0xcbf29ce484222325, limbs little-endian
  let h = [0x2325, 0x8422, 0x9ce4, 0xcbf2];
  const bytes = unescape(encodeURIComponent(str));
  for (let i = 0; i < bytes.length; i++) {
    h[0] ^= bytes.charCodeAt(i);
    // h * 0x100000001b3 = h * 0x1b3 + (h << 40)
    const r = [0, 0, 0, 0];
    let carry = 0;
    for (let j = 0; j < 4; j++) {
      const v = h[j] * 0x1b3 + carry;
      r[j] = v & 0xffff;
      carry = Math.floor(v / 0x10000);
    }
    // h << 40: limb j moves to limb j + 2 with an 8-bit shift
    const s = [0, 0, (h[0] << 8) & 0xffff, ((h[1] << 8) | (h[0] >>> 8)) & 0xffff];
    carry = 0;
    for (let j = 0; j < 4; j++) {
      const v = r[j] + s[j] + carry;
      r[j] = v & 0xffff;
      carry = v >>> 16;
    }
    h = r;
  }
  return h.slice().reverse().map((x) => x.toString(16).padStart(4, "0")).join("");
}

const ZONES = [
  ["Asia/Ho_Chi_Minh", 420],
  ["Asia/Ho_Chi_Minh", 420],
  ["Asia/Ho_Chi_Minh", 420],
  ["Asia/Bangkok", 420],
  ["Asia/Singapore", 480],
];
const KINDS = ["p", "s", "k"];

function cv(xs) {
  const m = xs.reduce((a, b) => a + b, 0) / xs.length;
  const sd = Math.sqrt(xs.reduce((a, b) => a + (b - m) * (b - m), 0) / xs.length);
  return m === 0 ? 0 : sd / m;
}

function intervals(rng) {
  for (;;) {
    const n = randInt(rng, 10, 24);
    const iv = [];
    for (let i = 0; i < n; i++) {
      // mostly quick moves with a few pauses, like a person reading the page
      iv.push(rng() < 0.2 ? randInt(rng, 800, 6000) : randInt(rng, 60, 700));
    }
    if (cv(iv) >= 0.25 && new Set(iv).size > 1) return iv;
  }
}

export function signalsFor(mode, { identityIdx, deviceId, rng }) {
  if (mode === "none") return {};
  if (mode === "min") return { v: 1 };
  if (mode !== "realistic") throw new Error(`SIGNALS must be one of ${SIGNAL_MODES.join(", ")}: ${mode}`);
  const iv = intervals(rng);
  const kinds = [];
  let pe = 1; // the join click
  let se = 0;
  let ke = 0;
  for (let i = 0; i < iv.length; i++) {
    const k = rng() < 0.6 ? "p" : KINDS[randInt(rng, 1, 2)];
    kinds.push(k);
    if (k === "p") pe++;
    else if (k === "s") se++;
    else ke++;
  }
  // sq: digest of the last <= 32 events as kind:bucket (bucket = interval / 50 ms, capped 40), salted with the
  // identity so that two load identities never share a sequence (R7 would read that as a replay).
  const events = ["p:0"].concat(iv.map((x, i) => `${kinds[i]}:${Math.min(40, Math.floor(x / 50))}`)).slice(-32);
  const [tz, tzo] = ZONES[randInt(rng, 0, ZONES.length - 1)];
  return {
    v: 1,
    rt: randInt(rng, 2500, 45000),
    pe,
    se,
    ke,
    iv,
    sq: fnv1a64Hex(`${events.join(",")}|lt${identityIdx}`),
    sn: events.length,
    fp: deviceId,
    wd: false,
    gl: true,
    fc: randInt(rng, 6, 14),
    tz,
    tzo,
  };
}
