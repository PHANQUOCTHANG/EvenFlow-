import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiError } from "@/lib/api-client";
import type { ModerationDetail } from "@/lib/moderation-service";

import ReviewDetailPage from "./page";

const mocks = vi.hoisted(() => ({ detail: vi.fn(), approve: vi.fn(), reject: vi.fn(), push: vi.fn() }));

vi.mock("@/lib/moderation-service", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/moderation-service")>()),
  getModerationDetail: mocks.detail,
  approveEvent: mocks.approve,
  rejectEvent: mocks.reject,
}));

vi.mock("@/hooks/use-auth", () => ({
  useAuth: () => ({
    user: { id: "m-1", role: "moderator" },
    loading: false,
    isAuthenticated: true,
    hasRole: (roles: string[]) => roles.includes("moderator"),
  }),
}));

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: mocks.push }) }));

const EVT = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";

const detail = (over: Partial<ModerationDetail["event"]> = {}, rest: Partial<ModerationDetail> = {}): ModerationDetail => ({
  event: {
    id: EVT,
    organizerId: "org-1",
    title: "AXEA Music Festival",
    description: "Đêm nhạc ngoài trời quy mô 25.000 khán giả",
    venue: "SVĐ Quân khu 7",
    startsAt: "2027-01-10T12:00:00Z",
    saleStartAt: "2026-12-01T03:00:00Z",
    status: "PENDING_REVIEW",
    tiers: [{ id: "t-1", name: "GA", priceCents: 1200000, currency: "VND", quota: 18000 }],
    ...over,
  },
  review: {
    id: "r-1",
    aiScore: 0.62,
    aiVerdict: "NEEDS_HUMAN",
    aiReasons: ["Nghi vấn bản quyền hình ảnh"],
    humanVerdict: null,
    humanNote: null,
    createdAt: "2026-10-20T08:00:00Z",
    decidedAt: null,
  },
  axes: { forbidden_content: 0.05, misleading: 0.4, copyright: 0.7, incomplete_info: 0.2 },
  audit: [
    { id: 1, actorType: "ai", actorId: null, action: "event.needs_human_review", reason: null, createdAt: "2026-10-20T08:00:00Z" },
  ],
  ...rest,
});

async function renderPage(eventId = EVT) {
  return render(await ReviewDetailPage({ params: Promise.resolve({ eventId }) }));
}

beforeEach(() => {
  mocks.detail.mockReset();
  mocks.approve.mockReset();
  mocks.reject.mockReset();
  mocks.push.mockReset();
});

describe("/moderator/reviews/[eventId] — thẩm định hồ sơ (EV-121)", () => {
  it("tách nội dung gốc của BTC khỏi phân tích AI", async () => {
    mocks.detail.mockResolvedValue(detail());
    await renderPage();

    const original = await screen.findByRole("region", { name: /Nội dung gốc từ ban tổ chức/ });
    const ai = screen.getByRole("region", { name: /Phân tích AI/ });
    expect(within(original).getByText(/Đêm nhạc ngoài trời/)).toBeInTheDocument();
    expect(within(original).getByText("GA")).toBeInTheDocument();
    expect(within(original).queryByText(/Nghi vấn bản quyền/)).not.toBeInTheDocument();
    expect(within(ai).getByText(/Nghi vấn bản quyền hình ảnh/)).toBeInTheDocument();
    expect(within(original).queryByText(/Nghi vấn/)).toBeNull();
  });

  it("hiện 4 trục rủi ro với nhãn tiếng Việt", async () => {
    mocks.detail.mockResolvedValue(detail());
    await renderPage();

    const ai = await screen.findByRole("region", { name: /Phân tích AI/ });
    expect(within(ai).getByText(/Bản quyền hình ảnh/)).toBeInTheDocument();
    expect(within(ai).getByRole("meter", { name: /Bản quyền hình ảnh/ })).toHaveAttribute("aria-valuenow", "70");
    expect(within(ai).getAllByRole("meter")).toHaveLength(4);
  });

  it("AI chỉ là khuyến nghị: có câu miễn trừ, không có nút chấp nhận theo AI", async () => {
    mocks.detail.mockResolvedValue(detail());
    await renderPage();

    expect(await screen.findByText(/chỉ là khuyến nghị/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /theo AI/i })).not.toBeInTheDocument();
  });

  it("lịch sử xử lý từ audit", async () => {
    mocks.detail.mockResolvedValue(detail());
    await renderPage();

    const audit = await screen.findByRole("region", { name: /Lịch sử xử lý/ });
    expect(within(audit).getByText(/AI chuyển người duyệt/)).toBeInTheDocument();
  });

  it("duyệt hồ sơ NEEDS_HUMAN: xác nhận rồi gọi approve, ghi chú không bắt buộc", async () => {
    mocks.detail.mockResolvedValue(detail());
    mocks.approve.mockResolvedValue({ ...detail().event, status: "SCHEDULED" });
    await renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Duyệt" }));
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận duyệt" }));

    await waitFor(() => expect(mocks.approve).toHaveBeenCalledWith(EVT, ""));
    expect(await screen.findByText(/Đã duyệt/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Duyệt" })).not.toBeInTheDocument();
  });

  it("từ chối bắt buộc lý do: thiếu lý do thì không gọi API", async () => {
    mocks.detail.mockResolvedValue(detail());
    mocks.reject.mockResolvedValue({ ...detail().event, status: "REJECTED" });
    await renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Từ chối" }));
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận từ chối" }));
    expect(await screen.findByText(/Cần nhập lý do/)).toBeInTheDocument();
    expect(mocks.reject).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText(/Lý do/), { target: { value: "  Thiếu giấy phép  " } });
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận từ chối" }));
    await waitFor(() => expect(mocks.reject).toHaveBeenCalledWith(EVT, "Thiếu giấy phép"));
  });

  it("lật ngược AI (sự kiện REJECTED): duyệt bắt buộc lý do và ghi rõ là lật ngược", async () => {
    mocks.detail.mockResolvedValue(
      detail({ status: "REJECTED" }, { review: { ...detail().review!, aiVerdict: "AUTO_REJECTED", aiScore: 0.93 } }),
    );
    mocks.approve.mockResolvedValue({ ...detail().event, status: "SCHEDULED" });
    await renderPage();

    fireEvent.click(await screen.findByRole("button", { name: /Lật ngược.*duyệt/ }));
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận duyệt" }));
    expect(await screen.findByText(/Cần nhập lý do/)).toBeInTheDocument();
    expect(mocks.approve).not.toHaveBeenCalled();

    fireEvent.change(screen.getByLabelText(/Lý do/), { target: { value: "AI nhận nhầm, giấy phép hợp lệ" } });
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận duyệt" }));
    await waitFor(() => expect(mocks.approve).toHaveBeenCalledWith(EVT, "AI nhận nhầm, giấy phép hợp lệ"));
    // Sự kiện đã bị từ chối thì không còn nút "Từ chối" nữa.
    expect(screen.queryByRole("button", { name: "Từ chối" })).not.toBeInTheDocument();
  });

  it("422 override-reason-required từ server hiện thành lỗi ở ô lý do", async () => {
    mocks.detail.mockResolvedValue(detail({ status: "REJECTED" }));
    mocks.approve.mockRejectedValue(
      new ApiError("/problems/override-reason-required", 422, { type: "/problems/override-reason-required", status: 422 }),
    );
    await renderPage();

    fireEvent.click(await screen.findByRole("button", { name: /Lật ngược.*duyệt/ }));
    fireEvent.change(screen.getByLabelText(/Lý do/), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận duyệt" }));

    expect(await screen.findByText(/bắt buộc ghi lý do/)).toBeInTheDocument();
  });

  it("409: hồ sơ vừa được Moderator khác quyết định → báo và tải lại", async () => {
    mocks.detail.mockResolvedValueOnce(detail());
    mocks.detail.mockResolvedValueOnce(detail({ status: "SCHEDULED" }));
    mocks.reject.mockRejectedValue(new ApiError("/problems/already-decided", 409, { type: "/problems/already-decided", status: 409 }));
    await renderPage();

    fireEvent.click(await screen.findByRole("button", { name: "Từ chối" }));
    fireEvent.change(screen.getByLabelText(/Lý do/), { target: { value: "x" } });
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận từ chối" }));

    expect(await screen.findByText(/Moderator khác vừa quyết định/)).toBeInTheDocument();
    await waitFor(() => expect(mocks.detail).toHaveBeenCalledTimes(2));
    // Ban moi nhat (SCHEDULED) duoc ap dung: khoa quyet dinh, thong bao tranh chap van con.
    expect(await screen.findByText(/Hồ sơ đã được xử lý/)).toBeInTheDocument();
    expect(screen.getByText(/Moderator khác vừa quyết định/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Từ chối" })).not.toBeInTheDocument();
  });

  it("hồ sơ đã xử lý (SCHEDULED) bị khoá: không có nút quyết định", async () => {
    mocks.detail.mockResolvedValue(detail({ status: "SCHEDULED" }));
    await renderPage();

    expect(await screen.findByText(/Hồ sơ đã được xử lý/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Duyệt" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Từ chối" })).not.toBeInTheDocument();
  });

  it("không tìm thấy", async () => {
    mocks.detail.mockResolvedValue(null);
    await renderPage("khong-co");
    expect(await screen.findByText(/Không tìm thấy hồ sơ/)).toBeInTheDocument();
  });

  it("403: không có quyền, không hiện nội dung", async () => {
    mocks.detail.mockRejectedValue(new ApiError("HTTP_403", 403, { status: 403 }));
    await renderPage();
    expect(await screen.findByText(/không có quyền Moderator/i)).toBeInTheDocument();
    expect(screen.queryByRole("region", { name: /Phân tích AI/ })).not.toBeInTheDocument();
  });

  it("không có review AI: nói rõ, vẫn quyết định được", async () => {
    mocks.detail.mockResolvedValue(detail({}, { review: null, axes: {} }));
    await renderPage();
    expect(await screen.findByText(/Chưa có kết quả phân tích AI/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Duyệt" })).toBeInTheDocument();
  });
});
