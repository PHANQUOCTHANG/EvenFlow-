/**
 * Contract test: lib/event-service.ts (AC-16, AC-17, AC-18).
 *
 * Hinh dang da dong bang (coordinator): getEventBySlug/getEventById tra EventDetailSnapshot | null
 * (null CHI khi 404; 400 id khong phai UUID cung la null), cac loi khac reject (Error/ApiError);
 * getAllPublicEvents tra mang va reject khi loi.
 * Gia dinh: availability chua biet = "UNKNOWN" (types/event.ts); test chi ep khac AVAILABLE/SOLD_OUT.
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { getAllPublicEvents, getEventById, getEventBySlug } from "@/lib/event-service";
import {
  BASE,
  installFetch,
  json,
  problem,
  rejection,
  resetEnv,
  setupEnv,
  type Call,
} from "@/test-utils/fetch-mock";

beforeEach(setupEnv);
afterEach(resetEnv);

const EVT_ID = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
const TIER_A = "11111111-1111-4111-8111-111111111111";
const TIER_B = "22222222-2222-4222-8222-222222222222";

function wire(over: Record<string, unknown> = {}) {
  return {
    id: EVT_ID,
    organizer_id: "org-1",
    slug: "rock-fest",
    title: "Rock Fest",
    description: "Mo ta",
    venue: "My Dinh",
    starts_at: "2026-12-01T12:00:00Z",
    sale_start_at: "2026-11-01T03:00:00Z",
    sale_end_at: "2026-11-30T03:00:00Z",
    status: "ON_SALE",
    max_per_identity: 4,
    max_per_order: 3,
    hold_ttl_seconds: 600,
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
    tiers: [
      { id: TIER_A, event_id: EVT_ID, name: "GA", price_cents: 650000, currency: "VND", quota: 100, bucket_count: 1 },
      { id: TIER_B, event_id: EVT_ID, name: "VIP", price_cents: 1800000, currency: "VND", quota: 10, bucket_count: 1 },
    ],
    ...over,
  };
}

const AVAIL = {
  event_id: EVT_ID,
  tiers: [
    { ticket_type_id: TIER_A, name: "GA", availability: "FEW_LEFT" },
    { ticket_type_id: TIER_B, name: "VIP", availability: "SOLD_OUT" },
  ],
};

function standard(c: Call, event = wire()) {
  if (c.path.endsWith("/availability")) return json(AVAIL);
  return json(event);
}

describe("getEventBySlug (AC-16)", () => {
  it("GET {base}/api/v1/events/slug/{slug} va anh xa EventResponse -> EventDetailSnapshot", async () => {
    const { calls } = installFetch((c) => standard(c));
    const e = await getEventBySlug("rock-fest");
    expect(calls[0].url).toBe(`${BASE}/api/v1/events/slug/rock-fest`);
    expect(calls[0].method).toBe("GET");
    expect(e).toMatchObject({
      id: EVT_ID,
      slug: "rock-fest",
      title: "Rock Fest",
      description: "Mo ta",
      venue: "My Dinh",
      startsAt: "2026-12-01T12:00:00Z",
      saleStartAt: "2026-11-01T03:00:00Z",
      state: "ON_SALE",
    });
  });

  it("gia ve = price_cents (VND nguyen, khong chia 100)", async () => {
    installFetch((c) => standard(c));
    const e = await getEventBySlug("rock-fest");
    expect(e?.tiers.map((t) => [t.id, t.name, t.price])).toEqual([
      [TIER_A, "GA", 650000],
      [TIER_B, "VIP", 1800000],
    ]);
  });

  it("ON_SALE: goi GET /v1/events/{id}/availability va join theo tier.id == ticket_type_id", async () => {
    const { calls } = installFetch((c) => standard(c));
    const e = await getEventBySlug("rock-fest");
    const av = calls.find((c) => c.path === `/v1/events/${EVT_ID}/availability`);
    expect(av).toBeDefined();
    expect(av?.url).toBe(`${BASE}/v1/events/${EVT_ID}/availability`);
    expect(e?.tiers.find((t) => t.id === TIER_A)?.availability).toBe("FEW_LEFT");
    expect(e?.tiers.find((t) => t.id === TIER_B)?.availability).toBe("SOLD_OUT");
  });

  it("availability that bai (404) -> availability 'unknown', KHONG gia AVAILABLE/SOLD_OUT, trang van render", async () => {
    installFetch((c) => (c.path.endsWith("/availability") ? problem(404, "/problems/not-found") : json(wire())));
    const e = await getEventBySlug("rock-fest");
    expect(e).not.toBeNull();
    for (const t of e!.tiers) {
      expect(t.availability).not.toBe("AVAILABLE");
      expect(t.availability).not.toBe("SOLD_OUT");
    }
  });

  it("availability loi mang -> khong lam hong ca trang", async () => {
    installFetch((c) => {
      if (c.path.endsWith("/availability")) throw new TypeError("Failed to fetch");
      return json(wire());
    });
    const e = await getEventBySlug("rock-fest");
    expect(e?.tiers).toHaveLength(2);
  });

  it("su kien khong ON_SALE (SCHEDULED) -> KHONG goi availability", async () => {
    const { calls } = installFetch((c) => standard(c, wire({ status: "SCHEDULED" })));
    const e = await getEventBySlug("rock-fest");
    expect(e?.state).toBe("SCHEDULED");
    expect(calls.some((c) => c.path.endsWith("/availability"))).toBe(false);
  });

  it("404 -> null, khong goi them endpoint nao (khong fallback mock/cache)", async () => {
    const { calls } = installFetch(() => problem(404, "/problems/not-found"));
    await expect(getEventBySlug("khong-ton-tai")).resolves.toBeNull();
    expect(calls).toHaveLength(1);
  });

  it("slug da tung tai thanh cong roi 404 -> van null (khong phuc vu ban nho)", async () => {
    installFetch((c) => standard(c));
    expect(await getEventBySlug("rock-fest")).not.toBeNull();
    installFetch(() => problem(404, "/problems/not-found"));
    await expect(getEventBySlug("rock-fest")).resolves.toBeNull();
  });

  it("slug chi co tren server van duoc tra (khong doc du lieu mau)", async () => {
    installFetch((c) => standard(c, wire({ slug: "slug-chi-co-tren-server" })));
    const e = await getEventBySlug("slug-chi-co-tren-server");
    expect(e?.slug).toBe("slug-chi-co-tren-server");
  });

  it("5xx -> reject (khong bien thanh 404 / null)", async () => {
    installFetch(() => problem(503, "/problems/upstream-unavailable"));
    expect(await rejection(getEventBySlug("rock-fest"))).toBeInstanceOf(Error);
  });
});

describe("getEventById (AC-18)", () => {
  it("GET {base}/api/v1/events/{id}", async () => {
    const { calls } = installFetch((c) => standard(c));
    const e = await getEventById(EVT_ID);
    expect(calls[0].url).toBe(`${BASE}/api/v1/events/${EVT_ID}`);
    expect(e?.id).toBe(EVT_ID);
  });

  it("404 -> null", async () => {
    installFetch(() => problem(404, "/problems/not-found"));
    await expect(getEventById(EVT_ID)).resolves.toBeNull();
  });

  it("400 (id khong phai UUID, vd id mau cu) -> null de UI goi notFound()", async () => {
    installFetch(() => problem(400, "about:blank", "invalid event uuid"));
    await expect(getEventById("evt-grand-concert-2026")).resolves.toBeNull();
  });

  it("5xx -> reject", async () => {
    installFetch(() => problem(503, "/problems/upstream-unavailable"));
    expect(await rejection(getEventById(EVT_ID))).toBeInstanceOf(Error);
  });
});

describe("getAllPublicEvents (AC-17)", () => {
  it("GET {base}/api/v1/events/public va anh xa {events:[...]}", async () => {
    const { calls } = installFetch((c) => (c.path === "/api/v1/events/public" ? json({ events: [wire()] }) : standard(c)));
    const list = await getAllPublicEvents();
    expect(calls[0].url).toBe(`${BASE}/api/v1/events/public`);
    expect(list).toHaveLength(1);
    expect(list[0]).toMatchObject({ id: EVT_ID, slug: "rock-fest", state: "ON_SALE" });
    expect(list[0].tiers[0].price).toBe(650000);
  });

  it("danh sach rong -> [] (UI hien empty state)", async () => {
    installFetch(() => json({ events: [] }));
    await expect(getAllPublicEvents()).resolves.toEqual([]);
  });

  it("events null -> [] (khong nem)", async () => {
    installFetch(() => json({ events: null }));
    await expect(getAllPublicEvents()).resolves.toEqual([]);
  });

  it("loi server / mang -> reject (UI hien error state), KHONG tra du lieu mau", async () => {
    installFetch(() => problem(502, "/problems/upstream-unavailable"));
    expect(await rejection(getAllPublicEvents())).toBeInstanceOf(Error);
    installFetch(() => {
      throw new TypeError("Failed to fetch");
    });
    expect(await rejection(getAllPublicEvents())).toBeInstanceOf(Error);
  });
});
