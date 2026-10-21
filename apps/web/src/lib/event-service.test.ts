/**
 * Test cho event-service (EVF-111, BR-A4) -- ban thay the cho test doc MOCK_EVENTS.
 * event-service goi API that qua gateway; o day fetch duoc mock (xem them event-service.contract.test.ts).
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { getAllPublicEvents, getEventBySlug } from "./event-service";
import { installFetch, json, problem, rejection, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(setupEnv);
afterEach(resetEnv);

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
const TIER = "11111111-1111-4111-8111-111111111111";

function wire(over: Record<string, unknown> = {}) {
  return {
    id: EVT,
    slug: "evenflow-grand-concert-2026",
    title: "EvenFlow Grand Concert 2026",
    description: "Mo ta",
    venue: "My Dinh",
    starts_at: "2026-12-01T12:00:00Z",
    sale_start_at: "2026-11-01T03:00:00Z",
    status: "ON_SALE",
    max_per_order: 4,
    tiers: [{ id: TIER, event_id: EVT, name: "GA", price_cents: 650000, currency: "VND", quota: 100, bucket_count: 1 }],
    ...over,
  };
}

describe("event-service (EVF-111, BR-A4)", () => {
  it("tim dung su kien theo slug hop le", async () => {
    installFetch((c) =>
      c.path.endsWith("/availability")
        ? json({ event_id: EVT, tiers: [{ ticket_type_id: TIER, name: "GA", availability: "AVAILABLE" }] })
        : json(wire()),
    );
    const event = await getEventBySlug("evenflow-grand-concert-2026");
    expect(event).not.toBeNull();
    expect(event?.slug).toBe("evenflow-grand-concert-2026");
    expect(event?.title).toContain("EvenFlow Grand Concert 2026");
  });

  it("tra ve null khi khong tim thay slug (404)", async () => {
    installFetch(() => problem(404, "/problems/not-found"));
    await expect(getEventBySlug("slug-khong-ton-tai-404")).resolves.toBeNull();
  });

  it("tra ve toan bo danh sach su kien cong khai tu API", async () => {
    installFetch(() => json({ events: [wire(), wire({ id: "other", slug: "other" })] }));
    const events = await getAllPublicEvents();
    expect(events).toHaveLength(2);
  });

  it("khong goi backend luc next build (NEXT_PHASE = phase-production-build)", async () => {
    const { calls } = installFetch(() => json({ events: [] }));
    const prev = process.env.NEXT_PHASE;
    process.env.NEXT_PHASE = "phase-production-build";
    try {
      expect(await rejection(getAllPublicEvents())).toBeInstanceOf(Error);
      expect(await rejection(getEventBySlug("x"))).toBeInstanceOf(Error);
    } finally {
      if (prev === undefined) delete process.env.NEXT_PHASE;
      else process.env.NEXT_PHASE = prev;
    }
    expect(calls).toHaveLength(0);
  });

  it("tuan thu BR-A4: availability chi dang khoang, snapshot khong lo quota/remaining", async () => {
    installFetch((c) =>
      c.path.endsWith("/availability")
        ? json({ event_id: EVT, tiers: [{ ticket_type_id: TIER, name: "GA", availability: "FEW_LEFT" }] })
        : json(wire()),
    );
    const event = await getEventBySlug("evenflow-grand-concert-2026");
    for (const tier of event?.tiers ?? []) {
      expect(["AVAILABLE", "FEW_LEFT", "SOLD_OUT", "UNKNOWN"]).toContain(tier.availability);
      // Khong co field quota/remaining chinh xac trong snapshot cong khai
      expect((tier as unknown as Record<string, unknown>).quota).toBeUndefined();
      expect((tier as unknown as Record<string, unknown>).remaining).toBeUndefined();
    }
    expect(event?.tiers[0].availability).toBe("FEW_LEFT");
  });
});
