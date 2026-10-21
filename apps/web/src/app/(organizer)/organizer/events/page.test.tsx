import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api-client";
import type { OrganizerEvent } from "@/lib/organizer-service";

import OrganizerEventsPage from "./page";

const { getEventsMock } = vi.hoisted(() => ({ getEventsMock: vi.fn() }));

vi.mock("@/lib/organizer-service", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/organizer-service")>()),
  getOrganizerEvents: getEventsMock,
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({
    user: { id: "u-1", role: "organizer" },
    loading: false,
    isAuthenticated: true,
    hasRole: (roles: string[]) => roles.includes("organizer"),
  }),
}));

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));

const event = (over: Partial<OrganizerEvent> = {}): OrganizerEvent => ({
  id: "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11",
  slug: "rock-fest",
  title: "Rock Fest",
  description: "",
  venue: "Mỹ Đình",
  startsAt: "2027-12-01T12:00:00Z",
  saleStartAt: "2027-11-01T03:00:00Z",
  state: "COMPLETED",
  organizerId: "org-1",
  tiers: [],
  createdAt: "2026-10-01T00:00:00Z",
  updatedAt: "2026-10-01T00:00:00Z",
  ...over,
});

beforeEach(() => {
  getEventsMock.mockReset();
});

describe("OrganizerEventsPage route (/organizer/events - AC-20)", () => {
  it("hiện trạng thái đang tải rồi danh sách lấy từ API", async () => {
    getEventsMock.mockResolvedValue([event(), event({ id: "e2", title: "Đêm nhạc", state: "CANCELLED" })]);
    render(<OrganizerEventsPage />);

    expect(screen.getByText("Đang tải danh sách sự kiện...")).toBeInTheDocument();
    expect(await screen.findByText("Sự kiện của tôi")).toBeInTheDocument();
    expect(screen.getByText("+ Tạo sự kiện mới")).toBeInTheDocument();
    expect(screen.getByText("Rock Fest")).toBeInTheDocument();
    expect(screen.getAllByText("Đã kết thúc").length).toBeGreaterThanOrEqual(1);
    expect(getEventsMock).toHaveBeenCalledTimes(1);
  });

  it("danh sách rỗng: hiện empty state mời tạo sự kiện", async () => {
    getEventsMock.mockResolvedValue([]);
    render(<OrganizerEventsPage />);
    expect(await screen.findByText("Không tìm thấy sự kiện nào")).toBeInTheDocument();
    expect(screen.getByText(/Bạn chưa tạo sự kiện nào/)).toBeInTheDocument();
  });

  it("lỗi: hiện thông báo lỗi (không dữ liệu mẫu) và nút Thử lại gọi lại API", async () => {
    getEventsMock.mockRejectedValueOnce(new ApiError("HTTP_500", 500, {}, "máy chủ đang bận"));
    render(<OrganizerEventsPage />);

    expect(await screen.findByText("máy chủ đang bận")).toBeInTheDocument();
    expect(screen.queryByText("Sự kiện của tôi")).not.toBeInTheDocument();

    getEventsMock.mockResolvedValueOnce([event()]);
    fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(await screen.findByText("Rock Fest")).toBeInTheDocument();
    expect(getEventsMock).toHaveBeenCalledTimes(2);
  });
});
