/** Ops Service — war room cho Ops (EV-187, EVF-117) va lenh ghi de admit rate (EV-147, EVF-60).
 *
 * getOpsWarRoom ghep MOT snapshot o client tu 4 nguon goi song song qua apiFetch (contracts ops-warroom §3-§6):
 *   GET /api/v1/events/{id}            (cong khai: tieu de, trang thai, quyet dinh 404)
 *   GET /v1/ops/events/{id}/queue      (waitingroom)
 *   GET /v1/ops/events/{id}/inventory  (ticketing)
 *   GET /v1/ops/ai/mode                (ai-worker)
 * Mot nguon hong -> nguon do `unavailable`, cac so con lai van song. Snapshot luon isSample = false: khong con du
 * lieu mau nao trong module nay (AC-28). Token nam o trinh duyet, nen chi goi tu client. */

import { ApiError, apiFetch } from "@/lib/api-client";
import { getAllPublicEvents } from "@/lib/event-service";

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

export interface OpsCheckoutHealth {
  p99Ms: number;
  errorRate: number;
  dbPoolUsage: number;
  pods: number;
}

export interface OpsWarRoomSnapshot {
  eventId: string;
  eventTitle: string;
  saleState: OpsSaleState;
  /** ISO, gio server cua nguon CU NHAT trong cac nguon tra loi duoc. */
  generatedAt: string;
  funnel: {
    lobby: number;
    queued: number;
    admitted: number;
    holding: number;
    paid: number;
    issued: number;
  };
  admission: {
    ratePerSec: number;
    mode: "AUTO" | "MANUAL";
    queueDepth: number;
    /** Nhip Ops dat tay; null khi AUTO. */
    overrideRatePerSec: number | null;
    /** Ghi de voi nhip 0 = tam dung khan cap. */
    paused: boolean;
    soldOut: boolean;
    stockZero: boolean;
    leaderPresent: boolean;
  };
  /** Suc khoe checkout (loadsignal); null khi khong co tin hieu. */
  checkout: OpsCheckoutHealth | null;
  inventory: OpsTierInventory[];
  ai: {
    mode: DegradationMode;
    manualOverride: boolean;
    chatQueueLagSeconds: number;
    budgetUsedRatio: number;
  };
  sources: { waitingroom: SourceHealth; ticketing: SourceHealth; ai: SourceHealth };
  /** true = du lieu mau, UI phai gan nhan. Duong that luon false. */
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

/** 401 = can dang nhap lai, 403 = khong co quyen, 404 = khong co su kien, 503 = khong tai duoc. */
export class OpsServiceError extends Error {
  status: number;

  constructor(status: number, message?: string) {
    super(message ?? defaultMessage(status));
    this.name = "OpsServiceError";
    this.status = status;
  }
}

function defaultMessage(status: number): string {
  if (status === 401) return "Phiên đăng nhập đã hết, vui lòng đăng nhập lại.";
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

function toSaleState(status: string): OpsSaleState {
  switch (status) {
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

/* ------------------------------ wire (contracts §3.1, §4.1, §5.1) ------------------------------ */

interface WireEventDetail {
  id: string;
  title: string;
  status: string;
}

interface WireQueue {
  generated_at: string;
  lobby: number;
  queued: number;
  admitted_active: number;
  admit_rate_per_sec: number;
  override: { active: boolean; rate_per_sec: number | null };
  paused: boolean;
  sold_out: boolean;
  stock_zero: boolean;
  controller: { leader_present: boolean; epoch: number };
  checkout: {
    signal: string;
    pods: number;
    p99_ms: number | null;
    error_rate: number | null;
    db_pool_usage: number | null;
  };
}

interface WireInventoryTier {
  ticket_type_id: string;
  name: string;
  quota: number;
  available: number;
  held: number;
  paid: number;
  issued: number;
}

interface WireInventory {
  generated_at: string;
  tiers: WireInventoryTier[] | null;
}

interface WireAiMode {
  mode: DegradationMode;
  manual_override: string | null;
  signals: { chat_queue_lag_seconds: number; budget_used_ratio: number; gemini_circuit_open: boolean };
  generated_at: string;
}

/** Loi cua mot nguon: status HTTP (0 = mang / body hong), auth = phien da mat. */
class SourceError extends Error {
  readonly status: number;
  readonly auth: boolean;

  constructor(status: number, auth = false) {
    super(`source ${status}`);
    this.status = status;
    this.auth = auth;
  }
}

async function getJson<T>(path: string, auth: boolean): Promise<T> {
  let res: Response;
  try {
    res = await apiFetch(path, { method: "GET", auth, cache: "no-store" });
  } catch (e) {
    if (e instanceof ApiError && e.code === "AUTH_REQUIRED") throw new SourceError(401, true);
    throw new SourceError(0);
  }
  if (!res.ok) throw new SourceError(res.status);
  try {
    return (await res.json()) as T;
  } catch {
    throw new SourceError(0);
  }
}

function failure(r: PromiseSettledResult<unknown>): SourceError | null {
  if (r.status === "fulfilled") return null;
  return r.reason instanceof SourceError ? r.reason : new SourceError(0);
}

const sum = (tiers: OpsTierInventory[], key: "held" | "paid" | "issued") =>
  tiers.reduce((acc, t) => acc + t[key], 0);

export async function getOpsWarRoom(eventId: string): Promise<OpsWarRoomSnapshot> {
  const id = encodeURIComponent(eventId);
  const [eventR, queueR, invR, aiR] = await Promise.allSettled([
    getJson<WireEventDetail>(`/api/v1/events/${id}`, false),
    getJson<WireQueue>(`/v1/ops/events/${id}/queue`, true),
    getJson<WireInventory>(`/v1/ops/events/${id}/inventory`, true),
    getJson<WireAiMode>("/v1/ops/ai/mode", true),
  ]);
  const failures = [eventR, queueR, invR, aiR].map(failure);
  const [eventErr, queueErr, invErr, aiErr] = failures;

  // Thu tu quyet dinh (AC-22): phien mat -> 401; su kien khong ton tai -> 404; mot nguon tu choi quyen -> 403
  // (khong coi la "unavailable"); su kien loi khac hoac ca ba nguon hong -> 503.
  if (failures.some((f) => f?.auth)) throw new OpsServiceError(401);
  if (eventErr?.status === 404 || eventErr?.status === 400) throw new OpsServiceError(404);
  if (failures.some((f) => f?.status === 403)) throw new OpsServiceError(403);
  if (eventR.status !== "fulfilled") throw new OpsServiceError(503);
  if (queueErr && invErr && aiErr) throw new OpsServiceError(503);

  const event = eventR.value;
  const saleState = toSaleState(event.status);
  const queue = queueR.status === "fulfilled" ? queueR.value : null;
  const inv = invR.status === "fulfilled" ? invR.value : null;
  const ai = aiR.status === "fulfilled" ? aiR.value : null;

  const inventory: OpsTierInventory[] = (inv?.tiers ?? []).map((t) => ({
    tierId: t.ticket_type_id,
    name: t.name,
    quota: t.quota,
    held: t.held,
    paid: t.paid,
    issued: t.issued,
    available: t.available,
  }));

  const overrideActive = queue?.override.active === true;
  const leaderPresent = queue?.controller.leader_present === true;
  const co = queue?.checkout;
  const checkout: OpsCheckoutHealth | null =
    co && co.signal === "ok" && co.p99_ms !== null && co.error_rate !== null && co.db_pool_usage !== null
      ? { p99Ms: co.p99_ms, errorRate: co.error_rate, dbPoolUsage: co.db_pool_usage, pods: co.pods }
      : null;

  let waitingroom: SourceHealth = queue ? "ok" : "unavailable";
  if (queue && !leaderPresent && saleState === "ON_SALE") waitingroom = "stale";

  const times = [queue?.generated_at, inv?.generated_at, ai?.generated_at]
    .map((t) => (t ? Date.parse(t) : Number.NaN))
    .filter((t) => !Number.isNaN(t));
  const generatedAt = new Date(times.length > 0 ? Math.min(...times) : Date.now()).toISOString();

  return {
    eventId: event.id,
    eventTitle: event.title,
    saleState,
    generatedAt,
    funnel: {
      lobby: queue?.lobby ?? 0,
      queued: queue?.queued ?? 0,
      admitted: queue?.admitted_active ?? 0,
      holding: sum(inventory, "held"),
      paid: sum(inventory, "paid"),
      issued: sum(inventory, "issued"),
    },
    admission: {
      ratePerSec: queue?.admit_rate_per_sec ?? 0,
      mode: overrideActive ? "MANUAL" : "AUTO",
      queueDepth: queue?.queued ?? 0,
      overrideRatePerSec: overrideActive ? (queue?.override.rate_per_sec ?? null) : null,
      paused: queue?.paused === true,
      soldOut: queue?.sold_out === true,
      stockZero: queue?.stock_zero === true,
      leaderPresent,
    },
    checkout,
    inventory,
    ai: {
      mode: ai?.mode ?? "NORMAL",
      manualOverride: ai ? ai.manual_override != null : false,
      chatQueueLagSeconds: ai?.signals.chat_queue_lag_seconds ?? 0,
      budgetUsedRatio: ai?.signals.budget_used_ratio ?? 0,
    },
    sources: {
      waitingroom,
      ticketing: inv ? "ok" : "unavailable",
      ai: ai ? "ok" : "unavailable",
    },
    isSample: false,
  };
}

/** Danh sach su kien cho war room: danh sach cong khai (SCHEDULED / ON_SALE / SOLD_OUT, contracts §6). */
export async function listOpsEvents(): Promise<OpsEventListItem[]> {
  const events = await getAllPublicEvents();
  return events.map((e) => ({ eventId: e.id, eventTitle: e.title, saleState: toSaleState(e.state) }));
}

/* ------------------------------ ghi de admit rate (contracts §3.2, §3.3) ------------------------------ */

export const MAX_OVERRIDE_RATE = 10_000;
export const MIN_REASON_LENGTH = 3;
export const MAX_REASON_LENGTH = 200;

/** Nhip ghi de hop le: so nguyen 0..10000 (CR-1: so le bi tu choi). */
export function isValidOverrideRate(rate: number): boolean {
  return Number.isInteger(rate) && rate >= 0 && rate <= MAX_OVERRIDE_RATE;
}

/** Ly do da cat khoang trang neu dai 3..200 ky tu, nguoc lai null. */
export function normalizeOverrideReason(reason: string): string | null {
  const r = (reason ?? "").trim();
  const n = Array.from(r).length;
  return n >= MIN_REASON_LENGTH && n <= MAX_REASON_LENGTH ? r : null;
}

export type OpsWriteErrorCode = "INVALID" | "FORBIDDEN" | "NOT_FOUND" | "UNAVAILABLE";

const WRITE_MESSAGE: Record<OpsWriteErrorCode, string> = {
  INVALID: "Tốc độ phải là số nguyên 0–10000 và lý do dài 3–200 ký tự.",
  FORBIDDEN: "Không có quyền thực hiện thao tác này.",
  NOT_FOUND: "Sự kiện chưa có phòng chờ.",
  UNAVAILABLE: "Không gửi được lệnh, vui lòng thử lại.",
};

/** Loi lenh ghi. Thong diep luon la cau co dinh phia client, khong bao gio la `detail` cua server. */
export class OpsWriteError extends Error {
  readonly code: OpsWriteErrorCode;

  constructor(code: OpsWriteErrorCode) {
    super(WRITE_MESSAGE[code]);
    this.name = "OpsWriteError";
    this.code = code;
  }
}

export interface AdmitOverrideResult {
  active: boolean;
  ratePerSec: number | null;
  paused: boolean;
  audit: "recorded" | "failed";
}

interface WireOverrideResult {
  override: { active: boolean; rate_per_sec: number | null };
  paused: boolean;
  audit: "recorded" | "failed";
}

async function sendOverride(eventId: string, method: "PUT" | "DELETE", body: unknown): Promise<AdmitOverrideResult> {
  let res: Response;
  try {
    res = await apiFetch(`/v1/ops/events/${encodeURIComponent(eventId)}/admit-override`, {
      method,
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  } catch (e) {
    // Phien mat: de nguyen ApiError(AUTH_REQUIRED) cho tang tren dua nguoi dung ve dang nhap.
    if (e instanceof ApiError && e.code === "AUTH_REQUIRED") throw e;
    throw new OpsWriteError("UNAVAILABLE");
  }
  if (!res.ok) {
    if (res.status === 400) throw new OpsWriteError("INVALID");
    if (res.status === 401 || res.status === 403) throw new OpsWriteError("FORBIDDEN");
    if (res.status === 404) throw new OpsWriteError("NOT_FOUND");
    throw new OpsWriteError("UNAVAILABLE");
  }
  let w: WireOverrideResult;
  try {
    w = (await res.json()) as WireOverrideResult;
  } catch {
    throw new OpsWriteError("UNAVAILABLE");
  }
  return {
    active: w.override.active,
    ratePerSec: w.override.rate_per_sec,
    paused: w.paused,
    audit: w.audit === "failed" ? "failed" : "recorded",
  };
}

/** Dat ghi de nhip admit (rate 0 = tam dung khan cap). Kiem tra o client truoc moi request (AC-24). */
export async function setAdmitOverride(eventId: string, ratePerSec: number, reason: string): Promise<AdmitOverrideResult> {
  const r = normalizeOverrideReason(reason);
  if (!isValidOverrideRate(ratePerSec) || r === null) throw new OpsWriteError("INVALID");
  return sendOverride(eventId, "PUT", { rate_per_sec: ratePerSec, reason: r });
}

/** Bo ghi de, tra quyen cho bo tu dieu chinh. */
export async function clearAdmitOverride(eventId: string, reason: string): Promise<AdmitOverrideResult> {
  const r = normalizeOverrideReason(reason);
  if (r === null) throw new OpsWriteError("INVALID");
  return sendOverride(eventId, "DELETE", { reason: r });
}
