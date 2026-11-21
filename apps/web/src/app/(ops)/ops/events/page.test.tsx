/**
 * Test cho trang danh sách war room `/ops/events` (EV-187, AC-8, plan §1.4 + §6 m-3).
 *
 * ops-warroom (test change request): trang không còn là server component đọc danh sách mẫu; nó render một client
 * list tải `listOpsEvents` (danh sách công khai thật) ở trình duyệt. Trạng thái loading / rỗng / lỗi nằm ở
 * app/(ops)/ops-pages.real.test.tsx; ở đây giữ các kiểm tra link, tiêu đề, force-dynamic và OPS_NAV.
 */
import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { OPS_NAV } from "@/components/layout/nav-config";
import { listOpsEvents, type OpsEventListItem } from "@/lib/ops-service";

import OpsEventsPage, { dynamic } from "./page";

vi.mock("@/lib/ops-service", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/ops-service")>();
  return { ...actual, listOpsEvents: vi.fn() };
});
const mockList = vi.mocked(listOpsEvents);

const EVENTS: OpsEventListItem[] = [
  { eventId: "e1", eventTitle: "Rock Fest", saleState: "ON_SALE" },
  { eventId: "e2", eventTitle: "Jazz Night", saleState: "SCHEDULED" },
  { eventId: "e3", eventTitle: "Hết vé rồi", saleState: "SOLD_OUT" },
];

beforeEach(() => {
  mockList.mockReset();
  mockList.mockResolvedValue(EVENTS);
});

describe("OpsEventsPage route (/ops/events - AC-8)", () => {
  it("liệt kê mọi sự kiện từ listOpsEvents, mỗi mục là link tới /ops/events/{id}", async () => {
    const { container } = render(OpsEventsPage());
    await screen.findByText("Rock Fest");

    for (const e of EVENTS) {
      const link = container.querySelector(`a[href="/ops/events/${e.eventId}"]`);
      expect(link, e.eventId).not.toBeNull();
      expect(link).toHaveTextContent(e.eventTitle);
    }
  });

  it("số link war room đúng bằng số sự kiện (không link giả)", async () => {
    const { container } = render(OpsEventsPage());
    await waitFor(() => expect(container.querySelectorAll("a").length).toBeGreaterThan(0));

    const warRoomLinks = Array.from(container.querySelectorAll("a")).filter((a) =>
      /^\/ops\/events\/[^/]+$/.test(a.getAttribute("href") ?? ""),
    );
    expect(warRoomLinks).toHaveLength(EVENTS.length);
  });

  it("có tiêu đề trang", () => {
    render(OpsEventsPage());
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
