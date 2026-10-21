/** Ops Service — war room cho Ops (EV-187, EVF-117).
 *
 * Hien chi la DU LIEU MAU tat dinh (isSample = true), cung mau voi lib/organizer-service.ts.
 * Khi co Ops API that (admit controller, ticketing, ai-worker /admin/mode) chi can thay than
 * cua getOpsWarRoom / listOpsEvents; hop dong kieu giu nguyen.
 *
 * Handoff muc 10.18 cam "fabricated green health": snapshot mau luon gan isSample = true de UI
 * hien nhan "Du lieu mau". So lieu khong dung Math.random va khong suy trang thai tu Date.now. */

import { SAMPLE_EVENTS } from "./sample-data/events";
import type { EventDetailSnapshot, TierAvailability } from "./types/event";

export type DegradationMode = "NORMAL" | "SAVING" | "FAQ_ONLY" | "OFF";
export type SourceHealth = "ok" | "stale" | "unavailable";
export type OpsSaleState = "SCHEDULED" | "ON_SALE" | "SOLD_OUT" | "CLOSED";

export interface OpsTierInventory {
  tierId: string;
  name: string;
  quota: number;
  held: number;
  paid: number;
  issued: number;
  available: number;
}

export interface OpsWarRoomSnapshot {
  eventId: string;
  eventTitle: string;
  saleState: OpsSaleState;
  /** ISO, gio server luc chup so lieu. */
  generatedAt: string;
  funnel: {
    lobby: number;
    queued: number;
    admitted: number;
    holding: number;
    paid: number;
    issued: number;
  };
  admission: { ratePerSec: number; mode: "AUTO" | "MANUAL"; queueDepth: number };
  inventory: OpsTierInventory[];
  ai: {
    mode: DegradationMode;
    manualOverride: boolean;
    chatQueueLagSeconds: number;
    budgetUsedRatio: number;
  };
  sources: { waitingroom: SourceHealth; ticketing: SourceHealth; ai: SourceHealth };
  /** true = du lieu mau, UI phai gan nhan. */
  isSample: boolean;
}

/** Nhan hien thi cua trang thai ban (dung chung danh sach va war room). */
export const SALE_STATE_LABEL: Record<OpsSaleState, string> = {
  SCHEDULED: "Chưa mở bán",
  ON_SALE: "Đang mở bán",
  SOLD_OUT: "Hết vé",
  CLOSED: "Đã đóng",
};

/** Nhan trang thai ban; gia tri la (du lieu hong) -> "Không rõ trạng thái". */
export function saleStateLabel(state: string): string {
  return (SALE_STATE_LABEL as Record<string, string | undefined>)[state] ?? "Không rõ trạng thái";
}

/** Lay `status` tu loi bat ky (OpsServiceError hoac object thuong); khong co thi undefined. */
export function opsErrorStatus(err: unknown): number | undefined {
  const status = (err as { status?: unknown } | null)?.status;
  return typeof status === "number" ? status : undefined;
}

export interface OpsEventListItem {
  eventId: string;
  eventTitle: string;
  saleState: OpsSaleState;
}

/** 403 = khong co quyen, 404 = khong co su kien. */
export class OpsServiceError extends Error {
  status: number;

  constructor(status: number, message?: string) {
    super(message ?? defaultMessage(status));
    this.name = "OpsServiceError";
    this.status = status;
  }
}

function defaultMessage(status: number): string {
  if (status === 403) return "Bạn không có quyền xem war room.";
  if (status === 404) return "Không tìm thấy sự kiện.";
  return "Không tải được số liệu war room.";
}

/** Du lieu tre qua nguong nay (ms) bi coi la cu (AC-2). */
export const STALE_AFTER_MS = 5_000;
/** Chu ky lam moi tu dong (ms). */
export const OPS_POLL_INTERVAL_MS = 2_000;

/** Kiem tra bat bien BR-O1 cho mot hang ve. Vi pham ca hai thi bao OVER_QUOTA; du lieu khong huu han -> INVALID_DATA. */
export function tierInventoryViolation(
  t: OpsTierInventory,
): "INVALID_DATA" | "OVER_QUOTA" | "NEGATIVE_AVAILABLE" | null {
  // So khong huu han (NaN/Infinity) thi so sanh nao cung sai lech: bao rieng, uu tien truoc.
  if (![t.quota, t.held, t.paid, t.issued, t.available].every(Number.isFinite)) {
    return "INVALID_DATA";
  }
  if (t.held + t.paid + t.issued > t.quota) return "OVER_QUOTA";
  if (t.available < 0) return "NEGATIVE_AVAILABLE";
  return null;
}

/** Tuoi du lieu (ms) = nowMs - generatedAt. generatedAt hong => Infinity (coi la cu). */
export function snapshotAgeMs(snapshot: Pick<OpsWarRoomSnapshot, "generatedAt">, nowMs: number): number {
  const at = Date.parse(snapshot.generatedAt);
  return Number.isNaN(at) ? Infinity : nowMs - at;
}

type SampleOverride = Partial<OpsWarRoomSnapshot> | { error: number };

const overrides = new Map<string, SampleOverride>();

/** Chi dung trong test: ghi de / ep loi du lieu mau cua mot su kien; null = dat lai.
 *  No-op khi production. */
export function __setOpsSampleOverride(eventId: string, patch: SampleOverride | null): void {
  if (process.env.NODE_ENV === "production") return;
  if (patch === null) {
    overrides.delete(eventId);
    return;
  }
  overrides.set(eventId, patch);
}

function toSaleState(state: EventDetailSnapshot["state"]): OpsSaleState {
  switch (state) {
    case "ON_SALE":
      return "ON_SALE";
    case "SOLD_OUT":
      return "SOLD_OUT";
    case "SCHEDULED":
      return "SCHEDULED";
    default:
      return "CLOSED";
  }
}

/** Bang so lieu mau theo availability cong khai (plan §6 M-4). */
const TIER_SAMPLE: Record<TierAvailability, Omit<OpsTierInventory, "tierId" | "name">> = {
  AVAILABLE: { quota: 1000, held: 40, paid: 300, issued: 250, available: 410 },
  FEW_LEFT: { quota: 200, held: 20, paid: 90, issued: 80, available: 10 },
  SOLD_OUT: { quota: 500, held: 0, paid: 0, issued: 500, available: 0 },
  UNKNOWN: { quota: 0, held: 0, paid: 0, issued: 0, available: 0 },
};

function buildSample(event: EventDetailSnapshot): OpsWarRoomSnapshot {
  const saleState = toSaleState(event.state);
  const live = saleState === "ON_SALE";

  const inventory: OpsTierInventory[] = event.tiers.map((tier) => ({
    tierId: tier.id,
    name: tier.name,
    ...TIER_SAMPLE[tier.availability],
  }));
  const sum = (key: "held" | "paid" | "issued") => inventory.reduce((acc, t) => acc + t[key], 0);

  const queued = live ? 4210 : 0;

  return {
    eventId: event.id,
    eventTitle: event.title,
    saleState,
    generatedAt: new Date().toISOString(),
    funnel: {
      lobby: 0,
      queued,
      admitted: live ? 350 : 0,
      holding: sum("held"),
      paid: sum("paid"),
      issued: sum("issued"),
    },
    admission: { ratePerSec: live ? 50 : 0, mode: "AUTO", queueDepth: queued },
    inventory,
    ai: { mode: "NORMAL", manualOverride: false, chatQueueLagSeconds: 2, budgetUsedRatio: 0.35 },
    sources: { waitingroom: "ok", ticketing: "ok", ai: "ok" },
    isSample: true,
  };
}

/** Nguon su kien cua war room: du lieu MAU (chua co API ops, EVF-60), khong phai su kien that. */
async function listSampleEvents(): Promise<EventDetailSnapshot[]> {
  return [...SAMPLE_EVENTS];
}

export async function getOpsWarRoom(eventId: string): Promise<OpsWarRoomSnapshot> {
  const override = overrides.get(eventId);
  if (override && "error" in override) {
    throw new OpsServiceError(override.error);
  }

  const events = await listSampleEvents();
  const event = events.find((e) => e.id === eventId);
  if (!event) {
    throw new OpsServiceError(404);
  }

  return { ...buildSample(event), ...(override ?? {}) };
}

export async function listOpsEvents(): Promise<OpsEventListItem[]> {
  const events = await listSampleEvents();
  return events.map((e) => ({
    eventId: e.id,
    eventTitle: e.title,
    saleState: toSaleState(e.state),
  }));
}
