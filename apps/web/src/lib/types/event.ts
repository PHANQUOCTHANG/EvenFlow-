/** Kieu du lieu su kien phia web (dong bang trong contracts.md, AC-16..AC-18).
 *
 * BR-A4: ton kho chi hien thi dang khoang, khong lo so luong chinh xac. "UNKNOWN" = chua biet
 * (su kien chua mo ban hoac goi availability that bai) -- khong duoc coi la con ve hay het ve. */

export type EventStatus =
  | "DRAFT"
  | "PENDING_REVIEW"
  | "SCHEDULED"
  | "ON_SALE"
  | "SOLD_OUT"
  | "CLOSED"
  | "COMPLETED"
  | "REJECTED"
  | "CANCELLED";

export type TierAvailability = "AVAILABLE" | "FEW_LEFT" | "SOLD_OUT" | "UNKNOWN";

export interface TicketTierSnapshot {
  id: string;
  name: string;
  /** VND nguyen (price_cents cua backend, khong chia 100). */
  price: number;
  currency: string;
  quota?: number;
  availability: TierAvailability;
}

export interface EventDetailSnapshot {
  id: string;
  slug: string;
  title: string;
  description: string;
  venue: string;
  startsAt: string; // RFC 3339
  saleStartAt: string; // RFC 3339
  saleEndAt?: string;
  state: EventStatus;
  maxPerOrder: number;
  tiers: TicketTierSnapshot[];
}
