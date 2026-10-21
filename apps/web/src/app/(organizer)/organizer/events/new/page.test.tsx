import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { OrganizerEvent } from "@/lib/organizer-service";

import NewEventPage from "./page";

const { pushMock, createMock } = vi.hoisted(() => ({ pushMock: vi.fn(), createMock: vi.fn() }));

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: pushMock,
  }),
}));

vi.mock("@/lib/organizer-service", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/organizer-service")>()),
  createOrganizerEvent: createMock,
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({
    user: { id: "u-1", role: "organizer" },
    loading: false,
    isAuthenticated: true,
    hasRole: (roles: string[]) => roles.includes("organizer"),
  }),
}));

const draft = (state: OrganizerEvent["state"] = "DRAFT"): OrganizerEvent => ({
  id: "evt-new",
  slug: "s",
  title: "Rock Fest",
  description: "",
  venue: "Mỹ Đình",
  startsAt: "2027-12-01T12:00:00Z",
  saleStartAt: "2027-11-01T03:00:00Z",
  state,
  organizerId: "org-1",
  tiers: [],
  createdAt: "",
  updatedAt: "",
});

function local(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function fillAndReachReview() {
  const next = () => fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));
  fireEvent.change(screen.getByLabelText(/Tên sự kiện/), { target: { value: "Rock Fest" } });
  fireEvent.change(screen.getByLabelText(/Địa điểm tổ chức/), { target: { value: "Mỹ Đình" } });
  next();
  next();
  fireEvent.change(screen.getByLabelText(/Thời gian bắt đầu mở bán \(T0\)/), {
    target: { value: local(new Date(Date.now() + 120 * 60 * 1000)) },
  });
  next();
}

beforeEach(() => {
  pushMock.mockReset();
  createMock.mockReset();
});

describe("NewEventPage route (/organizer/events/new - AC-21)", () => {
  it("render stepper wizard tạo sự kiện mới", () => {
    render(<NewEventPage />);
    expect(screen.getByText("Tạo sự kiện mới")).toBeInTheDocument();
    expect(screen.getByLabelText(/Tên sự kiện/)).toBeInTheDocument();
  });

  it("tạo và gửi duyệt thành công -> chuyển tới trang review-status của sự kiện vừa tạo", async () => {
    createMock.mockResolvedValue({ event: draft("PENDING_REVIEW") });
    render(<NewEventPage />);
    fillAndReachReview();
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));

    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/organizer/events/evt-new/review-status"));
    expect(createMock).toHaveBeenCalledWith(expect.objectContaining({ submitForReview: true }));
  });

  it("lưu nháp thành công -> về danh sách", async () => {
    createMock.mockResolvedValue({ event: draft() });
    render(<NewEventPage />);
    fillAndReachReview();
    fireEvent.click(screen.getByRole("button", { name: "Lưu nháp" }));
    await waitFor(() => expect(pushMock).toHaveBeenCalledWith("/organizer/events"));
  });

  it("lỗi ở bước hạng vé: ở lại trang, báo bước lỗi và link tới bản nháp đã tạo, không điều hướng", async () => {
    createMock.mockResolvedValue({ event: draft(), failedStep: "tiers", error: "tên hạng vé bị trùng" });
    render(<NewEventPage />);
    fillAndReachReview();
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));

    expect(await screen.findByText("Đã tạo bản nháp nhưng bước tạo hạng vé thất bại")).toBeInTheDocument();
    expect(screen.getByText("tên hạng vé bị trùng")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Mở bản nháp/ })).toHaveAttribute(
      "href",
      "/organizer/events/evt-new/edit",
    );
    expect(pushMock).not.toHaveBeenCalled();
  });

  it("lỗi ở bước submit 422: báo bước submit kèm detail", async () => {
    createMock.mockResolvedValue({ event: draft(), failedStep: "submit", error: "cần ít nhất 1 hạng vé" });
    render(<NewEventPage />);
    fillAndReachReview();
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));

    expect(await screen.findByText("Đã tạo bản nháp nhưng bước gửi kiểm duyệt thất bại")).toBeInTheDocument();
    expect(screen.getByText("cần ít nhất 1 hạng vé")).toBeInTheDocument();
    expect(pushMock).not.toHaveBeenCalled();
  });

  it("bước 1 (tạo sự kiện) lỗi: hiện thông báo lỗi, không có link bản nháp", async () => {
    createMock.mockRejectedValue(new Error("không tạo được sự kiện"));
    render(<NewEventPage />);
    fillAndReachReview();
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));

    expect(await screen.findByText("không tạo được sự kiện")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Mở bản nháp/ })).not.toBeInTheDocument();
  });

  it("F-2: sau lỗi một phần, không POST /events lần nữa (khoá nút lưu, giữ link bản nháp)", async () => {
    createMock.mockResolvedValue({ event: draft(), failedStep: "tiers", error: "tên hạng vé bị trùng" });
    render(<NewEventPage />);
    fillAndReachReview();
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));
    await screen.findByRole("link", { name: /Mở bản nháp/ });

    fireEvent.click(screen.getByRole("button", { name: "Lưu nháp" }));
    expect(createMock).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Lưu nháp" })).toBeDisabled();
  });
});
