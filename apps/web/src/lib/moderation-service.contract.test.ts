/**
 * Contract test: lib/moderation-service.ts qua API that cua event service (EV-121).
 *
 *  - GET  /api/v1/moderation/queue          -> hang doi (NEEDS_HUMAN + AUTO_REJECTED)
 *  - GET  /api/v1/moderation/events/{id}    -> ho so tham dinh (noi dung goc, review AI, 4 truc, audit)
 *  - POST /api/v1/events/{id}/approve|reject -> quyet dinh; 422 override-reason-required, 409 already-decided
 * Danh tinh di bang Bearer (gateway gan principal); client KHONG tu gui X-Identity-*.
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api-client";
import {
  approveEvent,
  getModerationDetail,
  getModerationQueue,
  isOverride,
  rejectEvent,
  MODERATION_PROBLEM,
} from "@/lib/moderation-service";
import {
  BASE,
  expectNoIdentityOrCredentials,
  installFetch,
  json,
  problem,
  rejection,
  resetEnv,
  seedSession,
  setupEnv,
} from "@/test-utils/fetch-mock";

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

beforeEach(() => {
  setupEnv();
  seedSession("acc-mod", "ref-mod", "moderator");
});
afterEach(resetEnv);

const review = (over: Record<string, unknown> = {}) => ({
  id: "r-1",
  event_id: EVT,
  ai_score: 0.62,
  ai_verdict: "NEEDS_HUMAN",
  ai_reasons: ["Nghi van ban quyen hinh anh"],
  created_at: "2026-10-20T08:00:00Z",
  ...over,
});

const queueRow = (over: Record<string, unknown> = {}) => ({
  event_id: EVT,
  event_title: "AXEA Music Festival",
  event_status: "PENDING_REVIEW",
  venue: "SVD Quan khu 7",
  starts_at: "2027-01-10T12:00:00Z",
  sale_start_at: "2026-12-01T03:00:00Z",
  review: review(),
  ...over,
});

const detailBody = (over: Record<string, unknown> = {}) => ({
  event: {
    id: EVT,
    organizer_id: "org-1",
    slug: "axea",
    title: "AXEA Music Festival",
    description: "Dem nhac ngoai troi",
    venue: "SVD Quan khu 7",
    starts_at: "2027-01-10T12:00:00Z",
    sale_start_at: "2026-12-01T03:00:00Z",
    status: "PENDING_REVIEW",
    max_per_identity: 4,
    max_per_order: 4,
    hold_ttl_seconds: 600,
    created_at: "2026-10-20T07:00:00Z",
    updated_at: "2026-10-20T07:00:00Z",
    tiers: [{ id: "t-1", event_id: EVT, name: "GA", price_cents: 1200000, currency: "VND", quota: 18000, bucket_count: 32 }],
  },
  review: review(),
  ai_axes: { forbidden_content: 0.05, misleading: 0.4, copyright: 0.7, incomplete_info: 0.2 },
  audit: [
    { id: 1, actor_type: "ai", action: "event.needs_human_review", subject: `event:${EVT}`, created_at: "2026-10-20T08:00:00Z" },
    { id: 2, actor_type: "human", actor_id: "m-1", action: "event.moderator_rejected", subject: `event:${EVT}`, reason: "Thieu giay phep", created_at: "2026-10-20T09:00:00Z" },
  ],
  ...over,
});

describe("getModerationQueue", () => {
  it("GET hang doi voi Bearer, map dung truong, khong tu gui danh tinh", async () => {
    const { calls } = installFetch(() =>
      json([queueRow(), queueRow({ event_id: "e2", event_status: "REJECTED", review: review({ ai_verdict: "AUTO_REJECTED", ai_score: 0.91 }) })]),
    );
    const items = await getModerationQueue();

    expect(calls[0].url).toBe(`${BASE}/api/v1/moderation/queue`);
    expect(calls[0].headers.get("Authorization")).toBe("Bearer acc-mod");
    expectNoIdentityOrCredentials(calls[0]);
    expect(items[0]).toMatchObject({
      eventId: EVT,
      title: "AXEA Music Festival",
      status: "PENDING_REVIEW",
      aiVerdict: "NEEDS_HUMAN",
      aiScore: 0.62,
      aiReasons: ["Nghi van ban quyen hinh anh"],
      submittedAt: "2026-10-20T08:00:00Z",
    });
    expect(items[1]).toMatchObject({ aiVerdict: "AUTO_REJECTED", status: "REJECTED" });
  });

  it("phan hoi khong co review van khong vo (review null)", async () => {
    installFetch(() => json([queueRow({ review: undefined })]));
    const [item] = await getModerationQueue();
    expect(item.aiVerdict).toBeNull();
    expect(item.aiReasons).toEqual([]);
  });

  it("403 -> ApiError status 403 (khong phai Moderator)", async () => {
    installFetch(() => problem(403, "about:blank", "forbidden"));
    const err = await rejection(getModerationQueue());
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).status).toBe(403);
  });
});

describe("getModerationDetail", () => {
  it("map noi dung goc, review, 4 truc va audit", async () => {
    const { calls } = installFetch(() => json(detailBody()));
    const d = await getModerationDetail(EVT);

    expect(calls[0].url).toBe(`${BASE}/api/v1/moderation/events/${EVT}`);
    expect(d).not.toBeNull();
    expect(d!.event).toMatchObject({ id: EVT, title: "AXEA Music Festival", status: "PENDING_REVIEW", venue: "SVD Quan khu 7" });
    expect(d!.event.tiers).toEqual([{ id: "t-1", name: "GA", priceCents: 1200000, currency: "VND", quota: 18000 }]);
    expect(d!.review).toMatchObject({ aiVerdict: "NEEDS_HUMAN", aiScore: 0.62 });
    expect(d!.axes.copyright).toBe(0.7);
    expect(d!.audit.map((a) => a.action)).toEqual(["event.needs_human_review", "event.moderator_rejected"]);
    expect(d!.audit[1]).toMatchObject({ actorType: "human", reason: "Thieu giay phep" });
  });

  it("404 -> null (khong ton tai)", async () => {
    installFetch(() => problem(404, "about:blank", "event not found"));
    expect(await getModerationDetail(EVT)).toBeNull();
  });

  it("review null va khong co truc -> van dung", async () => {
    installFetch(() => json(detailBody({ review: null, ai_axes: {} })));
    const d = await getModerationDetail(EVT);
    expect(d!.review).toBeNull();
    expect(d!.axes).toEqual({});
  });
});

describe("approveEvent / rejectEvent", () => {
  it("duyet: POST approve voi note, khong gui moderator_id (lay tu token)", async () => {
    const { calls } = installFetch(() => json(detailBody().event));
    await approveEvent(EVT, "Da xac minh giay phep");

    expect(calls[0].method).toBe("POST");
    expect(calls[0].url).toBe(`${BASE}/api/v1/events/${EVT}/approve`);
    expect(calls[0].body).toEqual({ note: "Da xac minh giay phep" });
    expectNoIdentityOrCredentials(calls[0]);
  });

  it("tu choi: POST reject voi reason", async () => {
    const { calls } = installFetch(() => json({ ...detailBody().event, status: "REJECTED" }));
    const evt = await rejectEvent(EVT, "Thieu giay phep");
    expect(calls[0].url).toBe(`${BASE}/api/v1/events/${EVT}/reject`);
    expect(calls[0].body).toEqual({ reason: "Thieu giay phep" });
    expect(evt.status).toBe("REJECTED");
  });

  it("422 override-reason-required giu type de UI huong dan", async () => {
    installFetch(() => problem(422, MODERATION_PROBLEM.overrideReasonRequired, "override reason is required"));
    const err = (await rejection(approveEvent(EVT, ""))) as ApiError;
    expect(err.status).toBe(422);
    expect(err.type).toBe("/problems/override-reason-required");
  });

  it("409 already-decided giu type", async () => {
    installFetch(() => problem(409, MODERATION_PROBLEM.alreadyDecided, "moderation already decided"));
    const err = (await rejection(rejectEvent(EVT, "x"))) as ApiError;
    expect(err.status).toBe(409);
    expect(err.type).toBe("/problems/already-decided");
  });

  it("mat mang -> ApiError NETWORK_ERROR", async () => {
    installFetch(() => {
      throw new TypeError("Failed to fetch");
    });
    const err = (await rejection(rejectEvent(EVT, "x"))) as ApiError;
    expect(err.code).toBe("NETWORK_ERROR");
  });
});

describe("isOverride", () => {
  it("duyet su kien dang REJECTED la lat nguoc (can ly do)", () => {
    expect(isOverride("REJECTED")).toBe(true);
    expect(isOverride("PENDING_REVIEW")).toBe(false);
  });
});
