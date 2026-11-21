/**
 * ops-warroom AC-20, AC-21, AC-22, AC-25 (service part), AC-28: lib/ops-service.ts against the frozen wire contracts.
 *
 * ASSUMPTIONS (plan WP4, contracts §7):
 *  - getOpsWarRoom(eventId) uses apiFetch with the four URLs below, all started before any answer (parallel).
 *    `GET /api/v1/events/{id}` may be followed by the public availability call for ON_SALE events (event-service);
 *    that extra call is served and ignored.
 *  - Snapshot fields: admission.{overrideRatePerSec,paused,soldOut,stockZero,leaderPresent}, checkout (null | {p99Ms,errorRate,dbPoolUsage,pods}).
 *  - OpsServiceError(status) as today. OpsWriteError has `.code` in INVALID|FORBIDDEN|NOT_FOUND|UNAVAILABLE and a
 *    client-side message that never contains the server `detail`.
 *  - setAdmitOverride/clearAdmitOverride validate on the client first: rate integer 0..10000, reason trimmed 3..200;
 *    invalid input rejects with code INVALID and performs no fetch.
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import * as ops from "@/lib/ops-service";
import {
  OpsServiceError,
  clearAdmitOverride,
  getOpsWarRoom,
  listOpsEvents,
  setAdmitOverride,
} from "@/lib/ops-service";
import {
  installFetch,
  json,
  problem,
  resetEnv,
  seedSession,
  setupEnv,
  rejection,
  type Call,
} from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession("acc-ops", "ref-ops", "ops");
});
afterEach(resetEnv);

const EV = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
const T_A = "11111111-1111-4111-8111-111111111111";
const T_B = "22222222-2222-4222-8222-222222222222";

const P_EVENT = `/api/v1/events/${EV}`;
const P_QUEUE = `/v1/ops/events/${EV}/queue`;
const P_INV = `/v1/ops/events/${EV}/inventory`;
const P_AI = "/v1/ops/ai/mode";
const P_OVR = `/v1/ops/events/${EV}/admit-override`;

function wireEvent(status = "ON_SALE") {
  return {
    id: EV, organizer_id: "o", slug: "rock", title: "Rock Fest", description: "", venue: "v",
    starts_at: "2026-12-01T12:00:00Z", sale_start_at: "2026-11-01T03:00:00Z", status,
    max_per_identity: 4, max_per_order: 3, hold_ttl_seconds: 600,
    tiers: [
      { id: T_A, event_id: EV, name: "GA", price_cents: 650000, currency: "VND", quota: 100, bucket_count: 1 },
      { id: T_B, event_id: EV, name: "VIP", price_cents: 1800000, currency: "VND", quota: 10, bucket_count: 1 },
    ],
  };
}

const QUEUE = {
  event_id: EV, generated_at: "2026-11-01T10:00:05.000Z", lobby: 11, queued: 4210, admitted_active: 350,
  admit_rate_per_sec: 50, override: { active: false, rate_per_sec: null }, paused: false, sold_out: false,
  stock_zero: false, controller: { leader_present: true, epoch: 7 },
  checkout: { signal: "ok", pods: 3, p99_ms: 420, error_rate: 0.004, db_pool_usage: 0.61 },
};
const INV = {
  event_id: EV, generated_at: "2026-11-01T10:00:03Z",
  tiers: [
    { ticket_type_id: T_A, name: "GA", quota: 100, available: 41, held: 4, paid: 30, issued: 25 },
    { ticket_type_id: T_B, name: "VIP", quota: 10, available: 1, held: 1, paid: 3, issued: 5 },
  ],
};
const AI = {
  mode: "SAVING", manual_override: "OFF",
  signals: { chat_queue_lag_seconds: 7, budget_used_ratio: 0.8, gemini_circuit_open: false },
  generated_at: "2026-11-01T10:00:04Z",
};

type Overrides = Partial<Record<"event" | "queue" | "inv" | "ai", (c: Call) => Response | Promise<Response>>>;

function wire(over: Overrides = {}, eventStatus = "ON_SALE") {
  return (c: Call): Response | Promise<Response> => {
    if (c.path === P_EVENT) return (over.event ?? (() => json(wireEvent(eventStatus))))(c);
    if (c.path === P_QUEUE) return (over.queue ?? (() => json(QUEUE)))(c);
    if (c.path === P_INV) return (over.inv ?? (() => json(INV)))(c);
    if (c.path === P_AI) return (over.ai ?? (() => json(AI)))(c);
    if (c.path.endsWith("/availability")) return json({ event_id: EV, tiers: [] });
    return problem(404, "/problems/not-found");
  };
}

describe("getOpsWarRoom - calls (AC-20)", () => {
  it("calls the four endpoints exactly once each, in parallel, with the Bearer token", async () => {
    const arrived: string[] = [];
    let arrivedWhenFirstAnswered = -1;
    const required = new Set([P_EVENT, P_QUEUE, P_INV, P_AI]);
    const inner = wire();
    const { calls } = installFetch(async (c) => {
      if (required.has(c.path)) {
        arrived.push(c.path);
        // hold every answer until all four were requested (or 300 ms): proves they were started in parallel
        const deadline = Date.now() + 300;
        while (new Set(arrived).size < 4 && Date.now() < deadline) await new Promise((r) => setTimeout(r, 5));
        if (arrivedWhenFirstAnswered < 0) arrivedWhenFirstAnswered = new Set(arrived).size;
      }
      return inner(c);
    });
    await getOpsWarRoom(EV);
    expect(arrivedWhenFirstAnswered).toBe(4);
    for (const p of required) expect(calls.filter((c) => c.path === p && c.method === "GET")).toHaveLength(1);
    for (const p of [P_QUEUE, P_INV, P_AI]) {
      expect(calls.find((c) => c.path === p)!.headers.get("authorization")).toBe("Bearer acc-ops");
    }
  });
});

describe("getOpsWarRoom - mapping (AC-20)", () => {
  it("maps the real sources and is not a sample", async () => {
    installFetch(wire());
    const s = await getOpsWarRoom(EV);
    expect(s.isSample).toBe(false);
    expect(s.eventId).toBe(EV);
    expect(s.eventTitle).toBe("Rock Fest");
    expect(s.saleState).toBe("ON_SALE");
    expect(s.funnel).toEqual({ lobby: 11, queued: 4210, admitted: 350, holding: 5, paid: 33, issued: 30 });
    expect(s.admission).toMatchObject({
      ratePerSec: 50, mode: "AUTO", queueDepth: 4210, overrideRatePerSec: null, paused: false,
      soldOut: false, stockZero: false, leaderPresent: true,
    });
    expect(s.inventory).toEqual([
      { tierId: T_A, name: "GA", quota: 100, held: 4, paid: 30, issued: 25, available: 41 },
      { tierId: T_B, name: "VIP", quota: 10, held: 1, paid: 3, issued: 5, available: 1 },
    ]);
    expect(s.ai).toEqual({ mode: "SAVING", manualOverride: true, chatQueueLagSeconds: 7, budgetUsedRatio: 0.8 });
    expect(s.checkout).toEqual({ p99Ms: 420, errorRate: 0.004, dbPoolUsage: 0.61, pods: 3 });
    expect(s.sources).toEqual({ waitingroom: "ok", ticketing: "ok", ai: "ok" });
  });

  it("generatedAt is the OLDEST generated_at among the sources that succeeded", async () => {
    installFetch(wire());
    expect(Date.parse((await getOpsWarRoom(EV)).generatedAt)).toBe(Date.parse("2026-11-01T10:00:03Z"));
    installFetch(wire({ inv: () => problem(503, "/problems/x") }));
    expect(Date.parse((await getOpsWarRoom(EV)).generatedAt)).toBe(Date.parse("2026-11-01T10:00:04Z"));
  });

  it("override active => MANUAL with the override rate; rate 0 => paused", async () => {
    installFetch(
      wire({
        queue: () =>
          json({ ...QUEUE, admit_rate_per_sec: 0, override: { active: true, rate_per_sec: 0 }, paused: true }),
      }),
    );
    const s = await getOpsWarRoom(EV);
    expect(s.admission).toMatchObject({ mode: "MANUAL", overrideRatePerSec: 0, paused: true });
  });

  it("sold_out / stock_zero flags and manual_override null", async () => {
    installFetch(
      wire({
        queue: () => json({ ...QUEUE, sold_out: true, stock_zero: true }),
        ai: () => json({ ...AI, manual_override: null }),
      }),
    );
    const s = await getOpsWarRoom(EV);
    expect(s.admission).toMatchObject({ soldOut: true, stockZero: true });
    expect(s.ai.manualOverride).toBe(false);
  });

  it("checkout signal none/error => checkout null (no fabricated numbers)", async () => {
    for (const signal of ["none", "error"]) {
      installFetch(
        wire({
          queue: () => json({ ...QUEUE, checkout: { signal, pods: 0, p99_ms: null, error_rate: null, db_pool_usage: null } }),
        }),
      );
      expect((await getOpsWarRoom(EV)).checkout).toBeNull();
    }
  });

  it.each([
    ["ON_SALE", "ON_SALE"], ["SOLD_OUT", "SOLD_OUT"], ["SCHEDULED", "SCHEDULED"],
    ["CLOSED", "CLOSED"], ["COMPLETED", "CLOSED"], ["CANCELLED", "CLOSED"],
  ])("event status %s => saleState %s", async (status, expected) => {
    installFetch(wire({}, status));
    expect((await getOpsWarRoom(EV)).saleState).toBe(expected);
  });
});

describe("getOpsWarRoom - source health (AC-21)", () => {
  it.each([
    ["network error", () => { throw new TypeError("failed to fetch"); }],
    ["5xx", () => problem(500, "/problems/internal")],
    ["503", () => problem(503, "/problems/unavailable")],
  ])("queue source failing (%s) => waitingroom unavailable, the rest stays ok", async (_n, fail) => {
    installFetch(wire({ queue: fail as never }));
    const s = await getOpsWarRoom(EV);
    expect(s.sources).toEqual({ waitingroom: "unavailable", ticketing: "ok", ai: "ok" });
    expect(s.isSample).toBe(false);
  });

  it("queue 404 (room not configured) => waitingroom unavailable, not an error", async () => {
    installFetch(wire({ queue: () => problem(404, "/problems/not-found") }));
    expect((await getOpsWarRoom(EV)).sources.waitingroom).toBe("unavailable");
  });

  it("inventory and ai failures are reported per source", async () => {
    installFetch(wire({ inv: () => problem(503, "/problems/x"), ai: () => problem(500, "/problems/x") }));
    expect((await getOpsWarRoom(EV)).sources).toEqual({ waitingroom: "ok", ticketing: "unavailable", ai: "unavailable" });
  });

  it("no leader while ON_SALE => waitingroom stale; not stale when not on sale", async () => {
    const noLeader = { queue: () => json({ ...QUEUE, controller: { leader_present: false, epoch: 0 } }) };
    installFetch(wire(noLeader));
    const s = await getOpsWarRoom(EV);
    expect(s.sources.waitingroom).toBe("stale");
    expect(s.admission.leaderPresent).toBe(false);
    installFetch(wire(noLeader, "SCHEDULED"));
    expect((await getOpsWarRoom(EV)).sources.waitingroom).toBe("ok");
  });
});

describe("getOpsWarRoom - errors (AC-22)", () => {
  it("event 404 => OpsServiceError(404)", async () => {
    installFetch(wire({ event: () => problem(404, "/problems/not-found") }));
    const e = await rejection(getOpsWarRoom(EV));
    expect(e).toBeInstanceOf(OpsServiceError);
    expect((e as OpsServiceError).status).toBe(404);
  });

  it.each(["queue", "inv", "ai"] as const)("a 403 from the %s source => OpsServiceError(403), not 'unavailable'", async (k) => {
    installFetch(wire({ [k]: () => problem(403, "/problems/forbidden") }));
    const e = await rejection(getOpsWarRoom(EV));
    expect(e).toBeInstanceOf(OpsServiceError);
    expect((e as OpsServiceError).status).toBe(403);
  });

  it("event detail failing with 500 => OpsServiceError(503)", async () => {
    installFetch(wire({ event: () => problem(500, "/problems/internal") }));
    expect(((await rejection(getOpsWarRoom(EV))) as OpsServiceError).status).toBe(503);
  });

  it("all three sources failing => OpsServiceError(503)", async () => {
    installFetch(
      wire({
        queue: () => problem(503, "/problems/x"),
        inv: () => problem(503, "/problems/x"),
        ai: () => problem(503, "/problems/x"),
      }),
    );
    const e = await rejection(getOpsWarRoom(EV));
    expect(e).toBeInstanceOf(OpsServiceError);
    expect((e as OpsServiceError).status).toBe(503);
  });
});

describe("listOpsEvents (AC-26 data source)", () => {
  it("lists /api/v1/events/public", async () => {
    const { calls } = installFetch((c) =>
      c.path === "/api/v1/events/public"
        ? json({ events: [wireEvent("ON_SALE"), { ...wireEvent("SCHEDULED"), id: "e2", title: "Two" }] })
        : problem(404, "/problems/x"),
    );
    const list = await listOpsEvents();
    expect(calls.map((c) => c.path)).toContain("/api/v1/events/public");
    expect(list).toEqual([
      { eventId: EV, eventTitle: "Rock Fest", saleState: "ON_SALE" },
      { eventId: "e2", eventTitle: "Two", saleState: "SCHEDULED" },
    ]);
  });

  it("rejects when the list fails", async () => {
    installFetch(() => problem(500, "/problems/x"));
    await expect(listOpsEvents()).rejects.toBeTruthy();
  });
});

describe("sample removal (AC-28)", () => {
  it("__setOpsSampleOverride is gone", () => {
    expect("__setOpsSampleOverride" in ops).toBe(false);
  });
});

/* ---------------- writes (AC-24 validation, AC-25 results) ---------------- */

const OK_SET = { event_id: EV, override: { active: true, rate_per_sec: 120 }, paused: false, applied_at: "2026-11-01T10:00:01Z", audit: "recorded" };
const OK_CLEAR = { event_id: EV, override: { active: false, rate_per_sec: null }, paused: false, applied_at: "2026-11-01T10:00:01Z", audit: "recorded" };

describe("setAdmitOverride", () => {
  it("PUTs {rate_per_sec, reason} (trimmed) and maps the result", async () => {
    const { calls } = installFetch(() => json(OK_SET));
    const r = await setAdmitOverride(EV, 120, "  giam tai  ");
    expect(calls).toHaveLength(1);
    expect(calls[0].method).toBe("PUT");
    expect(calls[0].path).toBe(P_OVR);
    expect(calls[0].body).toEqual({ rate_per_sec: 120, reason: "giam tai" });
    expect(calls[0].headers.get("authorization")).toBe("Bearer acc-ops");
    expect(r).toEqual({ active: true, ratePerSec: 120, paused: false, audit: "recorded" });
  });

  it("pause (rate 0) is paused; audit failed is surfaced", async () => {
    installFetch(() => json({ ...OK_SET, override: { active: true, rate_per_sec: 0 }, paused: true, audit: "failed" }));
    expect(await setAdmitOverride(EV, 0, "khan cap")).toEqual({ active: true, ratePerSec: 0, paused: true, audit: "failed" });
  });

  it.each([
    ["negative", -1, "abc"], ["too big", 10001, "abc"], ["fraction", 1.5, "abc"], ["NaN", Number.NaN, "abc"],
    ["Infinity", Infinity, "abc"], ["short reason", 5, "ab"], ["blank reason", 5, "    "], ["long reason", 5, "x".repeat(201)],
  ])("invalid input (%s) => INVALID and no request", async (_n, rate, reason) => {
    const { calls } = installFetch(() => json(OK_SET));
    const e = (await rejection(setAdmitOverride(EV, rate as number, reason as string))) as { code?: string };
    expect(e?.code).toBe("INVALID");
    expect(calls).toHaveLength(0);
  });

  it("boundaries 0 and 10000, reason 3 and 200 chars are accepted", async () => {
    const { calls } = installFetch(() => json(OK_SET));
    await setAdmitOverride(EV, 0, "abc");
    await setAdmitOverride(EV, 10000, "x".repeat(200));
    expect(calls).toHaveLength(2);
  });

  it.each([
    [400, "/problems/invalid-override", "INVALID"],
    [403, "/problems/forbidden", "FORBIDDEN"],
    [404, "/problems/not-found", "NOT_FOUND"],
    [503, "/problems/audit-unavailable", "UNAVAILABLE"],
    [503, "/problems/unavailable", "UNAVAILABLE"],
  ])("HTTP %s %s => code %s and the server detail is never shown", async (status, type, code) => {
    installFetch(() => problem(status, type, "SERVER-DETAIL-LEAK"));
    const e = (await rejection(setAdmitOverride(EV, 5, "abc"))) as { code?: string; message?: string };
    expect(e?.code).toBe(code);
    expect(e?.message ?? "").not.toContain("SERVER-DETAIL-LEAK");
  });

  it("network failure => UNAVAILABLE", async () => {
    installFetch(() => { throw new TypeError("failed to fetch"); });
    expect(((await rejection(setAdmitOverride(EV, 5, "abc"))) as { code?: string }).code).toBe("UNAVAILABLE");
  });
});

describe("clearAdmitOverride", () => {
  it("DELETEs {reason} and maps the result", async () => {
    const { calls } = installFetch(() => json(OK_CLEAR));
    const r = await clearAdmitOverride(EV, " su co da xu ly ");
    expect(calls[0].method).toBe("DELETE");
    expect(calls[0].path).toBe(P_OVR);
    expect(calls[0].body).toEqual({ reason: "su co da xu ly" });
    expect(r).toEqual({ active: false, ratePerSec: null, paused: false, audit: "recorded" });
  });

  it.each(["", "ab", "   ", "x".repeat(201)])("invalid reason %j => INVALID, no request", async (reason) => {
    const { calls } = installFetch(() => json(OK_CLEAR));
    expect(((await rejection(clearAdmitOverride(EV, reason))) as { code?: string }).code).toBe("INVALID");
    expect(calls).toHaveLength(0);
  });

  it("403 => FORBIDDEN", async () => {
    installFetch(() => problem(403, "/problems/forbidden"));
    expect(((await rejection(clearAdmitOverride(EV, "abc"))) as { code?: string }).code).toBe("FORBIDDEN");
  });
});
