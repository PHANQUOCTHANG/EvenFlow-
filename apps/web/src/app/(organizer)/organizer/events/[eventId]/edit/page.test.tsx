import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api-client";
import type { EventLifecycleState, OrganizerEvent } from "@/lib/organizer-service";

import EditEventPage from "./page";

const mocks = vi.hoisted(() => ({
  push: vi.fn(),
  getById: vi.fn(),
  update: vi.fn(),
  addTier: vi.fn(),
  updateQuota: vi.fn(),
  submit: vi.fn(),
}));

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: mocks.push }) }));

vi.mock("@/lib/organizer-service", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/organizer-service")>()),
  getOrganizerEventById: mocks.getById,
  updateOrganizerEvent: mocks.update,
  addTier: mocks.addTier,
  updateTierQuota: mocks.updateQuota,
  submitOrganizerEvent: mocks.submit,
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({
    user: { id: "u-1", role: "organizer" },
    loading: false,
    isAuthenticated: true,
    hasRole: (roles: string[]) => roles.includes("organizer"),
  }),
}));

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

const event = (state: EventLifecycleState = "DRAFT"): OrganizerEvent => ({
  id: EVT,
  slug: "rock-fest",
  title: "Rock Fest",
  description: "Mô tả",
  venue: "Mỹ Đình",
  startsAt: new Date(Date.now() + 10 * 24 * 3600 * 1000).toISOString(),
  saleStartAt: new Date(Date.now() + 5 * 24 * 3600 * 1000).toISOString(),
  state,
  organizerId: "org-1",
  tiers: [{ id: "tier-a", name: "GA", price: 650000, quota: 100 }],
  createdAt: "",
  updatedAt: "",
});

async function renderPage() {
  const ui = await EditEventPage({ params: Promise.resolve({ eventId: EVT }) });
  return render(ui);
}

const next = () => fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));

function reachReview() {
  next();
  next();
  next();
}

beforeEach(() => {
  Object.values(mocks).forEach((m) => m.mockReset());
});

describe("EditEventPage route (/organizer/events/[eventId]/edit - AC-22)", () => {
  it("tải sự kiện từ API rồi hiện form chỉnh sửa", async () => {
    mocks.getById.mockResolvedValue(event());
    await renderPage();
    expect(screen.getByText("Đang tải sự kiện...")).toBeInTheDocument();
    expect(await screen.findByText("Chỉnh sửa: Rock Fest")).toBeInTheDocument();
    expect(mocks.getById).toHaveBeenCalledWith(EVT);
  });

  it("404 -> thẻ không tìm thấy sự kiện", async () => {
    mocks.getById.mockResolvedValue(null);
    await renderPage();
    expect(await screen.findByText("Không tìm thấy sự kiện")).toBeInTheDocument();
  });

  it("lỗi mạng/5xx -> thông báo lỗi và nút Thử lại", async () => {
    mocks.getById.mockRejectedValueOnce(new ApiError("HTTP_503", 503, {}, "dịch vụ tạm ngừng"));
    await renderPage();
    expect(await screen.findByText("dịch vụ tạm ngừng")).toBeInTheDocument();
    mocks.getById.mockResolvedValueOnce(event());
    fireEvent.click(screen.getByRole("button", { name: "Thử lại" }));
    expect(await screen.findByText("Chỉnh sửa: Rock Fest")).toBeInTheDocument();
  });

  it.each(["PENDING_REVIEW", "SOLD_OUT", "CLOSED", "COMPLETED", "CANCELLED"] as const)(
    "trạng thái %s không được chỉnh sửa: không hiện form",
    async (state) => {
      mocks.getById.mockResolvedValue(event(state));
      await renderPage();
      expect(await screen.findByText("Không thể chỉnh sửa sự kiện")).toBeInTheDocument();
      expect(screen.queryByText(/Chỉnh sửa: /)).not.toBeInTheDocument();
    },
  );

  it.each(["DRAFT", "SCHEDULED", "REJECTED"] as const)("trạng thái %s có form chỉnh sửa", async (state) => {
    mocks.getById.mockResolvedValue(event(state));
    await renderPage();
    expect(await screen.findByText("Chỉnh sửa: Rock Fest")).toBeInTheDocument();
  });

  it("lưu: PUT đầy đủ -> quota hạng cũ đổi -> hạng mới POST -> về review-status", async () => {
    mocks.getById.mockResolvedValue(event());
    mocks.update.mockResolvedValue(event());
    mocks.updateQuota.mockResolvedValue({});
    mocks.addTier.mockResolvedValue({});
    await renderPage();
    await screen.findByText("Chỉnh sửa: Rock Fest");

    next();
    fireEvent.change(screen.getByLabelText(/Số lượng vé \(Quota\)/), { target: { value: "250" } });
    fireEvent.click(screen.getByRole("button", { name: "+ Thêm hạng vé" }));
    next();
    next();
    fireEvent.click(screen.getByRole("button", { name: "Lưu nháp" }));

    await waitFor(() => expect(mocks.push).toHaveBeenCalledWith(`/organizer/events/${EVT}/review-status`));
    expect(mocks.update).toHaveBeenCalledWith(
      EVT,
      expect.objectContaining({ title: "Rock Fest", venue: "Mỹ Đình", submitForReview: false }),
    );
    expect(mocks.updateQuota).toHaveBeenCalledWith(EVT, "tier-a", 250);
    expect(mocks.addTier).toHaveBeenCalledTimes(1);
    expect(mocks.addTier).toHaveBeenCalledWith(EVT, expect.objectContaining({ name: "Hạng vé #2" }));
    expect(mocks.submit).not.toHaveBeenCalled();
  });

  it("quota không đổi thì không gọi PUT hạng vé", async () => {
    mocks.getById.mockResolvedValue(event());
    mocks.update.mockResolvedValue(event());
    await renderPage();
    await screen.findByText("Chỉnh sửa: Rock Fest");
    reachReview();
    fireEvent.click(screen.getByRole("button", { name: "Lưu nháp" }));
    await waitFor(() => expect(mocks.push).toHaveBeenCalled());
    expect(mocks.updateQuota).not.toHaveBeenCalled();
    expect(mocks.addTier).not.toHaveBeenCalled();
  });

  it("gửi kiểm duyệt sau khi lưu", async () => {
    mocks.getById.mockResolvedValue(event());
    mocks.update.mockResolvedValue(event());
    mocks.submit.mockResolvedValue(event("PENDING_REVIEW"));
    await renderPage();
    await screen.findByText("Chỉnh sửa: Rock Fest");
    reachReview();
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));
    await waitFor(() => expect(mocks.submit).toHaveBeenCalledWith(EVT));
    expect(mocks.push).toHaveBeenCalledWith(`/organizer/events/${EVT}/review-status`);
  });

  it("PUT thất bại (422): hiện detail, không đổi hạng vé, không điều hướng", async () => {
    mocks.getById.mockResolvedValue(event());
    mocks.update.mockRejectedValue(new Error("sự kiện không thể chỉnh sửa"));
    await renderPage();
    await screen.findByText("Chỉnh sửa: Rock Fest");
    reachReview();
    fireEvent.click(screen.getByRole("button", { name: "Lưu nháp" }));
    expect(await screen.findByText("sự kiện không thể chỉnh sửa")).toBeInTheDocument();
    expect(mocks.addTier).not.toHaveBeenCalled();
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("đổi quota hạng vé thất bại: báo bước hạng vé, kèm link mở lại bản nháp", async () => {
    mocks.getById.mockResolvedValue(event());
    mocks.update.mockResolvedValue(event());
    mocks.updateQuota.mockRejectedValue(new Error("quota không hợp lệ"));
    await renderPage();
    await screen.findByText("Chỉnh sửa: Rock Fest");
    next();
    fireEvent.change(screen.getByLabelText(/Số lượng vé \(Quota\)/), { target: { value: "250" } });
    next();
    next();
    fireEvent.click(screen.getByRole("button", { name: "Lưu nháp" }));

    expect(await screen.findByText(/Đã lưu thông tin sự kiện nhưng bước tạo hạng vé thất bại/)).toBeInTheDocument();
    expect(screen.getByText(/Hạng vé "GA": quota không hợp lệ/)).toBeInTheDocument();
    // F-3: liên kết tới chính trang này không tải lại gì -> thay bằng tải lại dữ liệu thật từ máy chủ.
    expect(screen.queryByRole("link", { name: /Mở bản nháp/ })).not.toBeInTheDocument();
    expect(mocks.getById.mock.calls.length).toBeGreaterThanOrEqual(3);
    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("F-1: ON_SALE -> form tăng số lượng vé, chỉ gọi PUT tier quota (không PUT sự kiện, không POST hạng vé)", async () => {
    mocks.getById.mockResolvedValue(event("ON_SALE"));
    mocks.updateQuota.mockResolvedValue({});
    await renderPage();
    expect(await screen.findByText(/Tăng số lượng vé/)).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText(/Số lượng vé \(Quota\)/), { target: { value: "500" } });
    fireEvent.click(screen.getByRole("button", { name: "Lưu số lượng vé" }));
    await waitFor(() => expect(mocks.push).toHaveBeenCalledWith(`/organizer/events/${EVT}/review-status`));
    expect(mocks.updateQuota).toHaveBeenCalledWith(EVT, "tier-a", 500);
    expect(mocks.update).not.toHaveBeenCalled();
    expect(mocks.addTier).not.toHaveBeenCalled();
    expect(mocks.submit).not.toHaveBeenCalled();
  });

  it("F-1: ON_SALE giảm quota bị chặn phía client", async () => {
    mocks.getById.mockResolvedValue(event("ON_SALE"));
    await renderPage();
    await screen.findByText(/Tăng số lượng vé/);
    fireEvent.change(screen.getByLabelText(/Số lượng vé \(Quota\)/), { target: { value: "10" } });
    fireEvent.click(screen.getByRole("button", { name: "Lưu số lượng vé" }));
    expect(screen.getByText(/Tuyệt đối cấm giảm số lượng vé/)).toBeInTheDocument();
    expect(mocks.updateQuota).not.toHaveBeenCalled();
  });

  it("F-3: thử lại sau khi lỗi một phần không POST lại hạng vé đã tạo", async () => {
    const base = event();
    mocks.getById.mockResolvedValue(base);
    mocks.update.mockResolvedValue(base);
    mocks.addTier
      .mockResolvedValueOnce({ id: "srv-new-1", name: "Hạng vé #2", price: 200000, quota: 100 })
      .mockRejectedValueOnce(new Error("hạng 3 lỗi"))
      .mockResolvedValueOnce({ id: "srv-new-2", name: "Hạng vé #3", price: 200000, quota: 100 });
    await renderPage();
    await screen.findByText("Chỉnh sửa: Rock Fest");
    next();
    fireEvent.click(screen.getByRole("button", { name: "+ Thêm hạng vé" }));
    fireEvent.click(screen.getByRole("button", { name: "+ Thêm hạng vé" }));
    next();
    next();
    // Sau lần lưu đầu, máy chủ đã có hạng vé #2.
    mocks.getById.mockResolvedValue({
      ...base,
      tiers: [...base.tiers, { id: "srv-new-1", name: "Hạng vé #2", price: 200000, quota: 100 }],
    });
    fireEvent.click(screen.getByRole("button", { name: "Lưu nháp" }));
    expect(await screen.findByText(/hạng 3 lỗi/)).toBeInTheDocument();
    expect(mocks.addTier).toHaveBeenCalledTimes(2);

    fireEvent.click(screen.getByRole("button", { name: "Lưu nháp" }));
    await waitFor(() => expect(mocks.push).toHaveBeenCalled());
    expect(mocks.addTier).toHaveBeenCalledTimes(3);
    expect(mocks.addTier).toHaveBeenLastCalledWith(EVT, expect.objectContaining({ name: "Hạng vé #3" }));
  });
});
