// Pure per-VU rules: identities, idempotency keys, polling delay, target guard (AC-4, AC-5, AC-6).

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const MAX_N = 999999999999;

function checkN(n) {
  if (!Number.isInteger(n) || n < 1 || n > MAX_N) throw new Error(`identity index out of range: ${n}`);
}

// Load identities seeded by tests/load/seed-load.sh: 00000000-0000-4000-9000-{n as 12 decimal digits}.
export function identityFor(n) {
  checkN(n);
  return `00000000-0000-4000-9000-${String(n).padStart(12, "0")}`;
}

// Device id of identity n: claim "did" of its access token and anti-bot "fp".
export function deviceFor(n) {
  checkN(n);
  return `ltd-${String(n).padStart(12, "0")}`;
}

export const IDEM_STEPS = ["hold", "pay", "issue"];

export function idemKey(runId, identity, step) {
  if (typeof runId !== "string" || !/^[A-Za-z0-9_]{1,64}$/.test(runId)) {
    throw new Error(`RUN_ID must match [A-Za-z0-9_]{1,64}: ${runId}`);
  }
  if (typeof identity !== "string" || !UUID_RE.test(identity)) throw new Error(`identity is not a UUID: ${identity}`);
  if (!IDEM_STEPS.includes(step)) throw new Error(`unknown idempotency step: ${step}`);
  return `lt-${runId}-${identity}-${step}`;
}

export function header(headers, name) {
  if (!headers) return undefined;
  const want = name.toLowerCase();
  for (const k of Object.keys(headers)) if (k.toLowerCase() === want) return headers[k];
  return undefined;
}

const FLOOR_MS = 1000;

function positiveMs(v) {
  const n = typeof v === "number" ? v : typeof v === "string" && /^\d+(\.\d+)?$/.test(v.trim()) ? Number(v) : NaN;
  return Number.isFinite(n) && n > 0 ? Math.ceil(n) : null;
}

// Delay before the next status poll: what the server asked for (BR-Q4), plus client jitter >= 0.
// 200 -> body.poll_after_ms, 304 -> X-Poll-After-Ms, 429 -> Retry-After seconds; otherwise 1000 ms.
export function nextPollDelayMs(res, { jitterMaxMs = 0, rng = Math.random } = {}) {
  let ms = null;
  if (res.status === 200) ms = positiveMs(res.body && res.body.poll_after_ms);
  else if (res.status === 304) ms = positiveMs(header(res.headers, "X-Poll-After-Ms"));
  else if (res.status === 429) {
    const s = positiveMs(header(res.headers, "Retry-After"));
    ms = s === null ? null : s * 1000;
  }
  if (ms === null) ms = FLOOR_MS;
  const jitter = jitterMaxMs > 0 ? Math.floor(rng() * (jitterMaxMs + 1)) : 0;
  return ms + jitter;
}

const URL_RE = /^(https?):\/\/(\[[0-9a-fA-F:.]+\]|[A-Za-z0-9.-]+)(:\d{1,5})?(\/.*)?$/;
const LOCAL_HOSTS = ["localhost", "127.0.0.1", "[::1]"];

// Refuse to fire load at anything but this machine unless ALLOW_REMOTE=1 (AC-4).
export function guardTarget(base, env) {
  const m = typeof base === "string" ? URL_RE.exec(base) : null;
  if (!m) throw new Error(`BASE must be an http(s) URL with a host: ${base}`);
  const host = m[2].toLowerCase();
  if (LOCAL_HOSTS.includes(host)) return host;
  if ((env && env.ALLOW_REMOTE) === "1") return host;
  throw new Error(`BASE ${base} is not local; set ALLOW_REMOTE=1 to load a remote target on purpose`);
}

export function shouldReplay(rng, ratio) {
  if (ratio <= 0) return false;
  if (ratio >= 1) return true;
  return rng() < ratio;
}
