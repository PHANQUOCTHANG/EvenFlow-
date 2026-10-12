import { describe, expect, it } from "vitest";

import {
  getAllEventSlugs,
  getAllPublicEvents,
  getEventBySlug,
  MOCK_EVENTS,
} from "./event-service";

describe("event-service (EVF-111, BR-A4)", () => {
  it("tra ve danh sach slug dung cho generateStaticParams", async () => {
    const slugs = await getAllEventSlugs();
    expect(slugs.length).toBeGreaterThanOrEqual(1);
    expect(slugs).toContain("evenflow-grand-concert-2026");
  });

  it("tim dung su kien theo slug hop le", async () => {
    const event = await getEventBySlug("evenflow-grand-concert-2026");
    expect(event).not.toBeNull();
    expect(event?.slug).toBe("evenflow-grand-concert-2026");
    expect(event?.title).toContain("EvenFlow Grand Concert 2026");
  });

  it("tra ve null khi khong tim thay slug", async () => {
    const event = await getEventBySlug("slug-khong-ton-tai-404");
    expect(event).toBeNull();
  });

  it("tra ve toan bo danh sach su kien cong khai", async () => {
    const events = await getAllPublicEvents();
    expect(events.length).toBe(MOCK_EVENTS.length);
  });

  it("tuan thu BR-A4: moi hang ve deu mang availability dang khoang, khong co quota ro ri", () => {
    for (const ev of MOCK_EVENTS) {
      for (const tier of ev.tiers) {
        expect(["AVAILABLE", "FEW_LEFT", "SOLD_OUT"]).toContain(tier.availability);
        // Dam bao khong co field quota so luong chinh xac trong snapshot
        expect((tier as unknown as Record<string, unknown>).quota).toBeUndefined();
        expect((tier as unknown as Record<string, unknown>).remaining).toBeUndefined();
      }
    }
  });
});
