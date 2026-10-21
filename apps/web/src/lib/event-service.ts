/** Event service client -- doc su kien cong khai that qua gateway (AC-16, AC-17, AC-18, AC-19).
 *
 * Chi goi endpoint CONG KHAI, khong dung phien dang nhap (an toan khi chay o server/ISR).
 * Hinh dang ket qua (contracts.md muc 9):
 *  - getEventBySlug / getEventById -> EventDetailSnapshot | null (null khi 404; 400 id khong phai UUID cung la null),
 *    loi khac (mang, 5xx, timeout) reject bang ApiError.
 *  - getAllPublicEvents -> mang, reject khi loi.
 * BR-A4: ton kho chi o dang khoang (AVAILABLE / FEW_LEFT / SOLD_OUT / UNKNOWN).
 * KHONG bao gio tra du lieu mau/mock; khong fetch trong luc `next build`. */

import { ApiError, apiFetch, toApiError } from "@/lib/api-client";
import type { EventDetailSnapshot, EventStatus, TicketTierSnapshot, TierAvailability } from "@/lib/types/event";

export type { EventDetailSnapshot, EventStatus, TicketTierSnapshot, TierAvailability } from "@/lib/types/event";

/** Tuy chon cache cho fetch phia server (Next ISR); bo qua o trinh duyet. */
export interface EventFetchOptions {
  revalidate?: number;
}

const TIMEOUT_MS = 5000;

interface WireTier {
  id: string;
  name: string;
  price_cents: number;
  currency?: string;
}

interface WireEvent {
  id: string;
  slug: string;
  title: string;
  description?: string;
  venue?: string;
  starts_at: string;
  sale_start_at: string;
  sale_end_at?: string | null;
  status: EventStatus;
  max_per_order?: number;
  tiers?: WireTier[] | null;
}

interface WireAvailability {
  tiers?: Array<{ ticket_type_id: string; availability: string }> | null;
}

function guardBuildPhase() {
  // Khong goi backend luc build: backend khong duoc phep la dieu kien de build (plan 0, B1).
  if (process.env.NEXT_PHASE === "phase-production-build") {
    throw new ApiError("BUILD_PHASE", 0, {}, "Khong goi API trong luc next build");
  }
}

function timeoutSignal(): AbortSignal | undefined {
  return typeof AbortSignal !== "undefined" && typeof AbortSignal.timeout === "function"
    ? AbortSignal.timeout(TIMEOUT_MS)
    : undefined;
}

async function getPublic(path: string, opts: EventFetchOptions = {}): Promise<Response> {
  guardBuildPhase();
  try {
    return await apiFetch(path, {
      method: "GET",
      auth: false,
      signal: timeoutSignal(),
      ...(opts.revalidate !== undefined ? { next: { revalidate: opts.revalidate } } : {}),
    });
  } catch (e) {
    if (e instanceof ApiError) throw e;
    throw new ApiError("NETWORK_ERROR", 0, {}, "Khong ket noi duoc may chu su kien");
  }
}

function toAvailability(v: string | undefined): TierAvailability {
  return v === "AVAILABLE" || v === "FEW_LEFT" || v === "SOLD_OUT" ? v : "UNKNOWN";
}

function mapEvent(w: WireEvent, availability?: Map<string, TierAvailability>): EventDetailSnapshot {
  const tiers: TicketTierSnapshot[] = (w.tiers ?? []).map((t) => ({
    id: t.id,
    name: t.name,
    price: t.price_cents,
    currency: t.currency ?? "VND",
    // Het ve ca su kien thi moi hang deu het; chua mo ban / availability loi -> UNKNOWN (khong doan).
    availability: availability?.get(t.id) ?? (w.status === "SOLD_OUT" ? "SOLD_OUT" : "UNKNOWN"),
  }));
  return {
    id: w.id,
    slug: w.slug,
    title: w.title,
    description: w.description ?? "",
    venue: w.venue ?? "",
    startsAt: w.starts_at,
    saleStartAt: w.sale_start_at,
    ...(w.sale_end_at ? { saleEndAt: w.sale_end_at } : {}),
    state: w.status,
    maxPerOrder: w.max_per_order ?? 0,
    tiers,
  };
}

/** Availability cong khai cua mot su kien ON_SALE; loi bat ky -> map rong (UNKNOWN). */
async function fetchAvailability(eventId: string, opts: EventFetchOptions): Promise<Map<string, TierAvailability>> {
  const map = new Map<string, TierAvailability>();
  try {
    const res = await getPublic(`/v1/events/${eventId}/availability`, { revalidate: opts.revalidate });
    if (!res.ok) return map;
    const body = (await res.json()) as WireAvailability;
    for (const t of body.tiers ?? []) map.set(t.ticket_type_id, toAvailability(t.availability));
  } catch {
    /* availability chi mang tinh goi y: loi khong lam hong trang */
  }
  return map;
}

async function loadEvent(path: string, opts: EventFetchOptions): Promise<EventDetailSnapshot | null> {
  const res = await getPublic(path, opts);
  if (res.status === 404 || res.status === 400) return null;
  if (!res.ok) throw await toApiError(res);
  const wire = (await res.json()) as WireEvent;
  const availability = wire.status === "ON_SALE" ? await fetchAvailability(wire.id, opts) : undefined;
  return mapEvent(wire, availability);
}

/** Lay snapshot su kien theo slug (trang chi tiet ISR). */
export async function getEventBySlug(slug: string, opts: EventFetchOptions = {}): Promise<EventDetailSnapshot | null> {
  return loadEvent(`/api/v1/events/slug/${encodeURIComponent(slug)}`, opts);
}

/** Lay snapshot su kien theo UUID (checkout, phong cho). */
export async function getEventById(id: string, opts: EventFetchOptions = {}): Promise<EventDetailSnapshot | null> {
  return loadEvent(`/api/v1/events/${encodeURIComponent(id)}`, opts);
}

/** Lay danh sach su kien cong khai (SCHEDULED / ON_SALE / SOLD_OUT). Availability tung hang ve la UNKNOWN. */
export async function getAllPublicEvents(opts: EventFetchOptions = {}): Promise<EventDetailSnapshot[]> {
  const res = await getPublic("/api/v1/events/public", opts);
  if (!res.ok) throw await toApiError(res);
  const body = (await res.json()) as { events?: WireEvent[] | null };
  return (body.events ?? []).map((e) => mapEvent(e));
}
