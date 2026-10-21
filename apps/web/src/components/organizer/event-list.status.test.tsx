import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { EventLifecycleState, OrganizerEvent } from "@/lib/organizer-service";

import { EventList } from "./event-list";

const ALL: EventLifecycleState[] = [
  "DRAFT",
  "PENDING_REVIEW",
  "SCHEDULED",
  "ON_SALE",
  "SOLD_OUT",
  "CLOSED",
  "COMPLETED",
  "REJECTED",
  "CANCELLED",
];

function ev(state: EventLifecycleState, i: number, tiers: OrganizerEvent["tiers"] = []): OrganizerEvent {
  return {
    id: `id-${i}`,
    slug: `slug-${i}`,
    title: `Sự kiện ${state}`,
    description: "",
    venue: "Hà Nội",
    startsAt: "2027-12-01T12:00:00Z",
    saleStartAt: "2027-11-01T03:00:00Z",
    state,
    organizerId: "org-1",
    tiers,
    createdAt: "2026-10-01T00:00:00Z",
    updatedAt: "2026-10-01T00:00:00Z",
  };
}

describe("EventList — 9 trạng thái từ API (AC-20)", () => {
  it("hiển thị đủ cả 9 sự kiện, mỗi sự kiện một nhãn trạng thái, kể cả COMPLETED", () => {
    render(<EventList events={ALL.map((s, i) => ev(s, i))} />);
    const region = screen.getByRole("region", { name: "Danh sách sự kiện" });
    expect(region.querySelectorAll("h2")).toHaveLength(9);
    for (const label of [
      "Bản nháp",
      "Chờ duyệt",
      "Đã lên lịch",
      "Đang mở bán",
      "Hết vé",
      "Đã đóng",
      "Đã kết thúc",
      "Bị từ chối",
      "Đã huỷ",
    ]) {
      expect(region).toHaveTextContent(label);
    }
  });

  it("có thể lọc theo trạng thái COMPLETED", () => {
    render(<EventList events={ALL.map((s, i) => ev(s, i))} />);
    fireEvent.click(screen.getByRole("tab", { name: "Đã kết thúc" }));
    expect(screen.getByText("Sự kiện COMPLETED")).toBeInTheDocument();
    expect(screen.queryByText("Sự kiện DRAFT")).not.toBeInTheDocument();
  });

  it("chỉ DRAFT/SCHEDULED/REJECTED có link chỉnh sửa", () => {
    render(<EventList events={ALL.map((s, i) => ev(s, i))} />);
    const hrefs = screen.getAllByRole("link", { name: /Chỉnh sửa/ }).map((l) => l.getAttribute("href"));
    expect(hrefs).toEqual(["/organizer/events/id-0/edit", "/organizer/events/id-2/edit", "/organizer/events/id-7/edit"]);
  });

  it("danh sách không kèm hạng vé thì không bịa '0 hạng vé' hay giá 0", () => {
    render(<EventList events={[ev("DRAFT", 0)]} />);
    expect(screen.queryByText(/hạng vé \(Tổng/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Giá từ/)).not.toBeInTheDocument();
  });

  it("có hạng vé thì hiện tổng quota và giá thấp nhất", () => {
    render(<EventList events={[ev("ON_SALE", 0, [{ id: "t", name: "GA", price: 650000, quota: 100 }])]} />);
    expect(screen.getByText(/1 hạng vé \(Tổng 100 vé\)/)).toBeInTheDocument();
    expect(screen.getByText(/Giá từ/)).toBeInTheDocument();
  });

  it("F-1/G-2: ON_SALE và SOLD_OUT có link 'Tăng số lượng vé' tới trang sửa", () => {
    render(<EventList events={[ev("ON_SALE", 3), ev("SOLD_OUT", 4), ev("CLOSED", 5)]} />);
    const links = screen.getAllByRole("link", { name: /Tăng số lượng vé/ });
    expect(links).toHaveLength(2);
    expect(links[0]).toHaveAttribute("href", "/organizer/events/id-3/edit");
    expect(links[1]).toHaveAttribute("href", "/organizer/events/id-4/edit");
  });
});
