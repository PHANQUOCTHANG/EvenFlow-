import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api-client";
import type { EventLifecycleState, OrganizerEvent } from "@/lib/organizer-service";

import ReviewStatusPage from "./page";

const mocks = vi.hoisted(() => ({ getById: vi.fn(), submit: vi.fn(), cancel: vi.fn() }));

vi.mock("@/lib/organizer-service", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/organizer-service")>()),
  getOrganizerEventById: mocks.getById,
  submitOrganizerEvent: mocks.submit,
  cancelOrganizerEvent: mocks.cancel,
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

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

const event = (state: EventLifecycleState): OrganizerEvent => ({
  id: EVT,
  slug: "rock-fest",
  title: "Rock Fest",
  description: "",
  venue: "Mỹ Đình",
  startsAt: "2027-12-01T12:00:00Z",
  saleStartAt: "2027-11-01T03:00:00Z",
  state,
  organizerId: "org-1",
  tiers: [{ id: "t", name: "GA", price: 1, quota: 2 }],
  createdAt: "",
  updatedAt: "",
});

async function renderPage(eventId = EVT) {
  return render(await ReviewStatusPage({ params: Promise.resolve({ eventId }) }));
}

beforeEach(() => {
  Object.values(mocks).forEach((m) => m.mockReset());
});

describe("ReviewStatusPage route (/organizer/events/[eventId]/review-status - AC-23, AC-24)", () => {
  it("hiện trạng thái thật từ API và nhãn dữ liệu mẫu cho chi tiết kiểm duyệt", async () => {
    mocks.getById.mockResolvedValue(event("PENDING_REVIEW"));
    await renderPage();

    expect(await screen.findByText("Trạng thái kiểm duyệt")).toBeInTheDocument();
    expect(screen.getByText("Rock Fest")).toBeInTheDocument();
    expect(screen.getByTestId("real-status")).toHaveTextContent("PENDING_REVIEW");
    expect(screen.getByText(/dữ liệu mẫu \/ chưa khả dụng/)).toBeInTheDocument();
  });

  it("404 -> thẻ không tìm thấy sự kiện", async () => {
    mocks.getById.mockResolvedValue(null);
    await renderPage("non-existent");
    expect(await screen.findByText("Không tìm thấy sự kiện")).toBeInTheDocument();
  });

  it("lỗi tải: hiện lỗi và nút Thử lại", async () => {
    mocks.getById.mockRejectedValue(new ApiError("NETWORK_ERROR", 0, {}, "mất kết nối"));
    await renderPage();
    expect(await screen.findByText("mất kết nối")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Thử lại" })).toBeInTheDocument();
  });

  it("nộp duyệt lại sự kiện bị từ chối: gọi submit và cập nhật trạng thái trên màn hình", async () => {
    mocks.getById.mockResolvedValue(event("REJECTED"));
    mocks.submit.mockResolvedValue({ ...event("PENDING_REVIEW"), tiers: [] });
    await renderPage();

    fireEvent.click(await screen.findByRole("button", { name: /Nộp duyệt lại/ }));
    await waitFor(() => expect(screen.getByTestId("real-status")).toHaveTextContent("PENDING_REVIEW"));
    expect(mocks.submit).toHaveBeenCalledWith(EVT);
    expect(screen.queryByRole("button", { name: /Nộp duyệt lại/ })).not.toBeInTheDocument();
  });

  it("submit 422: hiện problem detail", async () => {
    mocks.getById.mockResolvedValue(event("DRAFT"));
    mocks.submit.mockRejectedValue(new ApiError("about:blank", 422, { detail: "cần ít nhất 1 hạng vé" }));
    await renderPage();
    fireEvent.click(await screen.findByRole("button", { name: /Nộp duyệt lại/ }));
    expect(await screen.findByText("cần ít nhất 1 hạng vé")).toBeInTheDocument();
  });

  it("huỷ có xác nhận rồi làm mới trạng thái thành CANCELLED", async () => {
    mocks.getById.mockResolvedValue(event("ON_SALE"));
    mocks.cancel.mockResolvedValue(event("CANCELLED"));
    await renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Huỷ sự kiện" }));
    expect(mocks.cancel).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận huỷ" }));

    await waitFor(() => expect(screen.getByTestId("real-status")).toHaveTextContent("CANCELLED"));
    expect(mocks.cancel).toHaveBeenCalledWith(EVT);
    expect(screen.queryByRole("button", { name: "Huỷ sự kiện" })).not.toBeInTheDocument();
  });

  it("huỷ 422: hiện problem detail và vẫn giữ trạng thái cũ", async () => {
    mocks.getById.mockResolvedValue(event("ON_SALE"));
    mocks.cancel.mockRejectedValue(new ApiError("about:blank", 422, { detail: "không thể huỷ" }));
    await renderPage();
    fireEvent.click(await screen.findByRole("button", { name: "Huỷ sự kiện" }));
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận huỷ" }));
    expect(await screen.findByText("không thể huỷ")).toBeInTheDocument();
    expect(screen.getByTestId("real-status")).toHaveTextContent("ON_SALE");
  });
});
