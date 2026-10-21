import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api-client";
import type { QueueItem } from "@/lib/moderation-service";

import ReviewQueuePage from "./page";

const mocks = vi.hoisted(() => ({ queue: vi.fn(), push: vi.fn(), role: "moderator" }));

vi.mock("@/lib/moderation-service", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/moderation-service")>()),
  getModerationQueue: mocks.queue,
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({
    user: { id: "m-1", role: mocks.role },
    loading: false,
    isAuthenticated: true,
    hasRole: (roles: string[]) => roles.includes(mocks.role),
  }),
}));

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: mocks.push }) }));

const item = (over: Partial<QueueItem> = {}): QueueItem => ({
  eventId: "e-1",
  title: "AXEA Music Festival",
  status: "PENDING_REVIEW",
  venue: "SVĐ Quân khu 7",
  startsAt: "2027-01-10T12:00:00Z",
  saleStartAt: "2026-12-01T03:00:00Z",
  aiVerdict: "NEEDS_HUMAN",
  aiScore: 0.62,
  aiReasons: ["Nghi vấn bản quyền hình ảnh"],
  submittedAt: "2026-10-20T08:00:00Z",
  ...over,
});

const QUEUE = [
  item(),
  item({ eventId: "e-2", title: "Hanoi Symphony", venue: "Nhà hát Lớn", aiScore: 0.41, submittedAt: "2026-10-20T06:00:00Z" }),
  item({
    eventId: "e-3",
    title: "Rave đêm",
    status: "REJECTED",
    aiVerdict: "AUTO_REJECTED",
    aiScore: 0.93,
    aiReasons: ["Nội dung cấm"],
    submittedAt: "2026-10-20T07:00:00Z",
  }),
];

beforeEach(() => {
  mocks.queue.mockReset();
  mocks.push.mockReset();
  mocks.role = "moderator";
});

describe("/moderator/reviews — hàng đợi thẩm định (EV-121)", () => {
  it("liệt kê hồ sơ kèm điểm AI, lý do và liên kết thẩm định", async () => {
    mocks.queue.mockResolvedValue(QUEUE);
    render(<ReviewQueuePage />);

    const row = await screen.findByRole("article", { name: /AXEA Music Festival/ });
    expect(within(row).getByText("62%")).toBeInTheDocument();
    expect(within(row).getByText("Nghi vấn bản quyền hình ảnh")).toBeInTheDocument();
    expect(within(row).getByRole("link", { name: /Mở thẩm định/ })).toHaveAttribute("href", "/moderator/reviews/e-1");
  });

  it("thẻ chỉ số: số hồ sơ cần người duyệt và số AI đã từ chối", async () => {
    mocks.queue.mockResolvedValue(QUEUE);
    render(<ReviewQueuePage />);

    expect(await screen.findByTestId("kpi-needs-human")).toHaveTextContent("2");
    expect(screen.getByTestId("kpi-ai-rejected")).toHaveTextContent("1");
  });

  it("sự kiện AI đã từ chối được đánh dấu có thể lật ngược", async () => {
    mocks.queue.mockResolvedValue(QUEUE);
    render(<ReviewQueuePage />);

    const row = await screen.findByRole("article", { name: /Rave đêm/ });
    expect(within(row).getByText(/AI đã từ chối/)).toBeInTheDocument();
    expect(within(row).getByText(/có thể lật ngược/)).toBeInTheDocument();
  });

  it("lọc 'AI đã từ chối' chỉ còn hồ sơ AUTO_REJECTED", async () => {
    // Hồ sơ thiếu review (aiVerdict null) không phải "AI đã từ chối".
    mocks.queue.mockResolvedValue([...QUEUE, item({ eventId: "e-4", title: "Chưa có AI", aiVerdict: null, aiScore: null })]);
    render(<ReviewQueuePage />);
    await screen.findByRole("article", { name: /AXEA/ });

    fireEvent.click(screen.getByRole("button", { name: /AI đã từ chối/ }));

    expect(screen.queryByRole("article", { name: /AXEA/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("article", { name: /Chưa có AI/ })).not.toBeInTheDocument();
    expect(screen.getByRole("article", { name: /Rave đêm/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /AI đã từ chối/ })).toHaveAttribute("aria-pressed", "true");
  });

  it("tìm theo tên hoặc địa điểm, không phân biệt dấu", async () => {
    mocks.queue.mockResolvedValue(QUEUE);
    render(<ReviewQueuePage />);
    await screen.findByRole("article", { name: /AXEA/ });

    fireEvent.change(screen.getByRole("searchbox", { name: /Tìm hồ sơ/ }), { target: { value: "nha hat" } });

    expect(screen.getAllByRole("article")).toHaveLength(1);
    expect(screen.getByRole("article", { name: /Hanoi Symphony/ })).toBeInTheDocument();
  });

  it("lọc không còn kết quả thì nói rõ, không báo hàng đợi trống", async () => {
    mocks.queue.mockResolvedValue(QUEUE);
    render(<ReviewQueuePage />);
    await screen.findByRole("article", { name: /AXEA/ });

    fireEvent.change(screen.getByRole("searchbox", { name: /Tìm hồ sơ/ }), { target: { value: "không có gì" } });

    expect(screen.getByText(/Không có hồ sơ khớp bộ lọc/)).toBeInTheDocument();
  });

  it("hàng đợi trống", async () => {
    mocks.queue.mockResolvedValue([]);
    render(<ReviewQueuePage />);
    expect(await screen.findByText(/Không còn hồ sơ nào chờ thẩm định/)).toBeInTheDocument();
  });

  it("lỗi tải có nút thử lại", async () => {
    mocks.queue.mockRejectedValueOnce(new ApiError("NETWORK_ERROR", 0, {}, "Không kết nối được máy chủ kiểm duyệt"));
    mocks.queue.mockResolvedValueOnce(QUEUE);
    render(<ReviewQueuePage />);

    expect(await screen.findByText(/Không kết nối được máy chủ kiểm duyệt/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(await screen.findByRole("article", { name: /AXEA/ })).toBeInTheDocument();
    expect(mocks.queue).toHaveBeenCalledTimes(2);
  });

  it("403 từ API: báo không có quyền, không hiện nút thử lại vô ích", async () => {
    mocks.queue.mockRejectedValue(new ApiError("HTTP_403", 403, { status: 403 }));
    render(<ReviewQueuePage />);

    expect(await screen.findByText(/không có quyền Moderator/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Thử lại" })).not.toBeInTheDocument();
  });

  it("vai trò khác không thấy hàng đợi và bị chuyển sang /unauthorized", async () => {
    mocks.role = "organizer";
    render(<ReviewQueuePage />);

    await waitFor(() => expect(mocks.push).toHaveBeenCalledWith("/unauthorized"));
    expect(mocks.queue).not.toHaveBeenCalled();
    expect(screen.queryByRole("article")).not.toBeInTheDocument();
  });
});
