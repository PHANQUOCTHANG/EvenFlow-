/** DU LIEU MAU su kien -- KHONG phai du lieu that (AC-19).
 *
 * Chi ops-service (snapshot war room, da gan `isSample: true`) duoc dung. event-service va
 * organizer-service khong bao gio import file nay. */

import type { EventDetailSnapshot } from "@/lib/types/event";

/** Du lieu mau su kien phuc vu man hinh ops chua co API (EVF-60). */
export const SAMPLE_EVENTS: EventDetailSnapshot[] = [
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
    maxPerOrder: 4,
    tiers: [
      {
        id: "tier-ga",
        name: "Vé Tiêu Chuẩn (GA)",
        price: 650000,
        currency: "VND",
        availability: "AVAILABLE",
      },
      {
        id: "tier-vip",
        name: "Vé VIP Gần Sân Khấu",
        price: 1800000,
        currency: "VND",
        availability: "AVAILABLE",
      },
      {
        id: "tier-svip",
        name: "Vé SVIP Lounge",
        price: 3200000,
        currency: "VND",
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
    maxPerOrder: 4,
    tiers: [
      {
        id: "tier-standard-pass",
        name: "Standard Pass",
        price: 900000,
        currency: "VND",
        availability: "AVAILABLE",
      },
      {
        id: "tier-all-access",
        name: "All-Access Executive Pass",
        price: 2500000,
        currency: "VND",
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
    maxPerOrder: 4,
    tiers: [
      {
        id: "tier-regular",
        name: "Vé Đồng Hạng",
        price: 450000,
        currency: "VND",
        availability: "SOLD_OUT",
      },
    ],
  },
];
