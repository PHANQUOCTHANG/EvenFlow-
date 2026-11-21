/** Khu lam viec Moderator (EV-121): hang doi tham dinh, ho so va quyet dinh qua gateway.
 *
 * Danh tinh di bang Bearer -- gateway gan principal (vai tro moderator). Client KHONG gui
 * `moderator_id`: event service lay tu token, gui kem chi tao them cho de lech. */

import { ApiError, apiFetch, toApiError } from "@/lib/api-client";
import type { EventLifecycleState } from "@/lib/organizer-service";

export type AiVerdict = "AUTO_APPROVED" | "NEEDS_HUMAN" | "AUTO_REJECTED";

/** `type` RFC 7807 cua event service (services/event/internal/adapter/http/dto.go). */
export const MODERATION_PROBLEM = {
  overrideReasonRequired: "/problems/override-reason-required",
  alreadyDecided: "/problems/already-decided",
} as const;

export interface ModerationReview {
  id: string;
  aiScore: number;
  aiVerdict: AiVerdict;
  aiReasons: string[];
  humanVerdict: "APPROVED" | "REJECTED" | null;
  humanNote: string | null;
  createdAt: string;
  decidedAt: string | null;
}

export interface QueueItem {
  eventId: string;
  title: string;
  status: EventLifecycleState;
  venue: string;
  startsAt: string;
  saleStartAt: string;
  aiVerdict: AiVerdict | null;
  aiScore: number | null;
  aiReasons: string[];
  /** Luc AI tra ket qua = luc ho so vao hang doi. */
  submittedAt: string | null;
}

export interface ModerationTier {
  id: string;
  name: string;
  priceCents: number;
  currency: string;
  quota: number;
}

export interface ModerationEvent {
  id: string;
  organizerId: string;
  title: string;
  description: string;
  venue: string;
  startsAt: string;
  saleStartAt: string;
  status: EventLifecycleState;
  tiers: ModerationTier[];
}

export interface AuditEntry {
  id: number;
  actorType: "ai" | "human" | "system" | string;
  actorId: string | null;
  action: string;
  reason: string | null;
  createdAt: string;
}

export interface ModerationDetail {
  /** Noi dung GOC cua ban to chuc. */
  event: ModerationEvent;
  /** Phan tich AI -- chi la khuyen nghi, nguoi quyet la Moderator. */
  review: ModerationReview | null;
  axes: Record<string, number>;
  audit: AuditEntry[];
}

// ---------------------------------------------------------------- wire

interface WireReview {
  id: string;
  ai_score?: number | null;
  ai_verdict?: AiVerdict | null;
  ai_reasons?: string[] | null;
  human_verdict?: "APPROVED" | "REJECTED" | null;
  human_note?: string | null;
  created_at: string;
  decided_at?: string | null;
}

interface WireQueueItem {
  event_id: string;
  event_title: string;
  event_status: EventLifecycleState;
  venue?: string;
  starts_at: string;
  sale_start_at: string;
  review?: WireReview | null;
}

interface WireEvent {
  id: string;
  organizer_id?: string;
  title: string;
  description?: string;
  venue?: string;
  starts_at: string;
  sale_start_at: string;
  status: EventLifecycleState;
  tiers?: { id: string; name: string; price_cents: number; currency?: string; quota: number }[] | null;
}

interface WireAudit {
  id: number;
  actor_type: string;
  actor_id?: string | null;
  action: string;
  reason?: string | null;
  created_at: string;
}

interface WireDetail {
  event: WireEvent;
  review?: WireReview | null;
  ai_axes?: Record<string, number> | null;
  audit?: WireAudit[] | null;
}

function mapReview(r: WireReview): ModerationReview {
  return {
    id: r.id,
    aiScore: r.ai_score ?? 0,
    aiVerdict: r.ai_verdict ?? "NEEDS_HUMAN",
    aiReasons: r.ai_reasons ?? [],
    humanVerdict: r.human_verdict ?? null,
    humanNote: r.human_note ?? null,
    createdAt: r.created_at,
    decidedAt: r.decided_at ?? null,
  };
}

function mapEvent(w: WireEvent): ModerationEvent {
  return {
    id: w.id,
    organizerId: w.organizer_id ?? "",
    title: w.title,
    description: w.description ?? "",
    venue: w.venue ?? "",
    startsAt: w.starts_at,
    saleStartAt: w.sale_start_at,
    status: w.status,
    tiers: (w.tiers ?? []).map((t) => ({
      id: t.id,
      name: t.name,
      priceCents: t.price_cents,
      currency: t.currency ?? "VND",
      quota: t.quota,
    })),
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
    throw new ApiError("NETWORK_ERROR", 0, {}, "Không kết nối được máy chủ kiểm duyệt");
  }
  if (!res.ok) throw await toApiError(res);
  return res;
}

// ---------------------------------------------------------------- API

export async function getModerationQueue(): Promise<QueueItem[]> {
  const res = await call("/api/v1/moderation/queue");
  const rows = ((await res.json()) as WireQueueItem[] | null) ?? [];
  return rows.map((w) => {
    const rev = w.review ? mapReview(w.review) : null;
    return {
      eventId: w.event_id,
      title: w.event_title,
      status: w.event_status,
      venue: w.venue ?? "",
      startsAt: w.starts_at,
      saleStartAt: w.sale_start_at,
      aiVerdict: rev?.aiVerdict ?? null,
      aiScore: rev?.aiScore ?? null,
      aiReasons: rev?.aiReasons ?? [],
      submittedAt: rev?.createdAt ?? null,
    };
  });
}

/** null = khong ton tai (404). Loi khac nem ApiError (403 khong phai Moderator, ...). */
export async function getModerationDetail(eventId: string): Promise<ModerationDetail | null> {
  let res: Response;
  try {
    res = await call(`/api/v1/moderation/events/${encodeURIComponent(eventId)}`);
  } catch (e) {
    if (e instanceof ApiError && e.status === 404) return null;
    throw e;
  }
  const w = (await res.json()) as WireDetail;
  return {
    event: mapEvent(w.event),
    review: w.review ? mapReview(w.review) : null,
    axes: w.ai_axes ?? {},
    audit: (w.audit ?? []).map((a) => ({
      id: a.id,
      actorType: a.actor_type,
      actorId: a.actor_id ?? null,
      action: a.action,
      reason: a.reason ?? null,
      createdAt: a.created_at,
    })),
  };
}

export async function approveEvent(eventId: string, note: string): Promise<ModerationEvent> {
  const res = await call(`/api/v1/events/${encodeURIComponent(eventId)}/approve`, {
    method: "POST",
    body: JSON.stringify({ note }),
  });
  return mapEvent((await res.json()) as WireEvent);
}

export async function rejectEvent(eventId: string, reason: string): Promise<ModerationEvent> {
  const res = await call(`/api/v1/events/${encodeURIComponent(eventId)}/reject`, {
    method: "POST",
    body: JSON.stringify({ reason }),
  });
  return mapEvent((await res.json()) as WireEvent);
}

/** Duyet su kien dang REJECTED = lat nguoc quyet dinh truoc do (AI hoac nguoi): bat buoc ly do (BR-E5). */
export function isOverride(status: EventLifecycleState): boolean {
  return status === "REJECTED";
}

/** Ho so con cho quyet dinh: dang cho duyet, hoac da bi tu choi (co the lat nguoc). */
export function isDecidable(status: EventLifecycleState): boolean {
  return status === "PENDING_REVIEW" || status === "REJECTED";
}
