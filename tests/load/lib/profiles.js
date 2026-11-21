// Profiles -> k6 options (AC-2, AC-3, AC-8, AC-9). Pure: no k6 import, every error is thrown at init.
import { guardTarget } from "./flow.js";
import { SIGNAL_MODES } from "./signals.js";

export const PROFILES = ["smoke", "spike", "wait", "checkout", "full"];

export const DEFAULT_BASE = "http://localhost:8080";
export const DEFAULT_EVENT = "22222222-2222-4222-8222-000000000194"; // seed-load.sh LOAD_EVENT
export const DEV_JWT_SECRET = "dev-jwt-secret-change-me-0123456789"; // compose DEV default, never a real secret

// Defaults of plan.md section 3. Every value can be overridden by env.
const BASE_DEFAULTS = {
  users: 0, // 0 = exactly what the profile needs
  quota: 10000,
  sloScale: 1,
  joinTarget: 100000,
  joinRamp: "5s",
  jitterMs: 5000,
  waitVus: 20000,
  waitDuration: "30m",
  checkoutRate: 200,
  checkoutDuration: "5m",
  webhookRate: 8,
  replayRatio: 0.1,
  signals: "realistic",
};
const PROFILE_DEFAULTS = {
  smoke: { joinTarget: 20, users: 50, quota: 200, sloScale: 4, jitterMs: 1000, waitDuration: "60s" },
  spike: {},
  wait: {},
  checkout: {},
  full: {},
};

const SMOKE_MAX_S = 100;
const SPIKE_TAIL_S = 25;
const FULL_WAIT_START_S = 10;
const FULL_CHECKOUT_START_S = 60;
const GRACE_S = 30;

export function parseDurationS(v, name) {
  const s = String(v === undefined ? "" : v);
  const m = /^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?$/.exec(s);
  const total = m && s !== "" ? Number(m[1] || 0) * 3600 + Number(m[2] || 0) * 60 + Number(m[3] || 0) : 0;
  if (!m || total <= 0) throw new Error(`${name} must be a positive duration like 5s, 30m, 1h, 1m30s: "${s}"`);
  return total;
}

function posInt(v, name) {
  const s = String(v);
  if (!/^\d+$/.test(s) || Number(s) <= 0) throw new Error(`${name} must be a positive integer: "${s}"`);
  return Number(s);
}

function nonNegInt(v, name) {
  const s = String(v);
  if (!/^\d+$/.test(s)) throw new Error(`${name} must be a non-negative integer: "${s}"`);
  return Number(s);
}

function posNumber(v, name) {
  const s = String(v);
  if (!/^\d+(\.\d+)?$/.test(s) || Number(s) <= 0) throw new Error(`${name} must be a positive number: "${s}"`);
  return Number(s);
}

function ratio(v, name) {
  const s = String(v);
  if (!/^\d+(\.\d+)?$/.test(s) || Number(s) > 1) throw new Error(`${name} must be a number in [0,1]: "${s}"`);
  return Number(s);
}

const pick = (env, key, def) => (env[key] === undefined || env[key] === null || env[key] === "" ? def : env[key]);

function spikeShape(cfg) {
  const rate = Math.ceil(cfg.joinTarget / cfg.joinRampS);
  const tail = Math.max(1, Math.ceil(rate * 0.02));
  // arrivals upper bound: `rate` for JOIN_RAMP, a 1 s step down, then the late tail
  const arrivals = rate * cfg.joinRampS + Math.ceil((rate + tail) / 2) + tail * (SPIKE_TAIL_S - 1);
  return { rate, tail, arrivals };
}

export function resolveConfig(env = {}) {
  const profile = String(pick(env, "PROFILE", "smoke"));
  if (!PROFILES.includes(profile)) throw new Error(`PROFILE must be one of ${PROFILES.join(", ")}: "${profile}"`);
  const d = { ...BASE_DEFAULTS, ...PROFILE_DEFAULTS[profile] };

  const base = String(pick(env, "BASE", DEFAULT_BASE));
  guardTarget(base, env);
  const event = String(pick(env, "EVENT", DEFAULT_EVENT));
  if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/.test(event)) {
    throw new Error(`EVENT is not a UUID: "${event}"`);
  }
  const ticketType = env.TICKET_TYPE ? String(env.TICKET_TYPE) : "";
  const jwtSecret = String(pick(env, "JWT_SECRET", DEV_JWT_SECRET));
  if (jwtSecret.length < 16) throw new Error("JWT_SECRET must be at least 16 bytes (authx minSecretLen)");
  const runId = env.RUN_ID === undefined ? "" : String(env.RUN_ID);
  if (runId !== "" && !/^[A-Za-z0-9_]{1,64}$/.test(runId)) {
    throw new Error(`RUN_ID must match [A-Za-z0-9_]{1,64}: "${runId}"`);
  }
  const signals = String(pick(env, "SIGNALS", d.signals));
  if (!SIGNAL_MODES.includes(signals)) throw new Error(`SIGNALS must be one of ${SIGNAL_MODES.join(", ")}: "${signals}"`);

  const cfg = {
    profile,
    base: base.replace(/\/+$/, ""),
    event,
    ticketType,
    jwtSecret,
    runId,
    signals,
    quota: posInt(pick(env, "QUOTA", d.quota), "QUOTA"),
    sloScale: posNumber(pick(env, "SLO_SCALE", d.sloScale), "SLO_SCALE"),
    joinTarget: posInt(pick(env, "JOIN_TARGET", d.joinTarget), "JOIN_TARGET"),
    joinRampS: parseDurationS(pick(env, "JOIN_RAMP", d.joinRamp), "JOIN_RAMP"),
    jitterMs: nonNegInt(pick(env, "JITTER_MS", d.jitterMs), "JITTER_MS"),
    waitVus: posInt(pick(env, "WAIT_VUS", d.waitVus), "WAIT_VUS"),
    waitDurationS: parseDurationS(pick(env, "WAIT_DURATION", d.waitDuration), "WAIT_DURATION"),
    checkoutRate: posInt(pick(env, "CHECKOUT_RATE", d.checkoutRate), "CHECKOUT_RATE"),
    checkoutDurationS: parseDurationS(pick(env, "CHECKOUT_DURATION", d.checkoutDuration), "CHECKOUT_DURATION"),
    webhookRate: posInt(pick(env, "WEBHOOK_RATE", d.webhookRate), "WEBHOOK_RATE"),
    replayRatio: ratio(pick(env, "REPLAY_RATIO", d.replayRatio), "REPLAY_RATIO"),
    // statuses that are a valid outcome of a step (304 not modified, 409 sold out / limit); used by checks.
    // http_req_failed itself counts only 5xx and network errors (AC-3), see main.js.
    expectedStatuses: [200, 201, 204, 304, 409],
  };
  cfg.joinMaxVus = posInt(pick(env, "JOIN_MAX_VUS", cfg.joinTarget), "JOIN_MAX_VUS");
  const usersEnv = pick(env, "USERS", null) === null ? null : posInt(env.USERS, "USERS");

  // identity ranges: scenarios never share an identity
  const sizes = { join: 0, wait: 0, checkout: 0 };
  if (profile === "smoke") sizes.join = cfg.joinTarget;
  if (profile === "spike" || profile === "full") sizes.join = spikeShape(cfg).arrivals;
  if (profile === "wait" || profile === "full") sizes.wait = cfg.waitVus;
  if (profile === "checkout" || profile === "full") sizes.checkout = cfg.checkoutRate * cfg.checkoutDurationS;
  cfg.ranges = {};
  let next = 1;
  for (const k of ["join", "wait", "checkout"]) {
    cfg.ranges[k] = { start: next, size: sizes[k] };
    next += sizes[k];
  }
  const required = next - 1;
  if (usersEnv !== null && usersEnv < required) {
    throw new Error(`USERS=${usersEnv} is smaller than the ${required} identities profile ${profile} needs`);
  }
  cfg.users = usersEnv !== null ? usersEnv : Math.max(d.users, required);

  const spikeS = cfg.joinRampS + SPIKE_TAIL_S + Math.ceil(cfg.jitterMs / 1000) + GRACE_S;
  const waitS = cfg.waitDurationS + GRACE_S;
  const checkoutS = cfg.checkoutDurationS + 60 + GRACE_S;
  cfg.runSeconds = {
    smoke: SMOKE_MAX_S,
    spike: spikeS,
    wait: waitS,
    checkout: checkoutS,
    full: Math.max(spikeS, FULL_WAIT_START_S + waitS, FULL_CHECKOUT_START_S + checkoutS),
  }[profile];
  return cfg;
}

function scenarios(cfg) {
  const sc = {};
  if (cfg.profile === "smoke") {
    sc.smoke = {
      executor: "shared-iterations",
      vus: cfg.joinTarget,
      iterations: cfg.joinTarget,
      maxDuration: `${SMOKE_MAX_S}s`,
      exec: "smokeFlow",
    };
    return sc;
  }
  const full = cfg.profile === "full";
  if (cfg.profile === "spike" || full) {
    const { rate, tail } = spikeShape(cfg);
    sc.joinBurst = {
      executor: "ramping-arrival-rate",
      startRate: rate,
      timeUnit: "1s",
      preAllocatedVUs: Math.min(rate, cfg.joinMaxVus),
      maxVUs: cfg.joinMaxVus,
      stages: [
        { target: rate, duration: `${cfg.joinRampS}s` },
        { target: tail, duration: "1s" },
        { target: tail, duration: `${SPIKE_TAIL_S - 1}s` },
      ],
      exec: "joinBurst",
    };
    if (full) sc.joinBurst.startTime = "0s";
  }
  if (cfg.profile === "wait" || full) {
    sc.waitAndPoll = { executor: "constant-vus", vus: cfg.waitVus, duration: `${cfg.waitDurationS}s`, exec: "waitAndPoll" };
    if (full) sc.waitAndPoll.startTime = `${FULL_WAIT_START_S}s`;
  }
  if (cfg.profile === "checkout" || full) {
    sc.checkoutWave = {
      executor: "constant-arrival-rate",
      rate: cfg.checkoutRate,
      timeUnit: "1s",
      duration: `${cfg.checkoutDurationS}s`,
      preAllocatedVUs: cfg.checkoutRate * 30,
      maxVUs: cfg.checkoutRate * 120,
      exec: "checkoutWave",
    };
    if (full) sc.checkoutWave.startTime = `${FULL_CHECKOUT_START_S}s`;
  }
  return sc;
}

const round = (x) => Math.round(x * 1000) / 1000;

function thresholds(cfg) {
  const S = cfg.sloScale;
  const lat = (ms) => [`p(99)<${round(ms * S)}`];
  const t = {
    wr_join_latency: lat(300),
    wr_status_latency: lat(150),
    co_hold_latency: lat(800),
    co_payment_latency: lat(800),
    "http_req_duration{phase:join}": lat(300),
    "http_req_duration{phase:status}": lat(150),
    "http_req_duration{phase:hold}": lat(800),
    "http_req_duration{phase:payment}": lat(800),
    wr_join_success: ["rate>0.995"],
    wr_status_429: ["rate<0.001"],
    http_req_failed: ["rate<0.01"],
    // count invariants never scale and stop the run at once
    checkout_holds_ok: [{ threshold: `count<=${cfg.quota}`, abortOnFail: true }],
    idem_mismatch: [{ threshold: "count==0", abortOnFail: true }],
  };
  if (cfg.profile === "smoke") {
    t.wr_admitted = ["count>=1"];
    t.checkout_issued = ["count>=1"];
    t.wr_status_304_ratio = ["rate>0"];
  }
  return t;
}

export function buildOptions(env = {}) {
  const cfg = resolveConfig(env);
  return {
    scenarios: scenarios(cfg),
    thresholds: thresholds(cfg),
    summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
    setupTimeout: "90s",
    teardownTimeout: "30s",
  };
}
