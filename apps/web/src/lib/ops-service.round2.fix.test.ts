/** ops-warroom fix round 2 (web service part):
 *  - R2-1/X2-1/X2-5: reason rules mirror app.ValidateReason (visible runes only, bidi and U+0085 rejected, NFC/NFD ok).
 *  - X2-3: a plain 401 is session loss only from the core queue source; AI / inventory 401 degrade that tile. */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  OpsServiceError,
  OpsWriteError,
  getOpsWarRoom,
  normalizeOverrideReason,
  reasonProblem,
  setAdmitOverride,
} from "@/lib/ops-service";
import { installFetch, json, problem, rejection, resetEnv, seedSession, setupEnv, type Call } from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession("acc-ops", "ref-ops", "ops");
});
afterEach(resetEnv);

const EV = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
const cp = (...codes: number[]) => String.fromCodePoint(...codes);

describe("reason rules match the server (R2-1, X2-1, X2-5)", () => {
  const BIDI = [
    cp(0x202e, 0x202e, 0x202e),
    cp(0x2066, 0x2067, 0x2068, 0x2069),
    "abc" + cp(0x200e),
    cp(0x200f) + "abc",
    "ab" + cp(0x061c) + "c",
    "resume " + cp(0x202e) + "esuap" + cp(0x202c) + " ok",
  ];
  const INVISIBLE = [
    cp(0x2060, 0x2060, 0x2060),
    cp(0xad, 0xad, 0xad),
    cp(0x301, 0x301, 0x301),
    cp(0x20dd, 0x20dd, 0x20dd),
    cp(0x3164, 0x115f, 0x1160, 0xffa0),
    cp(0x2800, 0x2800, 0x2800),
    cp(0xfe0f, 0xfe0f, 0xfe0f),
    cp(0xe0020, 0xe0020, 0xe0020),
    cp(0x180e, 0x180e, 0x180e),
    cp(0x2061, 0x2062, 0x2063, 0x2064),
    "ab" + cp(0x3164, 0x2800),
    "ab" + cp(0x301, 0x301),
  ];
  const CONTROL = [cp(0x85) + "abc" + cp(0x85), "ab" + cp(0x85) + "c", "ab\tcd"];

  it.each(BIDI.map((r) => [JSON.stringify(r), r]))("bidi %s is BIDI", (_n, r) => {
    expect(reasonProblem(r)).toBe("BIDI");
    expect(normalizeOverrideReason(r)).toBeNull();
  });
  it.each(INVISIBLE.map((r) => [JSON.stringify(r), r]))("invisible %s is LENGTH", (_n, r) => {
    expect(reasonProblem(r)).toBe("LENGTH");
    expect(normalizeOverrideReason(r)).toBeNull();
  });
  it.each(CONTROL.map((r) => [JSON.stringify(r), r]))("control %s is CONTROL (U+0085 included)", (_n, r) => {
    expect(reasonProblem(r)).toBe("CONTROL");
    expect(normalizeOverrideReason(r)).toBeNull();
  });

  it("accepts Vietnamese NFC and NFD and keeps the combining marks", () => {
    const nfc = "Khẩn cấp".normalize("NFC");
    const nfd = "Khẩn cấp".normalize("NFD");
    expect(nfd).not.toBe(nfc);
    expect(normalizeOverrideReason(nfc)).toBe(nfc);
    expect(normalizeOverrideReason(nfd)).toBe(nfd);
    expect(normalizeOverrideReason("Kha" + cp(0x302, 0x309) + "n")).toBe("Kha" + cp(0x302, 0x309) + "n");
  });

  it("trims non-bidi format characters at the edges and keeps ZWJ/ZWNJ inside", () => {
    expect(normalizeOverrideReason(cp(0x2060) + " abc " + cp(0xad))).toBe("abc");
    expect(normalizeOverrideReason("a" + cp(0x200d) + "b" + cp(0x200c) + "c")).toBe("a" + cp(0x200d) + "b" + cp(0x200c) + "c");
  });

  it("rejects a bidi reason before any fetch", async () => {
    const { calls } = installFetch(() => json({}));
    const e = await rejection(setAdmitOverride(EV, 0, BIDI[0]));
    expect(e).toBeInstanceOf(OpsWriteError);
    expect((e as OpsWriteError).code).toBe("INVALID");
    expect(calls).toHaveLength(0);
  });
});

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

type Over = Partial<Record<"event" | "queue" | "inv" | "ai", () => Response>>;
function wire(over: Over = {}) {
  return (c: Call): Response => {
    if (c.path === `/api/v1/events/${EV}`) return (over.event ?? (() => json(EVENT)))();
    if (c.path === `/v1/ops/events/${EV}/queue`) return (over.queue ?? (() => json(QUEUE)))();
    if (c.path === `/v1/ops/events/${EV}/inventory`) return (over.inv ?? (() => json(INV)))();
    if (c.path === "/v1/ops/ai/mode") return (over.ai ?? (() => json(AI)))();
    return problem(404, "/problems/not-found");
  };
}
const unauthorized = () => problem(401, "/problems/unauthorized");

describe("a 401 outside the core source degrades only that tile (X2-3)", () => {
  it("queue 200 + inventory 200 + ai 401 -> snapshot with ai unavailable", async () => {
    installFetch(wire({ ai: unauthorized }));
    const s = await getOpsWarRoom(EV);
    expect(s.sources).toEqual({ waitingroom: "ok", ticketing: "ok", ai: "unavailable" });
    expect(s.funnel.queued).toBe(4210);
  });

  it("inventory 401 -> snapshot with ticketing unavailable", async () => {
    installFetch(wire({ inv: unauthorized }));
    const s = await getOpsWarRoom(EV);
    expect(s.sources.ticketing).toBe("unavailable");
    expect(s.sources.waitingroom).toBe("ok");
  });

  it("public event detail 401 is not session loss (503, retry keeps the last snapshot)", async () => {
    installFetch(wire({ event: unauthorized }));
    const e = await rejection(getOpsWarRoom(EV));
    expect(e).toBeInstanceOf(OpsServiceError);
    expect((e as OpsServiceError).status).toBe(503);
  });

  it("queue 401 is still session loss", async () => {
    installFetch(wire({ queue: unauthorized }));
    const e = await rejection(getOpsWarRoom(EV));
    expect((e as OpsServiceError).status).toBe(401);
  });
});
