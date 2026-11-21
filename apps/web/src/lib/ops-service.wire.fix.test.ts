/** ops-warroom fix round 1 (web service part):
 *  - R-4 + X-6: a 200 write without `audit` (or an unknown value) is NOT recorded; malformed 200 bodies give a typed
 *    error, never a raw TypeError; a queue body without `override` keeps the rest of the snapshot.
 *  - R-5 + X-7: a plain HTTP 401 on a write or on a read source goes to the auth-required path. */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api-client";
import { OpsServiceError, OpsWriteError, clearAdmitOverride, getOpsWarRoom, setAdmitOverride } from "@/lib/ops-service";
import { installFetch, json, problem, rejection, resetEnv, seedSession, setupEnv, type Call } from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession("acc-ops", "ref-ops", "ops");
});
afterEach(resetEnv);

const EV = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
const P_EVENT = `/api/v1/events/${EV}`;
const P_QUEUE = `/v1/ops/events/${EV}/queue`;
const P_INV = `/v1/ops/events/${EV}/inventory`;
const P_AI = "/v1/ops/ai/mode";
const P_OVR = `/v1/ops/events/${EV}/admit-override`;

const EVENT = {
  id: EV, organizer_id: "o", slug: "rock", title: "Rock Fest", description: "", venue: "v",
  starts_at: "2026-12-01T12:00:00Z", sale_start_at: "2026-11-01T03:00:00Z", status: "ON_SALE",
  max_per_identity: 4, max_per_order: 3, hold_ttl_seconds: 600, tiers: [],
};
const QUEUE = {
  event_id: EV, generated_at: "2026-11-01T10:00:05.000Z", lobby: 11, queued: 4210, admitted_active: 350,
  admit_rate_per_sec: 50, override: { active: false, rate_per_sec: null }, paused: false, sold_out: false,
  stock_zero: false, controller: { leader_present: true, epoch: 7 },
  checkout: { signal: "ok", pods: 3, p99_ms: 420, error_rate: 0.004, db_pool_usage: 0.61 },
};
const INV = { event_id: EV, generated_at: "2026-11-01T10:00:03Z", tiers: [] };
const AI = {
  mode: "SAVING", manual_override: null,
  signals: { chat_queue_lag_seconds: 7, budget_used_ratio: 0.8, gemini_circuit_open: false },
  generated_at: "2026-11-01T10:00:04Z",
};

type Over = Partial<Record<"queue" | "inv" | "ai", () => Response>>;
function wire(over: Over = {}) {
  return (c: Call): Response => {
    if (c.path === P_EVENT) return json(EVENT);
    if (c.path === P_QUEUE) return (over.queue ?? (() => json(QUEUE)))();
    if (c.path === P_INV) return (over.inv ?? (() => json(INV)))();
    if (c.path === P_AI) return (over.ai ?? (() => json(AI)))();
    if (c.path.endsWith("/availability")) return json({ event_id: EV, tiers: [] });
    return problem(404, "/problems/not-found");
  };
}

const ovr = (res: () => Response) => (c: Call) => (c.path === P_OVR ? res() : problem(404, "/x"));

describe("write result audit status fails closed (R-4, X-6)", () => {
  it("a 200 without audit is reported as failed", async () => {
    installFetch(ovr(() => json({ override: { active: true, rate_per_sec: 0 }, paused: true })));
    expect((await setAdmitOverride(EV, 0, "abc")).audit).toBe("failed");
  });

  it("an unknown audit value is reported as failed", async () => {
    installFetch(ovr(() => json({ override: { active: false, rate_per_sec: null }, paused: false, audit: "ok" })));
    expect((await clearAdmitOverride(EV, "abc")).audit).toBe("failed");
  });

  it("only the exact value recorded is recorded", async () => {
    installFetch(ovr(() => json({ override: { active: true, rate_per_sec: 5 }, paused: false, audit: "recorded" })));
    expect((await setAdmitOverride(EV, 5, "abc")).audit).toBe("recorded");
  });

  it.each([[{}], [null], [[]], [{ override: null, paused: true, audit: "recorded" }], [{ override: { active: "yes" } }]])(
    "a malformed 200 body %j rejects with OpsWriteError, not a TypeError",
    async (body) => {
      installFetch(ovr(() => json(body)));
      const e = await rejection(setAdmitOverride(EV, 0, "abc"));
      expect(e).toBeInstanceOf(OpsWriteError);
      expect((e as OpsWriteError).code).toBe("UNAVAILABLE");
    },
  );
});

describe("queue body shape (X-6)", () => {
  it("a queue body without override keeps the rest of the snapshot", async () => {
    installFetch(wire({ queue: () => json({ ...QUEUE, override: undefined }) }));
    const s = await getOpsWarRoom(EV);
    expect(s.funnel.lobby).toBe(11);
    expect(s.admission.ratePerSec).toBe(50);
    expect(s.admission.mode).toBe("AUTO");
    expect(s.admission.overrideRatePerSec).toBeNull();
    expect(s.sources.waitingroom).toBe("ok");
  });

  it("a queue body without controller or checkout does not throw", async () => {
    installFetch(wire({ queue: () => json({ ...QUEUE, controller: undefined, checkout: undefined }) }));
    const s = await getOpsWarRoom(EV);
    expect(s.funnel.queued).toBe(4210);
    expect(s.checkout).toBeNull();
  });

  it("a non-object source body marks only that source unavailable", async () => {
    installFetch(wire({ queue: () => json(null), ai: () => json({ ...AI, signals: undefined }) }));
    const s = await getOpsWarRoom(EV);
    expect(s.sources.waitingroom).toBe("unavailable");
    expect(s.sources.ticketing).toBe("ok");
  });
});

describe("plain 401 goes to the auth-required path (R-5, X-7)", () => {
  it("a 401 on PUT rejects with ApiError AUTH_REQUIRED, not FORBIDDEN", async () => {
    installFetch(ovr(() => problem(401, "/problems/unauthorized")));
    const e = await rejection(setAdmitOverride(EV, 0, "abc"));
    expect(e).toBeInstanceOf(ApiError);
    expect((e as ApiError).code).toBe("AUTH_REQUIRED");
  });

  it("a 401 on DELETE rejects with ApiError AUTH_REQUIRED", async () => {
    installFetch(ovr(() => problem(401, "/problems/unauthorized")));
    const e = await rejection(clearAdmitOverride(EV, "abc"));
    expect((e as ApiError).code).toBe("AUTH_REQUIRED");
  });

  it("a 401 on a read source rejects the snapshot with OpsServiceError(401)", async () => {
    installFetch(wire({ queue: () => problem(401, "/problems/unauthorized") }));
    const e = await rejection(getOpsWarRoom(EV));
    expect(e).toBeInstanceOf(OpsServiceError);
    expect((e as OpsServiceError).status).toBe(401);
  });
});
