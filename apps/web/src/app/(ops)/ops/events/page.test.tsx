/**
 * Test cho trang danh sách war room `/ops/events` (EV-187, AC-8, plan §1.4 + §6 m-3).
 * Server component: gọi trực tiếp rồi render kết quả, như (organizer)/organizer/events/page.test.tsx.
 */
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { OPS_NAV } from "@/components/layout/nav-config";
import { listOpsEvents } from "@/lib/ops-service";

import OpsEventsPage, { dynamic } from "./page";

describe("OpsEventsPage route (/ops/events - AC-8)", () => {
  it("liệt kê mọi sự kiện từ listOpsEvents, mỗi mục là link tới /ops/events/{id}", async () => {
    const events = await listOpsEvents();
    expect(events.length).toBeGreaterThan(0);

    const { container } = render(await OpsEventsPage());

    for (const e of events) {
      const link = container.querySelector(`a[href="/ops/events/${e.eventId}"]`);
      expect(link, e.eventId).not.toBeNull();
      expect(link).toHaveTextContent(e.eventTitle);
    }
  });

  it("số link war room đúng bằng số sự kiện (không link giả)", async () => {
    const events = await listOpsEvents();
    const { container } = render(await OpsEventsPage());

    const warRoomLinks = Array.from(container.querySelectorAll("a")).filter((a) =>
      /^\/ops\/events\/[^/]+$/.test(a.getAttribute("href") ?? ""),
    );
    expect(warRoomLinks).toHaveLength(events.length);
  });

  it("có tiêu đề trang", async () => {
    render(await OpsEventsPage());
    expect(screen.getAllByRole("heading").length).toBeGreaterThan(0);
  });

  it('export dynamic = "force-dynamic" (m-3)', () => {
    expect(dynamic).toBe("force-dynamic");
  });

  it("mục War room trong OPS_NAV đã ready: true và trỏ /ops/events (AC-8)", () => {
    const item = OPS_NAV.find((nav) => nav.href === "/ops/events");
    expect(item).toBeDefined();
    expect(item?.ready).toBe(true);
  });
});
