/**
 * Test cho ops-service — hop dong du lieu war room (EV-187, spec §2, plan §1.1).
 *
 * Viet truoc khi co implementation: chi dua vao hop dong da chot, khong doan noi dung
 * so lieu mau cu the. Cac bat bien kiem o day:
 *   - hinh dang snapshot dung kieu da chot, isSample = true (AC-3);
 *   - so lieu tat dinh (khong Math.random) — hai lan goi chi khac generatedAt;
 *   - su kien mau lay tu getAllPublicEvents (khong chep tay);
 *   - 404 = OpsServiceError(status 404);
 *   - BR-O1 (tierInventoryViolation), tuoi du lieu (snapshotAgeMs), hang so 5s / 2s;
 *   - __setOpsSampleOverride: patch / ep loi / reset.
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { getAllPublicEvents, type EventDetailSnapshot } from "./event-service";
import {
  OPS_POLL_INTERVAL_MS,
  OpsServiceError,
  STALE_AFTER_MS,
  __setOpsSampleOverride,
  getOpsWarRoom,
  listOpsEvents,
  snapshotAgeMs,
  tierInventoryViolation,
  type OpsTierInventory,
  type OpsWarRoomSnapshot,
} from "./ops-service";

/* Them 3 su kien gia cho cac trang thai cong khai ma MOCK_EVENTS chua co (COMPLETED,
 * CANCELLED, CLOSED) de kiem bang map trang thai (plan §6 M-4). Van giu nguyen MOCK_EVENTS. */
const { EXTRA_EVENTS } = vi.hoisted(() => ({
  EXTRA_EVENTS: (
  [
    ["evt-ev187-completed", "COMPLETED"],
    ["evt-ev187-cancelled", "CANCELLED"],
    ["evt-ev187-closed", "CLOSED"],
  ] as const
).map(([id, state]) => ({
  id,
  slug: id,
  title: `Sự kiện thử ${state}`,
  description: "Sự kiện thử cho EV-187",
  venue: "Hà Nội",
  startsAt: "2026-12-01T19:00:00+07:00",
  saleStartAt: "2026-10-01T10:00:00+07:00",
  state,
  tiers: [{ id: `${id}-tier`, name: "Vé thử", price: 100000, availability: "AVAILABLE" }],
})) as EventDetailSnapshot[],
}));

vi.mock("./event-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("./event-service")>();
  const all = () => [...actual.MOCK_EVENTS, ...EXTRA_EVENTS];
  return {
    ...actual,
    getAllPublicEvents: vi.fn(async () => all()),
    getEventById: vi.fn(async (id: string) => all().find((e) => e.id === id || e.slug === id) ?? null),
    getEventBySlug: vi.fn(async (slug: string) => all().find((e) => e.slug === slug) ?? null),
    getAllEventSlugs: vi.fn(async () => all().map((e) => e.slug)),
  };
});

const SALE_STATES = ["SCHEDULED", "ON_SALE", "SOLD_OUT", "CLOSED"];
const DEGRADATION_MODES = ["NORMAL", "SAVING", "FAQ_ONLY", "OFF"];
const SOURCE_HEALTH = ["ok", "stale", "unavailable"];
const UNKNOWN_ID = "evt-khong-ton-tai-ev187";

/** Bo generatedAt de so sanh phan so lieu. */
function withoutGeneratedAt(s: OpsWarRoomSnapshot): Omit<OpsWarRoomSnapshot, "generatedAt"> {
  const { generatedAt: _ignored, ...rest } = s;
  return rest;
}

async function captureError(promise: Promise<unknown>): Promise<unknown> {
  try {
    await promise;
  } catch (err) {
    return err;
  }
  throw new Error("Ky vong promise bi reject nhung no da resolve");
}

function tier(overrides: Partial<OpsTierInventory> = {}): OpsTierInventory {
  return {
    tierId: "t1",
    name: "Hạng thử",
    quota: 100,
    held: 10,
    paid: 20,
    issued: 30,
    available: 40,
    ...overrides,
  };
}

let touchedIds: string[] = [];

afterEach(() => {
  vi.unstubAllEnvs();
  for (const id of touchedIds) {
    __setOpsSampleOverride(id, null);
  }
  touchedIds = [];
});

function override(
  id: string,
  patch: Parameters<typeof __setOpsSampleOverride>[1],
): void {
  touchedIds.push(id);
  __setOpsSampleOverride(id, patch);
}

describe("ops-service — hằng số độ tươi (AC-2)", () => {
  it("STALE_AFTER_MS = 5000 (dữ liệu trễ không quá 5s)", () => {
    expect(STALE_AFTER_MS).toBe(5_000);
  });

  it("OPS_POLL_INTERVAL_MS = 2000 (làm mới mỗi 2s)", () => {
    expect(OPS_POLL_INTERVAL_MS).toBe(2_000);
  });
});

describe("ops-service — listOpsEvents lấy sự kiện mẫu từ getAllPublicEvents", () => {
  it("trả về đúng tập id và tên của getAllPublicEvents (không chép tay)", async () => {
    const publicEvents = await getAllPublicEvents();
    const list = await listOpsEvents();

    expect(list.map((e) => e.eventId).sort()).toEqual(publicEvents.map((e) => e.id).sort());
    for (const pub of publicEvents) {
      const item = list.find((e) => e.eventId === pub.id);
      expect(item?.eventTitle).toBe(pub.title);
    }
  });

  it("mỗi mục có saleState thuộc SCHEDULED/ON_SALE/SOLD_OUT/CLOSED", async () => {
    const list = await listOpsEvents();
    for (const item of list) {
      expect(SALE_STATES).toContain(item.saleState);
    }
  });

  it("có ít nhất một sự kiện ON_SALE (kịch bản E2E cần vào war room đang bán)", async () => {
    const list = await listOpsEvents();
    expect(list.some((e) => e.saleState === "ON_SALE")).toBe(true);
  });

  it("saleState trong danh sách khớp saleState của war room cùng sự kiện", async () => {
    const list = await listOpsEvents();
    for (const item of list) {
      const snap = await getOpsWarRoom(item.eventId);
      expect(snap.saleState).toBe(item.saleState);
      expect(snap.eventTitle).toBe(item.eventTitle);
    }
  });

  it("không chứa sự kiện không tồn tại", async () => {
    const list = await listOpsEvents();
    expect(list.some((e) => e.eventId === UNKNOWN_ID)).toBe(false);
  });
});

describe("ops-service — getOpsWarRoom hình dạng snapshot (spec §2)", () => {
  it("trả đủ các nhóm trường với kiểu đúng cho mọi sự kiện mẫu", async () => {
    const publicEvents = await getAllPublicEvents();

    for (const pub of publicEvents) {
      const s = await getOpsWarRoom(pub.id);

      expect(s.eventId).toBe(pub.id);
      expect(s.eventTitle).toBe(pub.title);
      expect(SALE_STATES).toContain(s.saleState);

      for (const key of ["lobby", "queued", "admitted", "holding", "paid", "issued"] as const) {
        expect(Number.isFinite(s.funnel[key]), `${pub.id} funnel.${key}`).toBe(true);
        expect(s.funnel[key]).toBeGreaterThanOrEqual(0);
      }

      expect(Number.isFinite(s.admission.ratePerSec)).toBe(true);
      expect(s.admission.ratePerSec).toBeGreaterThanOrEqual(0);
      expect(["AUTO", "MANUAL"]).toContain(s.admission.mode);
      expect(s.admission.queueDepth).toBeGreaterThanOrEqual(0);

      expect(DEGRADATION_MODES).toContain(s.ai.mode);
      expect(typeof s.ai.manualOverride).toBe("boolean");
      expect(s.ai.chatQueueLagSeconds).toBeGreaterThanOrEqual(0);
      expect(s.ai.budgetUsedRatio).toBeGreaterThanOrEqual(0);
      expect(s.ai.budgetUsedRatio).toBeLessThanOrEqual(1);

      expect(SOURCE_HEALTH).toContain(s.sources.waitingroom);
      expect(SOURCE_HEALTH).toContain(s.sources.ticketing);
      expect(SOURCE_HEALTH).toContain(s.sources.ai);
    }
  });

  it("isSample = true cho dữ liệu mẫu (AC-3, cấm tô xanh giả)", async () => {
    const list = await listOpsEvents();
    for (const item of list) {
      expect((await getOpsWarRoom(item.eventId)).isSample).toBe(true);
    }
  });

  it("hạng vé tồn kho lấy từ hạng vé của sự kiện công khai (id và tên)", async () => {
    const publicEvents = await getAllPublicEvents();

    for (const pub of publicEvents) {
      const s = await getOpsWarRoom(pub.id);
      expect(s.inventory.map((t) => t.tierId).sort()).toEqual(pub.tiers.map((t) => t.id).sort());
      for (const t of pub.tiers) {
        expect(s.inventory.find((inv) => inv.tierId === t.id)?.name).toBe(t.name);
      }
    }
  });

  it("số liệu tồn kho mẫu không vi phạm BR-O1 (không bày lỗi giả)", async () => {
    const list = await listOpsEvents();
    for (const item of list) {
      const s = await getOpsWarRoom(item.eventId);
      for (const t of s.inventory) {
        expect(tierInventoryViolation(t), `${item.eventId}/${t.tierId}`).toBeNull();
      }
    }
  });

  it("generatedAt là ISO hợp lệ, lấy giờ lúc gọi", async () => {
    const before = Date.now();
    const s = await getOpsWarRoom("evt-tech-summit-2026");
    const after = Date.now();

    const at = Date.parse(s.generatedAt);
    expect(Number.isNaN(at)).toBe(false);
    expect(at).toBeGreaterThanOrEqual(before - 1000);
    expect(at).toBeLessThanOrEqual(after + 1000);
  });
});

describe("ops-service — số liệu tất định (không Math.random)", () => {
  it("hai lần gọi cùng sự kiện cho số liệu y hệt, chỉ khác generatedAt", async () => {
    const list = await listOpsEvents();
    for (const item of list) {
      const a = await getOpsWarRoom(item.eventId);
      const b = await getOpsWarRoom(item.eventId);
      expect(withoutGeneratedAt(b)).toEqual(withoutGeneratedAt(a));
    }
  });

  it("listOpsEvents gọi hai lần cho cùng kết quả", async () => {
    expect(await listOpsEvents()).toEqual(await listOpsEvents());
  });
});

describe("ops-service — lỗi 404 cho sự kiện không tồn tại (AC-7)", () => {
  it("ném OpsServiceError với status 404", async () => {
    const err = await captureError(getOpsWarRoom(UNKNOWN_ID));
    expect(err).toBeInstanceOf(OpsServiceError);
    expect((err as OpsServiceError).status).toBe(404);
  });

  it("OpsServiceError là Error và có message", async () => {
    const err = await captureError(getOpsWarRoom(UNKNOWN_ID));
    expect(err).toBeInstanceOf(Error);
    expect((err as Error).message.length).toBeGreaterThan(0);
  });
});

describe("tierInventoryViolation — bất biến BR-O1 (AC-4)", () => {
  it("hạng vé hợp lệ trả null", () => {
    expect(tierInventoryViolation(tier())).toBeNull();
  });

  it("held + paid + issued đúng bằng quota, available = 0 vẫn hợp lệ", () => {
    expect(
      tierInventoryViolation(tier({ quota: 60, held: 10, paid: 20, issued: 30, available: 0 })),
    ).toBeNull();
  });

  it("held + paid + issued > quota trả OVER_QUOTA", () => {
    expect(
      tierInventoryViolation(tier({ quota: 59, held: 10, paid: 20, issued: 30, available: 0 })),
    ).toBe("OVER_QUOTA");
  });

  it("available < 0 (tổng chưa vượt quota) trả NEGATIVE_AVAILABLE", () => {
    expect(
      tierInventoryViolation(tier({ quota: 100, held: 10, paid: 20, issued: 30, available: -1 })),
    ).toBe("NEGATIVE_AVAILABLE");
  });

  it("vi phạm cả hai điều kiện thì trả OVER_QUOTA (plan §6 m-6)", () => {
    expect(
      tierInventoryViolation(tier({ quota: 10, held: 10, paid: 20, issued: 30, available: -50 })),
    ).toBe("OVER_QUOTA");
  });
});

describe("snapshotAgeMs — tuổi dữ liệu tính từ generatedAt (AC-2)", () => {
  const GEN = "2026-10-13T03:00:00.000Z";
  const GEN_MS = Date.parse(GEN);

  it("bằng nowMs - generatedAt", () => {
    expect(snapshotAgeMs({ generatedAt: GEN }, GEN_MS + 1234)).toBe(1234);
  });

  it("bằng 0 khi now trùng generatedAt", () => {
    expect(snapshotAgeMs({ generatedAt: GEN }, GEN_MS)).toBe(0);
  });

  it("vượt STALE_AFTER_MS khi trễ hơn 5s", () => {
    expect(snapshotAgeMs({ generatedAt: GEN }, GEN_MS + 5001)).toBeGreaterThan(STALE_AFTER_MS);
  });

  it("generatedAt hỏng trả Infinity (coi là cũ, không coi là mới)", () => {
    expect(snapshotAgeMs({ generatedAt: "khong-phai-ngay" }, GEN_MS)).toBe(Infinity);
  });

  it("generatedAt rỗng trả Infinity", () => {
    expect(snapshotAgeMs({ generatedAt: "" }, GEN_MS)).toBe(Infinity);
  });
});

describe("__setOpsSampleOverride — chỉ dùng trong test", () => {
  const ID = "evt-tech-summit-2026";

  it("patch ghi đè đúng trường được truyền, giữ nguyên phần còn lại", async () => {
    const base = await getOpsWarRoom(ID);
    const nextState = base.saleState === "SOLD_OUT" ? "CLOSED" : "SOLD_OUT";

    override(ID, { saleState: nextState });
    const patched = await getOpsWarRoom(ID);

    expect(patched.saleState).toBe(nextState);
    expect(patched.funnel).toEqual(base.funnel);
    expect(patched.inventory).toEqual(base.inventory);
    expect(patched.isSample).toBe(true);
  });

  it("patch có thể đặt tồn kho vi phạm để kiểm AC-4", async () => {
    const bad: OpsTierInventory = tier({ tierId: "x", quota: 1, held: 5, available: -4 });
    override(ID, { inventory: [bad] });

    const patched = await getOpsWarRoom(ID);
    expect(patched.inventory).toEqual([bad]);
    expect(tierInventoryViolation(patched.inventory[0])).not.toBeNull();
  });

  it("patch generatedAt cũ được giữ nguyên để kiểm cảnh báo dữ liệu cũ", async () => {
    const old = "2020-01-01T00:00:00.000Z";
    override(ID, { generatedAt: old });
    expect((await getOpsWarRoom(ID)).generatedAt).toBe(old);
  });

  it("{ error: 403 } làm getOpsWarRoom ném OpsServiceError status 403", async () => {
    override(ID, { error: 403 });
    const err = await captureError(getOpsWarRoom(ID));
    expect(err).toBeInstanceOf(OpsServiceError);
    expect((err as OpsServiceError).status).toBe(403);
  });

  it("{ error: 500 } làm getOpsWarRoom ném OpsServiceError status 500", async () => {
    override(ID, { error: 500 });
    const err = await captureError(getOpsWarRoom(ID));
    expect(err).toBeInstanceOf(OpsServiceError);
    expect((err as OpsServiceError).status).toBe(500);
  });

  it("null đặt lại về dữ liệu mẫu gốc", async () => {
    const base = await getOpsWarRoom(ID);

    override(ID, { error: 403 });
    __setOpsSampleOverride(ID, null);
    const resetOnce = await getOpsWarRoom(ID);
    expect(withoutGeneratedAt(resetOnce)).toEqual(withoutGeneratedAt(base));

    override(ID, { saleState: "CLOSED", isSample: true });
    __setOpsSampleOverride(ID, null);
    expect(withoutGeneratedAt(await getOpsWarRoom(ID))).toEqual(withoutGeneratedAt(base));
  });

  it("override một sự kiện không ảnh hưởng sự kiện khác", async () => {
    const list = await listOpsEvents();
    const other = list.find((e) => e.eventId !== ID);
    expect(other).toBeDefined();
    const before = await getOpsWarRoom(other!.eventId);

    override(ID, { error: 403 });
    const after = await getOpsWarRoom(other!.eventId);
    expect(withoutGeneratedAt(after)).toEqual(withoutGeneratedAt(before));
  });
});

describe("ops-service — bảng số liệu mẫu tất định (plan §6 M-4)", () => {
  it("sự kiện ON_SALE: tồn kho theo availability (AVAILABLE 1000/40/300/250, FEW_LEFT 200/20/90/80)", async () => {
    const s = await getOpsWarRoom("evt-tech-summit-2026");

    expect(s.saleState).toBe("ON_SALE");
    expect(s.inventory.find((t) => t.tierId === "tier-standard-pass")).toMatchObject({
      quota: 1000,
      held: 40,
      paid: 300,
      issued: 250,
      available: 410,
    });
    expect(s.inventory.find((t) => t.tierId === "tier-all-access")).toMatchObject({
      quota: 200,
      held: 20,
      paid: 90,
      issued: 80,
      available: 10,
    });
  });

  it("sự kiện ON_SALE: phễu cố định, holding/paid/issued = tổng theo hạng vé", async () => {
    const s = await getOpsWarRoom("evt-tech-summit-2026");

    expect(s.funnel).toEqual({
      lobby: 0,
      queued: 4210,
      admitted: 350,
      holding: 60,
      paid: 390,
      issued: 330,
    });
    const sum = (k: "held" | "paid" | "issued") => s.inventory.reduce((acc, t) => acc + t[k], 0);
    expect(s.funnel.holding).toBe(sum("held"));
    expect(s.funnel.paid).toBe(sum("paid"));
    expect(s.funnel.issued).toBe(sum("issued"));
  });

  it("sự kiện ON_SALE: tuyển sinh, AI và nguồn cố định", async () => {
    const s = await getOpsWarRoom("evt-tech-summit-2026");

    expect(s.admission).toEqual({ ratePerSec: 50, mode: "AUTO", queueDepth: 4210 });
    expect(s.ai).toEqual({
      mode: "NORMAL",
      manualOverride: false,
      chatQueueLagSeconds: 2,
      budgetUsedRatio: 0.35,
    });
    expect(s.sources).toEqual({ waitingroom: "ok", ticketing: "ok", ai: "ok" });
  });

  it("hạng vé SOLD_OUT: quota 500, held 0, paid 0, issued 500, available 0", async () => {
    const s = await getOpsWarRoom("evt-indie-acoustic-night");
    expect(s.inventory.find((t) => t.tierId === "tier-regular")).toMatchObject({
      quota: 500,
      held: 0,
      paid: 0,
      issued: 500,
      available: 0,
    });
  });

  it("sự kiện không ON_SALE: queued/admitted/queueDepth/ratePerSec = 0", async () => {
    for (const id of ["evt-grand-concert-2026", "evt-indie-acoustic-night", "evt-ev187-completed"]) {
      const s = await getOpsWarRoom(id);
      expect(s.saleState, id).not.toBe("ON_SALE");
      expect(s.funnel.queued, id).toBe(0);
      expect(s.funnel.admitted, id).toBe(0);
      expect(s.admission.queueDepth, id).toBe(0);
      expect(s.admission.ratePerSec, id).toBe(0);
    }
  });
});

describe("ops-service — map trạng thái công khai sang saleState (plan §6 M-4)", () => {
  it.each([
    ["evt-tech-summit-2026", "ON_SALE"],
    ["evt-indie-acoustic-night", "SOLD_OUT"],
    ["evt-grand-concert-2026", "SCHEDULED"],
    ["evt-ev187-completed", "CLOSED"],
    ["evt-ev187-cancelled", "CLOSED"],
    ["evt-ev187-closed", "CLOSED"],
  ])("%s → %s", async (id, expected) => {
    expect((await getOpsWarRoom(id)).saleState).toBe(expected);
    expect((await listOpsEvents()).find((e) => e.eventId === id)?.saleState).toBe(expected);
  });

  it("không suy trạng thái từ Date.now (đổi giờ hệ thống không đổi kết quả)", async () => {
    const before = await listOpsEvents();
    vi.useFakeTimers();
    try {
      vi.setSystemTime(new Date("2030-01-01T00:00:00Z"));
      expect(await listOpsEvents()).toEqual(before);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("__setOpsSampleOverride — no-op khi production (plan §6 m-4)", () => {
  it("NODE_ENV=production thì override bị bỏ qua", async () => {
    vi.stubEnv("NODE_ENV", "production");
    override("evt-tech-summit-2026", { error: 403 });

    const s = await getOpsWarRoom("evt-tech-summit-2026");
    expect(s.eventId).toBe("evt-tech-summit-2026");
    expect(s.isSample).toBe(true);
  });
});

describe("tierInventoryViolation — INVALID_DATA (plan §8 C-4)", () => {
  it.each(["quota", "held", "paid", "issued", "available"] as const)(
    "%s không hữu hạn (NaN) trả INVALID_DATA",
    (field) => {
      expect(tierInventoryViolation(tier({ [field]: Number.NaN }))).toBe("INVALID_DATA");
    },
  );

  it("Infinity cũng là INVALID_DATA", () => {
    expect(tierInventoryViolation(tier({ held: Number.POSITIVE_INFINITY }))).toBe("INVALID_DATA");
    expect(tierInventoryViolation(tier({ available: Number.NEGATIVE_INFINITY }))).toBe("INVALID_DATA");
  });

  it("INVALID_DATA ưu tiên trước OVER_QUOTA", () => {
    expect(
      tierInventoryViolation(tier({ quota: 10, held: 100, paid: 100, issued: Number.NaN, available: -5 })),
    ).toBe("INVALID_DATA");
  });
});
