import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { OrganizerEvent } from "@/lib/organizer-service";

import { EventList } from "./event-list";

describe("EventList component (EV-186, EVF-116, Stitch 10.12)", () => {
  const mockEvents: OrganizerEvent[] = [
    {
      id: "evt-1",
      slug: "tech-summit-2026",
      title: "Vietnam Tech Summit 2026",
      description: "Hội thảo công nghệ hàng đầu",
      venue: "Hà Nội",
      startsAt: "2026-12-05T08:30:00+07:00",
      saleStartAt: "2026-10-01T09:00:00+07:00",
      state: "ON_SALE",
      organizerId: "org-1",
      tiers: [
        { id: "t1", name: "Standard", price: 500000, quota: 1000 },
        { id: "t2", name: "VIP", price: 1500000, quota: 200 },
      ],
      createdAt: "2026-09-01T00:00:00Z",
      updatedAt: "2026-09-01T00:00:00Z",
    },
    {
      id: "evt-2",
      slug: "indie-concert",
      title: "Đêm Nhạc Indie Acoustic",
      description: "Đêm nhạc mùa thu",
      venue: "TP. Hồ Chí Minh",
      startsAt: "2026-11-20T19:00:00+07:00",
      saleStartAt: "2026-10-15T10:00:00+07:00",
      state: "SCHEDULED",
      organizerId: "org-1",
      tiers: [{ id: "t3", name: "Vé Đứng", price: 300000, quota: 500 }],
      createdAt: "2026-09-05T00:00:00Z",
      updatedAt: "2026-09-05T00:00:00Z",
    },
    {
      id: "evt-3",
      slug: "secret-night",
      title: "Tiệc Âm Nhạc Trái Phép",
      description: "Không rõ địa điểm",
      venue: "Chưa xác định",
      startsAt: "2026-11-25T23:00:00+07:00",
      saleStartAt: "2026-10-20T20:00:00+07:00",
      state: "REJECTED",
      organizerId: "org-1",
      tiers: [{ id: "t4", name: "Vé Vào Cửa", price: 200000, quota: 300 }],
      moderation: {
        overallRiskScore: 0.85,
        decision: "AUTO_REJECTED",
        scannedAt: "2026-10-08T11:00:00+07:00",
        reasons: ["Nội dung cấm: Địa điểm không rõ ràng."],
        axes: [],
      },
      createdAt: "2026-09-10T00:00:00Z",
      updatedAt: "2026-09-10T00:00:00Z",
    },
  ];

  it("render danh sách sự kiện đầy đủ thông tin, ngày giờ và status badge (AC-1)", () => {
    render(<EventList events={mockEvents} />);

    expect(screen.getByText("Sự kiện của tôi")).toBeInTheDocument();
    expect(screen.getByText("+ Tạo sự kiện mới")).toBeInTheDocument();

    expect(screen.getByText("Vietnam Tech Summit 2026")).toBeInTheDocument();
    expect(screen.getAllByText("Đang mở bán").length).toBeGreaterThanOrEqual(1);

    expect(screen.getByText("Đêm Nhạc Indie Acoustic")).toBeInTheDocument();
    expect(screen.getAllByText("Đã lên lịch").length).toBeGreaterThanOrEqual(1);

    expect(screen.getByText("Tiệc Âm Nhạc Trái Phép")).toBeInTheDocument();
    expect(screen.getAllByText("Bị từ chối").length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText(/Nội dung cấm: Địa điểm không rõ ràng/)).toBeInTheDocument();
  });

  it("tìm kiếm sự kiện theo từ khoá tên hoặc địa điểm", () => {
    render(<EventList events={mockEvents} />);

    const searchInput = screen.getByPlaceholderText(/Tìm theo tên sự kiện, địa điểm/);
    fireEvent.change(searchInput, { target: { value: "Acoustic" } });

    expect(screen.getByText("Đêm Nhạc Indie Acoustic")).toBeInTheDocument();
    expect(screen.queryByText("Vietnam Tech Summit 2026")).not.toBeInTheDocument();
  });

  it("lọc sự kiện theo tab trạng thái", () => {
    render(<EventList events={mockEvents} />);

    const onSaleTab = screen.getByRole("tab", { name: "Đang mở bán" });
    fireEvent.click(onSaleTab);

    expect(screen.getByText("Vietnam Tech Summit 2026")).toBeInTheDocument();
    expect(screen.queryByText("Đêm Nhạc Indie Acoustic")).not.toBeInTheDocument();
  });

  it("hiển thị empty state khi không có sự kiện nào khớp", () => {
    render(<EventList events={mockEvents} />);

    const searchInput = screen.getByPlaceholderText(/Tìm theo tên sự kiện, địa điểm/);
    fireEvent.change(searchInput, { target: { value: "Từ khoá không hề tồn tại" } });

    expect(screen.getByText("Không tìm thấy sự kiện nào")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Đặt lại bộ lọc" })).toBeInTheDocument();
  });

  it("render các link hành động nhanh tới sales, review-status và edit", () => {
    render(<EventList events={mockEvents} />);

    const salesLinks = screen.getAllByRole("link", { name: /Doanh số/ });
    expect(salesLinks[0]).toHaveAttribute("href", "/organizer/events/evt-1/sales");

    const reviewLinks = screen.getAllByRole("link", { name: /Kiểm duyệt/ });
    expect(reviewLinks[0]).toHaveAttribute("href", "/organizer/events/evt-1/review-status");

    // Chỉ DRAFT/SCHEDULED/REJECTED mới được chỉnh sửa (AC-22): evt-1 là ON_SALE nên không có link.
    const editLinks = screen.getAllByRole("link", { name: /Chỉnh sửa/ });
    expect(editLinks.map((l) => l.getAttribute("href"))).toEqual([
      "/organizer/events/evt-2/edit",
      "/organizer/events/evt-3/edit",
    ]);
  });
});
