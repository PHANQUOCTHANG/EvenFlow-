import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { EventDetailSnapshot } from "@/lib/types/event";

import { EventListView } from "./event-list-view";

const ev = (over: Partial<EventDetailSnapshot>): EventDetailSnapshot =>
  ({
    id: "1",
    slug: "su-kien-1",
    title: "Sự kiện 1",
    description: "",
    venue: "Hà Nội",
    startsAt: "2026-12-01T12:00:00Z",
    saleStartAt: "2026-11-01T03:00:00Z",
    state: "ON_SALE",
    currency: "VND",
    maxPerOrder: 4,
    tiers: [],
    ...over,
  }) as EventDetailSnapshot;

describe("EventListView (AC-17)", () => {
  it("co du lieu: moi the co link toi /events/{slug}", () => {
    render(<EventListView events={[ev({}), ev({ id: "2", slug: "su-kien-2", title: "Sự kiện 2", state: "SOLD_OUT" })]} />);
    const links = screen.getAllByRole("link", { name: /Xem chi tiết/ });
    expect(links.map((l) => l.getAttribute("href"))).toEqual(["/events/su-kien-1", "/events/su-kien-2"]);
    expect(screen.getByText("Hết vé")).toBeInTheDocument();
  });

  it("rong: empty state, khong co alert", () => {
    render(<EventListView events={[]} />);
    expect(screen.getByText(/chưa có sự kiện nào/i)).toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("loi: alert voi lien ket tai lai, ke ca khi con events", () => {
    render(<EventListView events={[ev({})]} error />);
    expect(screen.getByRole("alert")).toHaveTextContent(/Không tải được danh sách/);
    expect(screen.getByRole("link", { name: /tải lại trang/i })).toHaveAttribute("href", "/events");
    expect(screen.queryByText("Sự kiện 1")).not.toBeInTheDocument();
  });
});
