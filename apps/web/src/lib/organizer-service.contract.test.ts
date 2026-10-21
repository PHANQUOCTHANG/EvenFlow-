/**
 * Contract test: lib/organizer-service.ts qua API that (AC-20..AC-23, AC-3).
 *
 * Gia dinh:
 *  - Loi validation phia client (truoc khi goi mang): ham reject HOAC resolve voi `error` va khong co `event`;
 *    test chap nhan ca hai, chi ep "khong co request nao".
 *  - updateOrganizerEvent(id, partial) tu ghep voi su kien hien tai (GET) de luon PUT day du.
 *  - Loi 422 tu submit/cancel/update: Error.message chua problem `detail`.
 *  - updateTierQuota(eventId, tierId, newQuota) tu doc su kien hien tai de chan giam quota khi ON_SALE.
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  addTier,
  cancelOrganizerEvent,
  createOrganizerEvent,
  getOrganizerEventById,
  getOrganizerEvents,
  submitOrganizerEvent,
  updateOrganizerEvent,
  updateTierQuota,
  type CreateEventInput,
  type EventLifecycleState,
} from "@/lib/organizer-service";
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
  type Call,
} from "@/test-utils/fetch-mock";

const EVT_ID = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
const TIER_ID = "11111111-1111-4111-8111-111111111111";

beforeEach(() => {
  setupEnv();
  seedSession("acc-org", "ref-org", "organizer");
});
afterEach(resetEnv);

function wire(over: Record<string, unknown> = {}) {
  return {
    id: EVT_ID,
    organizer_id: "org-1",
    slug: "rock-fest",
    title: "Rock Fest",
    description: "Mo ta",
    venue: "My Dinh",
    starts_at: "2027-12-01T12:00:00Z",
    sale_start_at: "2027-11-01T03:00:00Z",
    status: "DRAFT",
    max_per_identity: 4,
    max_per_order: 4,
    hold_ttl_seconds: 600,
    created_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-01T00:00:00Z",
    ...over,
  };
}

const INPUT: CreateEventInput = {
  title: "Rock Fest",
  description: "Mo ta",
  venue: "My Dinh",
  startsAt: "2027-12-01T12:00:00Z",
  saleStartAt: "2027-11-01T03:00:00Z",
  tiers: [
    { id: "t-new-1", name: "GA", price: 650000, quota: 100 },
    { id: "t-new-2", name: "VIP", price: 1800000, quota: 10 },
  ],
};

const sameInstant = (a: unknown, b: string) => new Date(String(a)).getTime() === new Date(b).getTime();

/** Gia lap backend tao su kien thanh cong. */
function createRoutes(c: Call) {
  if (c.method === "POST" && c.path === "/api/v1/events") return json(wire(), 201);
  if (c.method === "POST" && c.path === `/api/v1/events/${EVT_ID}/tiers`) {
    const b = c.body as { name: string; price_cents: number; quota: number };
    return json({ id: `srv-${b.name}`, event_id: EVT_ID, ...b, currency: "VND", bucket_count: 1 }, 201);
  }
  if (c.method === "POST" && c.path === `/api/v1/events/${EVT_ID}/submit`) return json(wire({ status: "PENDING_REVIEW" }));
  return problem(404, "/problems/not-found");
}

describe("getOrganizerEvents / getOrganizerEventById (AC-20)", () => {
  it("GET {base}/api/v1/events voi Bearer, khong yeu cau tiers", async () => {
    const { calls } = installFetch(() => json([wire(), wire({ id: "e2", status: "COMPLETED" })]));
    const list = await getOrganizerEvents();
    expect(calls[0].url).toBe(`${BASE}/api/v1/events`);
    expect(calls[0].method).toBe("GET");
    expect(calls[0].headers.get("Authorization")).toBe("Bearer acc-org");
    expectNoIdentityOrCredentials(calls[0]);
    expect(list).toHaveLength(2);
    expect(list[0]).toMatchObject({ id: EVT_ID, slug: "rock-fest", title: "Rock Fest", state: "DRAFT" });
    expect(list[0].tiers ?? []).toEqual([]);
  });

  it("hien du ca 9 trang thai, ke ca COMPLETED", async () => {
    const all: EventLifecycleState[] = [
      "DRAFT",
      "PENDING_REVIEW",
      "SCHEDULED",
      "ON_SALE",
      "SOLD_OUT",
      "CLOSED",
      "COMPLETED",
      "REJECTED",
      "CANCELLED",
    ];
    installFetch(() => json(all.map((status, i) => wire({ id: `e${i}`, status }))));
    const list = await getOrganizerEvents();
    expect(list.map((e) => e.state)).toEqual(all);
  });

  it("danh sach rong -> []", async () => {
    installFetch(() => json([]));
    await expect(getOrganizerEvents()).resolves.toEqual([]);
  });

  it("danh sach null -> []", async () => {
    installFetch(() => json(null));
    await expect(getOrganizerEvents()).resolves.toEqual([]);
  });

  it("loi server -> reject, khong tra du lieu mock", async () => {
    installFetch(() => problem(500, "about:blank"));
    expect(await rejection(getOrganizerEvents())).toBeInstanceOf(Error);
  });

  it("getOrganizerEventById: GET /api/v1/events/{id}; 404 -> null", async () => {
    const { calls } = installFetch(() =>
      json(wire({ tiers: [{ id: TIER_ID, event_id: EVT_ID, name: "GA", price_cents: 650000, quota: 100 }] })),
    );
    const e = await getOrganizerEventById(EVT_ID);
    expect(calls[0].url).toBe(`${BASE}/api/v1/events/${EVT_ID}`);
    expect(e?.tiers[0]).toMatchObject({ id: TIER_ID, name: "GA", price: 650000, quota: 100 });
    installFetch(() => problem(404, "/problems/not-found"));
    await expect(getOrganizerEventById(EVT_ID)).resolves.toBeNull();
  });
});

describe("createOrganizerEvent (AC-21)", () => {
  it("POST /api/v1/events -> POST tiers cho tung hang ve -> khong submit neu khong chon", async () => {
    const { calls } = installFetch(createRoutes);
    const r = await createOrganizerEvent(INPUT);
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "POST /api/v1/events",
      `POST /api/v1/events/${EVT_ID}/tiers`,
      `POST /api/v1/events/${EVT_ID}/tiers`,
    ]);
    const body = calls[0].body as Record<string, unknown>;
    expect(calls[0].url).toBe(`${BASE}/api/v1/events`);
    expect(body.title).toBe("Rock Fest");
    expect(body.description).toBe("Mo ta");
    expect(body.venue).toBe("My Dinh");
    expect(sameInstant(body.starts_at, INPUT.startsAt)).toBe(true);
    expect(sameInstant(body.sale_start_at, INPUT.saleStartAt)).toBe(true);
    expect(calls[1].body).toEqual({ name: "GA", price_cents: 650000, quota: 100 });
    expect(calls[2].body).toEqual({ name: "VIP", price_cents: 1800000, quota: 10 });
    for (const c of calls) {
      expect(c.headers.get("Authorization")).toBe("Bearer acc-org");
      expectNoIdentityOrCredentials(c);
    }
    expect(r.event.id).toBe(EVT_ID);
    expect(r.failedStep).toBeUndefined();
    expect(r.error).toBeUndefined();
  });

  it("submitForReview=true -> goi them POST /submit sau khi tao xong tiers", async () => {
    const { calls } = installFetch(createRoutes);
    const r = await createOrganizerEvent({ ...INPUT, submitForReview: true });
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      "POST /api/v1/events",
      `POST /api/v1/events/${EVT_ID}/tiers`,
      `POST /api/v1/events/${EVT_ID}/tiers`,
      `POST /api/v1/events/${EVT_ID}/submit`,
    ]);
    expect(r.event.state).toBe("PENDING_REVIEW");
    expect(r.failedStep).toBeUndefined();
  });

  it("slug duoc gui khi nguoi dung nhap", async () => {
    const { calls } = installFetch(createRoutes);
    await createOrganizerEvent({ ...INPUT, slug: "my-slug" });
    expect((calls[0].body as { slug?: string }).slug).toBe("my-slug");
  });

  it("tier thu 2 loi 422 -> failedStep 'tiers', co error, tra ban nhap da tao, KHONG goi submit", async () => {
    const { calls } = installFetch((c) => {
      if (c.method === "POST" && c.path.endsWith("/tiers") && (c.body as { name: string }).name === "VIP") {
        return problem(422, "about:blank", "ten hang ve bi trung");
      }
      return createRoutes(c);
    });
    const r = await createOrganizerEvent({ ...INPUT, submitForReview: true });
    expect(r.failedStep).toBe("tiers");
    expect(r.error).toBeTruthy();
    expect(r.event.id).toBe(EVT_ID);
    expect(calls.some((c) => c.path.endsWith("/submit"))).toBe(false);
  });

  it("submit loi 422 -> failedStep 'submit', error chua detail, van tra ban nhap", async () => {
    installFetch((c) =>
      c.path.endsWith("/submit") ? problem(422, "about:blank", "can it nhat 1 hang ve") : createRoutes(c),
    );
    const r = await createOrganizerEvent({ ...INPUT, submitForReview: true });
    expect(r.failedStep).toBe("submit");
    expect(String(r.error)).toContain("can it nhat 1 hang ve");
    expect(r.event.id).toBe(EVT_ID);
  });

  const invalid: [string, Partial<CreateEventInput>][] = [
    ["starts_at == sale_start_at", { startsAt: "2027-11-01T03:00:00Z", saleStartAt: "2027-11-01T03:00:00Z" }],
    ["starts_at < sale_start_at", { startsAt: "2027-10-01T03:00:00Z", saleStartAt: "2027-11-01T03:00:00Z" }],
    ["khong co hang ve", { tiers: [] }],
    ["quota <= 0", { tiers: [{ id: "t", name: "GA", price: 1000, quota: 0 }] }],
    ["gia < 0", { tiers: [{ id: "t", name: "GA", price: -1, quota: 5 }] }],
  ];
  it.each(invalid)("validate truoc khi goi mang: %s", async (_name, patch) => {
    const { calls } = installFetch(createRoutes);
    let threw = false;
    let value: { event?: unknown; error?: unknown } | undefined;
    try {
      value = await createOrganizerEvent({ ...INPUT, ...patch });
    } catch {
      threw = true;
    }
    expect(calls).toHaveLength(0);
    if (!threw) {
      expect(value?.error).toBeTruthy();
      expect(value?.event).toBeFalsy();
    }
  });

  it("tao su kien (buoc 1) loi -> khong goi tier nao", async () => {
    const { calls } = installFetch(() => problem(500, "about:blank"));
    let threw = false;
    let value: { event?: unknown } | undefined;
    try {
      value = await createOrganizerEvent(INPUT);
    } catch {
      threw = true;
    }
    expect(calls.filter((c) => c.path.endsWith("/tiers"))).toHaveLength(0);
    if (!threw) expect(value?.event).toBeFalsy();
  });
});

describe("updateOrganizerEvent (AC-22)", () => {
  it("PUT /api/v1/events/{id} voi DAY DU truong, khong slug, khong tiers", async () => {
    const { calls } = installFetch(() => json(wire({ title: "Moi" })));
    const out = await updateOrganizerEvent(EVT_ID, {
      title: "Moi",
      description: "D2",
      venue: "V2",
      startsAt: "2027-12-02T12:00:00Z",
      saleStartAt: "2027-11-02T03:00:00Z",
    });
    const put = calls.filter((c) => c.method === "PUT");
    expect(put).toHaveLength(1);
    expect(put[0].url).toBe(`${BASE}/api/v1/events/${EVT_ID}`);
    const body = put[0].body as Record<string, unknown>;
    expect(Object.keys(body)).toEqual(
      expect.arrayContaining(["title", "description", "venue", "starts_at", "sale_start_at"]),
    );
    expect(body).not.toHaveProperty("slug");
    expect(body).not.toHaveProperty("tiers");
    expect(body.title).toBe("Moi");
    expect(body.description).toBe("D2");
    expect(sameInstant(body.starts_at, "2027-12-02T12:00:00Z")).toBe(true);
    expect(put[0].headers.get("Authorization")).toBe("Bearer acc-org");
    expect(out.title).toBe("Moi");
  });

  it("cap nhat mot phan ({title}) -> van PUT day du: lay phan con lai tu su kien hien tai", async () => {
    const { calls } = installFetch(() => json(wire()));
    await updateOrganizerEvent(EVT_ID, { title: "Chi doi tieu de" });
    const put = calls.find((c) => c.method === "PUT");
    expect(put).toBeDefined();
    const body = put!.body as Record<string, unknown>;
    expect(body.title).toBe("Chi doi tieu de");
    expect(body.description).toBe("Mo ta");
    expect(body.venue).toBe("My Dinh");
    expect(sameInstant(body.starts_at, "2027-12-01T12:00:00Z")).toBe(true);
    expect(sameInstant(body.sale_start_at, "2027-11-01T03:00:00Z")).toBe(true);
  });

  it("starts_at <= sale_start_at -> khong goi PUT (backend se lang le bo qua)", async () => {
    const { calls } = installFetch(() => json(wire()));
    await updateOrganizerEvent(EVT_ID, {
      startsAt: "2027-11-01T03:00:00Z",
      saleStartAt: "2027-11-01T03:00:00Z",
    }).catch(() => undefined);
    expect(calls.some((c) => c.method === "PUT")).toBe(false);
  });

  it("422 trang thai khong cho sua -> reject voi detail", async () => {
    installFetch((c) => (c.method === "PUT" ? problem(422, "about:blank", "su kien khong the chinh sua") : json(wire())));
    const err = await rejection(
      updateOrganizerEvent(EVT_ID, {
        title: "x",
        description: "d",
        venue: "v",
        startsAt: "2027-12-01T12:00:00Z",
        saleStartAt: "2027-11-01T03:00:00Z",
      }),
    );
    expect((err as Error).message).toContain("su kien khong the chinh sua");
  });
});

describe("tiers (AC-22)", () => {
  it("addTier: POST /api/v1/events/{id}/tiers {name, price_cents, quota}", async () => {
    const { calls } = installFetch(() =>
      json({ id: TIER_ID, event_id: EVT_ID, name: "Early", price_cents: 300000, quota: 50 }, 201),
    );
    await addTier(EVT_ID, { id: "tmp", name: "Early", price: 300000, quota: 50 });
    expect(calls[0].url).toBe(`${BASE}/api/v1/events/${EVT_ID}/tiers`);
    expect(calls[0].method).toBe("POST");
    expect(calls[0].body).toEqual({ name: "Early", price_cents: 300000, quota: 50 });
  });

  it("updateTierQuota: PUT /api/v1/events/{id}/tiers/{tierId} chi {new_quota}", async () => {
    const { calls } = installFetch((c) =>
      c.method === "PUT"
        ? json({ id: TIER_ID, event_id: EVT_ID, name: "GA", price_cents: 650000, quota: 200 })
        : json(
            wire({
              status: "DRAFT",
              tiers: [{ id: TIER_ID, event_id: EVT_ID, name: "GA", price_cents: 650000, quota: 100 }],
            }),
          ),
    );
    await updateTierQuota(EVT_ID, TIER_ID, 200);
    const put = calls.find((c) => c.method === "PUT");
    expect(put?.url).toBe(`${BASE}/api/v1/events/${EVT_ID}/tiers/${TIER_ID}`);
    expect(put?.body).toEqual({ new_quota: 200 });
  });

  it("ON_SALE: giam quota bi chan o client (khong goi PUT); tang quota duoc phep", async () => {
    const onSale = wire({
      status: "ON_SALE",
      tiers: [{ id: TIER_ID, event_id: EVT_ID, name: "GA", price_cents: 650000, quota: 100 }],
    });
    const tier = (quota: number) => ({ id: TIER_ID, event_id: EVT_ID, name: "GA", price_cents: 650000, quota });
    const a = installFetch((c) => (c.method === "PUT" ? json(tier(50)) : json(onSale)));
    expect(await rejection(updateTierQuota(EVT_ID, TIER_ID, 50))).toBeInstanceOf(Error);
    expect(a.calls.some((c) => c.method === "PUT")).toBe(false);

    const b = installFetch((c) => (c.method === "PUT" ? json(tier(150)) : json(onSale)));
    await updateTierQuota(EVT_ID, TIER_ID, 150);
    expect(b.calls.some((c) => c.method === "PUT")).toBe(true);
  });
});

describe("submit / cancel (AC-23)", () => {
  it("submitOrganizerEvent: POST /api/v1/events/{id}/submit -> PENDING_REVIEW", async () => {
    const { calls } = installFetch(() => json(wire({ status: "PENDING_REVIEW" })));
    const e = await submitOrganizerEvent(EVT_ID);
    expect(calls[0].url).toBe(`${BASE}/api/v1/events/${EVT_ID}/submit`);
    expect(calls[0].method).toBe("POST");
    expect(calls[0].headers.get("Authorization")).toBe("Bearer acc-org");
    expect(e.state).toBe("PENDING_REVIEW");
  });

  it("cancelOrganizerEvent: POST /api/v1/events/{id}/cancel -> CANCELLED", async () => {
    const { calls } = installFetch(() => json(wire({ status: "CANCELLED" })));
    const e = await cancelOrganizerEvent(EVT_ID);
    expect(calls[0].url).toBe(`${BASE}/api/v1/events/${EVT_ID}/cancel`);
    expect(calls[0].method).toBe("POST");
    expect(e.state).toBe("CANCELLED");
  });

  it.each([
    ["submit", () => submitOrganizerEvent(EVT_ID)],
    ["cancel", () => cancelOrganizerEvent(EVT_ID)],
  ])("%s 422 -> reject voi problem detail", async (_n, run) => {
    installFetch(() => problem(422, "about:blank", "khong the chuyen trang thai"));
    const err = await rejection(run());
    expect(err).toBeInstanceOf(Error);
    expect((err as Error).message).toContain("khong the chuyen trang thai");
  });
});
