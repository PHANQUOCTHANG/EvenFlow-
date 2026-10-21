/** Organizer Service — Quản lý sự kiện qua API thật; chi tiết kiểm duyệt AI và doanh số vẫn là dữ liệu mẫu (EV-186, EVF-116).
 *
 * Tuân thủ nghiêm ngặt các Business Rules:
 * - BR-E1: Sự kiện chỉ chuyển SCHEDULED khi đã có >= 1 hạng vé với quota > 0 và price >= 0.
 * - BR-E2: sale_start_at phải cách thời điểm duyệt/hiện tại >= 30 phút để kịp pre-warm hạ tầng.
 * - BR-E3: Sau khi ON_SALE, CẤM giảm quota của hạng vé; chỉ được phép tăng.
 * - BR-E4: AI kiểm duyệt chấm 4 trục (nội dung cấm, sai lệch/lừa đảo, bản quyền hình ảnh, thông tin thiếu).
 *          Điểm rủi ro >= 0.8 -> AUTO_REJECTED; 0.3–0.8 -> NEEDS_HUMAN; < 0.3 -> AUTO_APPROVED.
 * - BR-E5: Mọi quyết định ghi nhận audit_log kèm lý do minh bạch, Moderator có thể lật ngược.
 */

import { ApiError, apiFetch, toApiError } from "@/lib/api-client";
import type { EventStatus } from "@/lib/types/event";

export type EventLifecycleState = EventStatus;

export interface TicketTierConfig {
  id: string;
  name: string;
  price: number;
  quota: number;
  description?: string;
}

export type ModerationAxis =
  | "PROHIBITED_CONTENT"
  | "MISLEADING_SCAM"
  | "IMAGE_COPYRIGHT"
  | "MISSING_INFO";

export interface ModerationAxisResult {
  axis: ModerationAxis;
  title: string;
  riskScore: number; // 0.0 -> 1.0
  passed: boolean;
  notes?: string;
}

export type ModerationDecision =
  | "AUTO_APPROVED"
  | "NEEDS_HUMAN"
  | "AUTO_REJECTED"
  | "MODERATOR_OVERRIDE";

export interface ModerationDetail {
  overallRiskScore: number;
  decision: ModerationDecision;
  scannedAt: string;
  reasons: string[];
  moderatorNotes?: string;
  axes: ModerationAxisResult[];
}

export interface OrganizerEvent {
  id: string;
  slug: string;
  title: string;
  description: string;
  venue: string;
  startsAt: string;
  saleStartAt: string;
  saleEndAt?: string;
  state: EventLifecycleState;
  organizerId: string;
  tiers: TicketTierConfig[];
  /** Backend chưa có endpoint chi tiết kiểm duyệt cho organizer: API không bao giờ điền trường này. */
  moderation?: ModerationDetail;
  createdAt: string;
  updatedAt: string;
}

export interface TierSalesDetail {
  tierId: string;
  name: string;
  price: number;
  quota: number;
  sold: number;
  activeHolds: number;
  revenue: number;
  fillRatePercentage: number;
}

export interface RealtimeSalesSnapshot {
  /** Luôn true: chưa có API doanh số thật. UI phải gắn nhãn "Dữ liệu mẫu". */
  isSample: true;
  eventId: string;
  timestamp: string;
  grossRevenue: number;
  totalTicketsSold: number;
  totalQuota: number;
  overallFillRatePercentage: number;
  paidOrdersCount: number;
  activeHoldsCount: number;
  tierSales: TierSalesDetail[];
  postSaleReport?: {
    averageOrderValue: number;
    peakSalesVelocityTps: number;
    paymentBreakdown: {
      vnpay: number;
      momo: number;
      card: number;
    };
  };
}

export interface CreateEventInput {
  title: string;
  slug?: string;
  description: string;
  venue: string;
  startsAt: string;
  saleStartAt: string;
  tiers: TicketTierConfig[];
  submitForReview?: boolean;
}

export interface ValidationResult {
  isValid: boolean;
  error?: string;
}

/** BR-E2: Kiểm tra sale_start_at phải cách thời điểm hiện tại/duyệt tối thiểu 30 phút */
export function validateEventSchedule(
  saleStartAtInput: string | Date,
  nowInput: Date = new Date(),
): ValidationResult & { diffMinutes?: number } {
  const saleStart = typeof saleStartAtInput === "string" ? new Date(saleStartAtInput) : saleStartAtInput;
  const now = nowInput;

  if (isNaN(saleStart.getTime())) {
    return {
      isValid: false,
      error: "Thời gian mở bán (sale_start_at) không hợp lệ.",
    };
  }

  const diffMs = saleStart.getTime() - now.getTime();
  const diffMinutes = Math.floor(diffMs / (1000 * 60));

  if (diffMinutes < 30) {
    return {
      isValid: false,
      diffMinutes,
      error: `Thời gian mở bán phải cách thời điểm hiện tại tối thiểu 30 phút (hiện cách ${diffMinutes} phút) theo quy định BR-E2 để hệ thống kịp pre-warm hạ tầng.`,
    };
  }

  return {
    isValid: true,
    diffMinutes,
  };
}

/** BR-E1: Phải có ít nhất 1 hạng vé với quota > 0 và price >= 0 */
export function validateTiers(tiers: TicketTierConfig[]): ValidationResult {
  if (!tiers || tiers.length === 0) {
    return {
      isValid: false,
      error: "Sự kiện phải có ít nhất 1 hạng vé để có thể lên lịch hoặc gửi kiểm duyệt (BR-E1).",
    };
  }

  for (let i = 0; i < tiers.length; i++) {
    const tier = tiers[i];
    if (!tier.name || tier.name.trim().length === 0) {
      return {
        isValid: false,
        error: `Hạng vé #${i + 1} chưa có tên gọi.`,
      };
    }
    if (typeof tier.price !== "number" || isNaN(tier.price) || tier.price < 0) {
      return {
        isValid: false,
        error: `Giá vé của hạng "${tier.name}" phải lớn hơn hoặc bằng 0 VND (BR-E1).`,
      };
    }
    if (typeof tier.quota !== "number" || isNaN(tier.quota) || tier.quota <= 0) {
      return {
        isValid: false,
        error: `Số lượng vé (quota) của hạng "${tier.name}" phải lớn hơn 0 (BR-E1).`,
      };
    }
  }

  return { isValid: true };
}

/** BR-E3: Sau khi ON_SALE, cấm giảm quota của hạng vé; chỉ được phép tăng */
export function validateTierQuotaUpdate(
  oldQuota: number,
  newQuota: number,
  eventState: EventLifecycleState,
): ValidationResult {
  if ((eventState === "ON_SALE" || eventState === "SOLD_OUT") && newQuota < oldQuota) {
    return {
      isValid: false,
      error: `Sự kiện ${eventState === "ON_SALE" ? "đang mở bán" : "đã hết vé"} (${eventState}). Tuyệt đối cấm giảm số lượng vé của hạng vé từ ${oldQuota} xuống ${newQuota} (BR-E3); chỉ được phép tăng quota.`,
    };
  }
  return { isValid: true };
}

/** BR-E4: Đánh giá điểm rủi ro kiểm duyệt 4 trục của AI */
export function evaluateAiScanRisk(axes: ModerationAxisResult[]): {
  overallRiskScore: number;
  decision: ModerationDecision;
  reasons: string[];
  passed: boolean;
} {
  if (!axes || axes.length === 0) {
    return {
      overallRiskScore: 0,
      decision: "AUTO_APPROVED",
      reasons: [],
      passed: true,
    };
  }

  // Điểm rủi ro tổng hợp lấy theo giá trị lớn nhất trong 4 trục
  const maxScore = Math.max(...axes.map((a) => a.riskScore));
  const roundedScore = Math.round(maxScore * 100) / 100;
  const reasons: string[] = [];

  for (const axis of axes) {
    if (axis.riskScore >= 0.3 && axis.notes) {
      reasons.push(`${axis.title}: ${axis.notes}`);
    }
  }

  let decision: ModerationDecision;
  if (roundedScore >= 0.8) {
    decision = "AUTO_REJECTED";
  } else if (roundedScore >= 0.3) {
    decision = "NEEDS_HUMAN";
  } else {
    decision = "AUTO_APPROVED";
  }

  return {
    overallRiskScore: roundedScore,
    decision,
    reasons,
    passed: decision === "AUTO_APPROVED",
  };
}

/** Trục kiểm duyệt mặc định với tên tiếng Việt */
export const DEFAULT_MODERATION_AXES_METADATA: Record<ModerationAxis, string> = {
  PROHIBITED_CONTENT: "Nội dung cấm & bạo lực / phản cảm",
  MISLEADING_SCAM: "Thông tin sai lệch & dấu hiệu lừa đảo",
  IMAGE_COPYRIGHT: "Bản quyền hình ảnh & nhận diện thương hiệu",
  MISSING_INFO: "Thông tin pháp lý & quy định tổ chức",
};

// ---------------------------------------------------------------------------
// Goi API that qua gateway (AC-19..AC-23). Khong con kho mock.
// ---------------------------------------------------------------------------

interface WireTier {
  id: string;
  name: string;
  price_cents: number;
  quota: number;
}

interface WireEvent {
  id: string;
  organizer_id?: string;
  slug: string;
  title: string;
  description?: string;
  venue?: string;
  starts_at: string;
  sale_start_at: string;
  sale_end_at?: string | null;
  status: EventLifecycleState;
  created_at?: string;
  updated_at?: string;
  tiers?: WireTier[] | null;
}

/** Trang thai cho phep PUT /events/{id} (contracts muc 2). */
export const EDITABLE_STATES: readonly EventLifecycleState[] = ["DRAFT", "SCHEDULED", "REJECTED"];
/** Trang thai cho phep nop duyet (DRAFT/REJECTED -> PENDING_REVIEW). */
export const SUBMITTABLE_STATES: readonly EventLifecycleState[] = ["DRAFT", "REJECTED"];

export function canEditEvent(state: EventLifecycleState): boolean {
  return EDITABLE_STATES.includes(state);
}

/** ON_SALE / SOLD_OUT: chi duoc TANG quota hang ve (PUT .../tiers/{id} {new_quota}); cac truong su kien chi doc.
 *  Backend (event.go) cho tang quota khi SOLD_OUT va mo ban lai (G-2). */
export function canIncreaseTierQuota(state: EventLifecycleState): boolean {
  return state === "ON_SALE" || state === "SOLD_OUT";
}

/** Co loi vao trang sua (day du hoac chi tang quota). */
export function canOpenEventEditor(state: EventLifecycleState): boolean {
  return canEditEvent(state) || canIncreaseTierQuota(state);
}

export function canSubmitEvent(state: EventLifecycleState): boolean {
  return SUBMITTABLE_STATES.includes(state);
}

export function canCancelEvent(state: EventLifecycleState): boolean {
  return state !== "COMPLETED" && state !== "CANCELLED";
}

function mapTier(t: WireTier): TicketTierConfig {
  return { id: t.id, name: t.name, price: t.price_cents, quota: t.quota };
}

function mapEvent(w: WireEvent): OrganizerEvent {
  return {
    id: w.id,
    slug: w.slug,
    title: w.title,
    description: w.description ?? "",
    venue: w.venue ?? "",
    startsAt: w.starts_at,
    saleStartAt: w.sale_start_at,
    saleEndAt: w.sale_end_at ?? undefined,
    state: w.status,
    organizerId: w.organizer_id ?? "",
    // Danh sach GET /events khong kem hang ve; chi chi tiet moi co.
    tiers: (w.tiers ?? []).map(mapTier),
    createdAt: w.created_at ?? "",
    updatedAt: w.updated_at ?? "",
  };
}

async function call(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers);
  if (init.body !== undefined) headers.set("Content-Type", "application/json");
  let res: Response;
  try {
    res = await apiFetch(path, { ...init, headers });
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError("NETWORK_ERROR", 0, {}, "Không kết nối được máy chủ sự kiện");
  }
  if (!res.ok) throw await toApiError(res);
  return res;
}

const jsonBody = (value: unknown) => JSON.stringify(value);

function startsAfterSale(startsAt: string, saleStartAt: string): boolean {
  const s = new Date(startsAt).getTime();
  const t = new Date(saleStartAt).getTime();
  return !isNaN(s) && !isNaN(t) && s > t;
}

/** Backend lang le bo qua PUT khi starts_at <= sale_start_at, nen kiem tra truoc o client. */
export function validateEventDates(startsAt: string, saleStartAt: string): ValidationResult {
  if (!startsAfterSale(startsAt, saleStartAt)) {
    return {
      isValid: false,
      error: "Thời gian diễn ra sự kiện phải sau thời gian bắt đầu mở bán.",
    };
  }
  return { isValid: true };
}

/** Lay danh sach su kien cua organizer (GET /api/v1/events, khong kem hang ve). */
export async function getOrganizerEvents(): Promise<OrganizerEvent[]> {
  const res = await call("/api/v1/events", { method: "GET" });
  const data = (await res.json()) as WireEvent[] | null;
  return Array.isArray(data) ? data.map(mapEvent) : [];
}

/** Lay chi tiet su kien theo ID (co hang ve). 404 hoac id khong hop le -> null. */
export async function getOrganizerEventById(id: string): Promise<OrganizerEvent | null> {
  try {
    const res = await call(`/api/v1/events/${encodeURIComponent(id)}`, { method: "GET" });
    return mapEvent((await res.json()) as WireEvent);
  } catch (e) {
    if (e instanceof ApiError && (e.status === 404 || e.status === 400)) return null;
    throw e;
  }
}

export interface CreateEventResult {
  event: OrganizerEvent;
  /** Buoc loi sau khi ban nhap da duoc tao; undefined = thanh cong tron ven. */
  failedStep?: "tiers" | "submit";
  error?: string;
}

const errMsg = (e: unknown) => (e instanceof Error ? e.message : String(e));

/** Them mot hang ve (POST /events/{id}/tiers). */
export async function addTier(eventId: string, tier: TicketTierConfig): Promise<TicketTierConfig> {
  const check = validateTiers([tier]);
  if (!check.isValid) throw new Error(check.error);
  const res = await call(`/api/v1/events/${encodeURIComponent(eventId)}/tiers`, {
    method: "POST",
    body: jsonBody({ name: tier.name.trim(), price_cents: tier.price, quota: tier.quota }),
  });
  return mapTier((await res.json()) as WireTier);
}

/** POST /events -> POST tiers (tung hang) -> POST submit (tuy chon). Loi sau buoc 1 tra ve ban nhap kem failedStep. */
export async function createOrganizerEvent(input: CreateEventInput): Promise<CreateEventResult> {
  const tierValidation = validateTiers(input.tiers);
  if (!tierValidation.isValid) throw new Error(tierValidation.error);
  const dates = validateEventDates(input.startsAt, input.saleStartAt);
  if (!dates.isValid) throw new Error(dates.error);
  if (input.submitForReview) {
    const scheduleValidation = validateEventSchedule(input.saleStartAt);
    if (!scheduleValidation.isValid) throw new Error(scheduleValidation.error);
  }

  const body: Record<string, unknown> = {
    title: input.title,
    description: input.description,
    venue: input.venue,
    starts_at: input.startsAt,
    sale_start_at: input.saleStartAt,
  };
  if (input.slug) body.slug = input.slug;

  const created = await call("/api/v1/events", { method: "POST", body: jsonBody(body) });
  let event = mapEvent((await created.json()) as WireEvent);

  const tiers: TicketTierConfig[] = [];
  for (const tier of input.tiers) {
    try {
      tiers.push(await addTier(event.id, tier));
    } catch (e) {
      return { event: { ...event, tiers }, failedStep: "tiers", error: errMsg(e) };
    }
  }
  event = { ...event, tiers };

  if (input.submitForReview) {
    try {
      event = { ...(await submitOrganizerEvent(event.id)), tiers };
    } catch (e) {
      return { event, failedStep: "submit", error: errMsg(e) };
    }
  }
  return { event };
}

/** Cap nhat su kien: luon PUT day du cac truong (ghep voi su kien hien tai). Khong gui slug/tiers. */
export async function updateOrganizerEvent(
  id: string,
  input: Partial<CreateEventInput>,
): Promise<OrganizerEvent> {
  const existing = await getOrganizerEventById(id);
  if (!existing) throw new Error(`Không tìm thấy sự kiện với ID: ${id}`);
  if (!canEditEvent(existing.state)) {
    throw new Error(`Sự kiện ở trạng thái ${existing.state} không thể chỉnh sửa.`);
  }

  const merged = {
    title: input.title ?? existing.title,
    description: input.description ?? existing.description,
    venue: input.venue ?? existing.venue,
    startsAt: input.startsAt ?? existing.startsAt,
    saleStartAt: input.saleStartAt ?? existing.saleStartAt,
  };
  const dates = validateEventDates(merged.startsAt, merged.saleStartAt);
  if (!dates.isValid) throw new Error(dates.error);

  const body: Record<string, unknown> = {
    title: merged.title,
    description: merged.description,
    venue: merged.venue,
    starts_at: merged.startsAt,
    sale_start_at: merged.saleStartAt,
  };
  if (existing.saleEndAt) body.sale_end_at = existing.saleEndAt;

  const res = await call(`/api/v1/events/${encodeURIComponent(id)}`, {
    method: "PUT",
    body: jsonBody(body),
  });
  return { ...mapEvent((await res.json()) as WireEvent), tiers: existing.tiers };
}

/** Doi quota hang ve hien co (PUT .../tiers/{tierId} {new_quota}); chan giam quota khi ON_SALE (BR-E3). */
export async function updateTierQuota(
  eventId: string,
  tierId: string,
  newQuota: number,
): Promise<TicketTierConfig> {
  const event = await getOrganizerEventById(eventId);
  if (!event) throw new Error(`Không tìm thấy sự kiện với ID: ${eventId}`);
  const tier = event.tiers.find((t) => t.id === tierId);
  if (!tier) throw new Error(`Không tìm thấy hạng vé với ID: ${tierId}`);
  if (typeof newQuota !== "number" || isNaN(newQuota) || newQuota <= 0) {
    throw new Error(`Số lượng vé (quota) của hạng "${tier.name}" phải lớn hơn 0 (BR-E1).`);
  }
  const check = validateTierQuotaUpdate(tier.quota, newQuota, event.state);
  if (!check.isValid) throw new Error(check.error);

  const res = await call(
    `/api/v1/events/${encodeURIComponent(eventId)}/tiers/${encodeURIComponent(tierId)}`,
    { method: "PUT", body: jsonBody({ new_quota: newQuota }) },
  );
  return mapTier((await res.json()) as WireTier);
}

/** Nop duyet (DRAFT/REJECTED -> PENDING_REVIEW). 422 -> Error mang problem detail. */
export async function submitOrganizerEvent(id: string): Promise<OrganizerEvent> {
  const res = await call(`/api/v1/events/${encodeURIComponent(id)}/submit`, { method: "POST" });
  return mapEvent((await res.json()) as WireEvent);
}

/** Huy su kien (moi trang thai tru COMPLETED/CANCELLED). */
export async function cancelOrganizerEvent(id: string): Promise<OrganizerEvent> {
  const res = await call(`/api/v1/events/${encodeURIComponent(id)}/cancel`, { method: "POST" });
  return mapEvent((await res.json()) as WireEvent);
}

/** DU LIEU MAU: chua co endpoint doanh so cho organizer (contracts muc 7). Luon gan isSample = true.
 *
 * So lieu duoc suy tat dinh tu trang thai va quota that cua su kien; khong phai doanh so that. */
export async function getRealtimeSales(eventId: string): Promise<RealtimeSalesSnapshot> {
  const event = await getOrganizerEventById(eventId);
  if (!event) {
    throw new Error(`Sự kiện ${eventId} không tồn tại.`);
  }

  const now = new Date();
  const timestamp = now.toLocaleTimeString("vi-VN", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });

  let soldRatio = 0;
  let activeHolds = 0;
  if (event.state === "ON_SALE") {
    soldRatio = 0.72;
    activeHolds = 48;
  } else if (
    event.state === "SOLD_OUT" ||
    event.state === "CLOSED" ||
    event.state === "COMPLETED"
  ) {
    soldRatio = 1.0;
  }

  const tierSales: TierSalesDetail[] = event.tiers.map((t) => {
    const sold = Math.floor(t.quota * soldRatio);
    const holds = Math.min(activeHolds, Math.floor(t.quota * 0.05));
    const revenue = sold * t.price;
    const fillRatePercentage = t.quota > 0 ? Math.round((sold / t.quota) * 100) : 0;

    return {
      tierId: t.id,
      name: t.name,
      price: t.price,
      quota: t.quota,
      sold,
      activeHolds: holds,
      revenue,
      fillRatePercentage,
    };
  });

  const totalTicketsSold = tierSales.reduce((acc, curr) => acc + curr.sold, 0);
  const totalQuota = tierSales.reduce((acc, curr) => acc + curr.quota, 0);
  const grossRevenue = tierSales.reduce((acc, curr) => acc + curr.revenue, 0);
  const overallFillRatePercentage =
    totalQuota > 0 ? Math.round((totalTicketsSold / totalQuota) * 100) : 0;
  const paidOrdersCount = Math.floor(totalTicketsSold / 1.8);

  return {
    isSample: true,
    eventId: event.id,
    timestamp,
    grossRevenue,
    totalTicketsSold,
    totalQuota,
    overallFillRatePercentage,
    paidOrdersCount,
    activeHoldsCount: activeHolds,
    tierSales,
    postSaleReport: {
      averageOrderValue: paidOrdersCount > 0 ? Math.round(grossRevenue / paidOrdersCount) : 0,
      peakSalesVelocityTps: 84.5,
      paymentBreakdown: {
        vnpay: Math.round(grossRevenue * 0.55),
        momo: Math.round(grossRevenue * 0.3),
        card: Math.round(grossRevenue * 0.15),
      },
    },
  };
}
