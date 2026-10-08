/** Event Snapshot Service cho ISR (EVF-111, BR-A4, docs/02 Tang 1).
 *
 * Cung cap snapshot du lieu su kien va danh sach hang ve.
 * Tuan thu tuyet doi BR-A4: Ton kho chi hien thi dang khoang (AVAILABLE, FEW_LEFT, SOLD_OUT),
 * khong duoc phep lo so luong ton kho chinh xac de tranh bot vet ve va ro ri du lieu he thong. */

import type { EventPublicState } from "@/components/event/event-card";

export type TierAvailability = "AVAILABLE" | "FEW_LEFT" | "SOLD_OUT";

export interface TicketTierSnapshot {
  id: string;
  name: string;
  price: number;
  description?: string;
  availability: TierAvailability;
}

export interface EventDetailSnapshot {
  id: string;
  slug: string;
  title: string;
  description: string;
  venue: string;
  startsAt: string; // ISO-8601 co timezone (e.g. "2026-11-20T19:00:00+07:00")
  saleStartAt: string; // T0: ISO-8601 co timezone (e.g. "2026-10-10T10:00:00+07:00")
  state: EventPublicState;
  bannerUrl?: string;
  tiers: TicketTierSnapshot[];
}

/** Du lieu snapshot su kien phuc vu render tinh va ISR. */
export const MOCK_EVENTS: EventDetailSnapshot[] = [
  {
    id: "evt-grand-concert-2026",
    slug: "evenflow-grand-concert-2026",
    title: "EvenFlow Grand Concert 2026 — Đêm Nhạc Ánh Sáng",
    description:
      "Đại nhạc hội quy tụ dàn nghệ sĩ hàng đầu Việt Nam và quốc tế. Hệ thống bán vé áp dụng công nghệ chống quá tải và phòng chờ công bằng.",
    venue: "Sân vận động Quốc gia Mỹ Đình, Hà Nội",
    startsAt: "2026-11-20T19:00:00+07:00",
    saleStartAt: "2026-10-15T10:00:00+07:00",
    state: "SCHEDULED",
    tiers: [
      {
        id: "tier-ga",
        name: "Vé Tiêu Chuẩn (GA)",
        price: 650000,
        description: "Khu vực đứng tự do tại khán đài B.",
        availability: "AVAILABLE",
      },
      {
        id: "tier-vip",
        name: "Vé VIP Gần Sân Khấu",
        price: 1800000,
        description: "Khu vực gần sân khấu kèm quà tặng độc quyền và lối vào ưu tiên.",
        availability: "AVAILABLE",
      },
      {
        id: "tier-svip",
        name: "Vé SVIP Lounge",
        price: 3200000,
        description: "Ghế ngồi khán đài VIP, cocktail nhẹ và check-in riêng.",
        availability: "FEW_LEFT",
      },
    ],
  },
  {
    id: "evt-tech-summit-2026",
    slug: "vietnam-tech-summit-2026",
    title: "Vietnam Tech Summit 2026 — Kỷ Nguyên AI & Cloud Scale",
    description:
      "Hội thảo công nghệ hàng đầu khu vực quy tụ các chuyên gia Kiến trúc hệ thống, High Concurrency và Trí tuệ nhân tạo.",
    venue: "Trung tâm Hội nghị Quốc gia, Hà Nội",
    startsAt: "2026-12-05T08:30:00+07:00",
    saleStartAt: "2026-09-01T09:00:00+07:00",
    state: "ON_SALE",
    tiers: [
      {
        id: "tier-standard-pass",
        name: "Standard Pass",
        price: 900000,
        description: "Tham dự toàn bộ các phiên hội thảo chính và triển lãm công nghệ.",
        availability: "AVAILABLE",
      },
      {
        id: "tier-all-access",
        name: "All-Access Executive Pass",
        price: 2500000,
        description: "Bao gồm tiệc networking tối cùng diễn giả và tài liệu độc quyền.",
        availability: "FEW_LEFT",
      },
    ],
  },
  {
    id: "evt-indie-acoustic-night",
    slug: "indie-acoustic-night-hanoi",
    title: "Indie Acoustic Night — Giai Điệu Mùa Thu",
    description:
      "Đêm nhạc acoustic ấm cúng trong không gian mở cùng các nghệ sĩ indie được yêu thích nhất.",
    venue: "Hồ Gươm Opera House, Hà Nội",
    startsAt: "2026-10-30T20:00:00+07:00",
    saleStartAt: "2026-09-10T12:00:00+07:00",
    state: "SOLD_OUT",
    tiers: [
      {
        id: "tier-regular",
        name: "Vé Đồng Hạng",
        price: 450000,
        description: "Bao gồm 01 đồ uống miễn phí.",
        availability: "SOLD_OUT",
      },
    ],
  },
];

/** Lay snapshot su kien theo slug phuc vu trang chi tiet ISR. */
export async function getEventBySlug(slug: string): Promise<EventDetailSnapshot | null> {
  const event = MOCK_EVENTS.find((e) => e.slug === slug);
  return event ?? null;
}

/** Lay danh sach toan bo slug de sinh static params (generateStaticParams). */
export async function getAllEventSlugs(): Promise<string[]> {
  return MOCK_EVENTS.map((e) => e.slug);
}

/** Lay toan bo su kien cong khai. */
export async function getAllPublicEvents(): Promise<EventDetailSnapshot[]> {
  return [...MOCK_EVENTS];
}
