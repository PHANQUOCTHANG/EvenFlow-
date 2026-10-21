import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { EventLifecycleState, OrganizerEvent } from "@/lib/organizer-service";

import { ModerationStatusCard } from "./moderation-status-card";

function ev(state: EventLifecycleState, moderation?: OrganizerEvent["moderation"]): OrganizerEvent {
  return {
    id: "evt-1",
    slug: "s",
    title: "Rock Fest",
    description: "",
    venue: "Mỹ Đình",
    startsAt: "2027-12-01T12:00:00Z",
    saleStartAt: "2027-11-01T03:00:00Z",
    state,
    organizerId: "org-1",
    tiers: [],
    moderation,
    createdAt: "2026-10-01T00:00:00Z",
    updatedAt: "2026-10-01T00:00:00Z",
  };
}

describe("ModerationStatusCard — trạng thái thật và nhãn mẫu (AC-24)", () => {
  it.each([
    "DRAFT",
    "PENDING_REVIEW",
    "SCHEDULED",
    "ON_SALE",
    "SOLD_OUT",
    "CLOSED",
    "COMPLETED",
    "REJECTED",
    "CANCELLED",
  ] as const)("hiển thị trạng thái thực %s", (state) => {
    render(<ModerationStatusCard event={ev(state)} />);
    expect(screen.getByTestId("real-status")).toHaveTextContent(state);
  });

  it("không có dữ liệu kiểm duyệt: nêu rõ chưa khả dụng và không hiện điểm rủi ro/4 trục", () => {
    render(<ModerationStatusCard event={ev("PENDING_REVIEW")} />);
    expect(screen.getByText(/dữ liệu mẫu \/ chưa khả dụng/)).toBeInTheDocument();
    expect(screen.queryByText(/Phân tích 4 trục/)).not.toBeInTheDocument();
    expect(screen.queryByRole("progressbar")).not.toBeInTheDocument();
  });

  it("có dữ liệu kiểm duyệt: vẫn gắn nhãn dữ liệu mẫu", () => {
    render(
      <ModerationStatusCard
        event={ev("REJECTED", {
          overallRiskScore: 0.9,
          decision: "AUTO_REJECTED",
          scannedAt: "2026-10-01T00:00:00Z",
          reasons: ["x"],
          axes: [],
        })}
      />,
    );
    expect(screen.getByText(/là dữ liệu mẫu, chưa nối với nguồn thật/)).toBeInTheDocument();
  });
});

describe("ModerationStatusCard — nộp duyệt và huỷ (AC-23)", () => {
  it("nộp duyệt chỉ có ở DRAFT/REJECTED", () => {
    const onResubmit = vi.fn();
    const { rerender } = render(<ModerationStatusCard event={ev("DRAFT")} onResubmit={onResubmit} />);
    expect(screen.getByRole("button", { name: /Nộp duyệt lại/ })).toBeInTheDocument();
    rerender(<ModerationStatusCard event={ev("REJECTED")} onResubmit={onResubmit} />);
    expect(screen.getByRole("button", { name: /Nộp duyệt lại/ })).toBeInTheDocument();
    rerender(<ModerationStatusCard event={ev("PENDING_REVIEW")} onResubmit={onResubmit} />);
    expect(screen.queryByRole("button", { name: /Nộp duyệt lại/ })).not.toBeInTheDocument();
  });

  it("nộp duyệt lỗi 422: hiển thị problem detail", async () => {
    const onResubmit = vi.fn().mockRejectedValue(new Error("cần ít nhất 1 hạng vé"));
    render(<ModerationStatusCard event={ev("DRAFT")} onResubmit={onResubmit} />);
    fireEvent.click(screen.getByRole("button", { name: /Nộp duyệt lại/ }));
    expect(await screen.findByText("cần ít nhất 1 hạng vé")).toBeInTheDocument();
  });

  it("huỷ cần xác nhận: bấm Huỷ sự kiện chưa gọi onCancel", () => {
    const onCancel = vi.fn();
    render(<ModerationStatusCard event={ev("ON_SALE")} onCancel={onCancel} />);
    fireEvent.click(screen.getByRole("button", { name: "Huỷ sự kiện" }));
    expect(onCancel).not.toHaveBeenCalled();
    expect(screen.getByText("Xác nhận huỷ sự kiện")).toBeInTheDocument();
  });

  it("xác nhận huỷ gọi onCancel với id sự kiện", async () => {
    const onCancel = vi.fn().mockResolvedValue(undefined);
    render(<ModerationStatusCard event={ev("SCHEDULED")} onCancel={onCancel} />);
    fireEvent.click(screen.getByRole("button", { name: "Huỷ sự kiện" }));
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận huỷ" }));
    await waitFor(() => expect(onCancel).toHaveBeenCalledWith("evt-1"));
  });

  it("giữ lại sự kiện thì không gọi onCancel", () => {
    const onCancel = vi.fn();
    render(<ModerationStatusCard event={ev("SCHEDULED")} onCancel={onCancel} />);
    fireEvent.click(screen.getByRole("button", { name: "Huỷ sự kiện" }));
    fireEvent.click(screen.getByRole("button", { name: "Giữ lại sự kiện" }));
    expect(onCancel).not.toHaveBeenCalled();
    expect(screen.queryByText("Xác nhận huỷ sự kiện")).not.toBeInTheDocument();
  });

  it("huỷ lỗi 422: hiển thị problem detail", async () => {
    const onCancel = vi.fn().mockRejectedValue(new Error("không thể huỷ sự kiện đã kết thúc"));
    render(<ModerationStatusCard event={ev("ON_SALE")} onCancel={onCancel} />);
    fireEvent.click(screen.getByRole("button", { name: "Huỷ sự kiện" }));
    fireEvent.click(screen.getByRole("button", { name: "Xác nhận huỷ" }));
    expect(await screen.findByText("không thể huỷ sự kiện đã kết thúc")).toBeInTheDocument();
  });

  it.each(["COMPLETED", "CANCELLED"] as const)("không có nút huỷ khi %s", (state) => {
    render(<ModerationStatusCard event={ev(state)} onCancel={vi.fn()} />);
    expect(screen.queryByRole("button", { name: "Huỷ sự kiện" })).not.toBeInTheDocument();
  });

  it("link chỉnh sửa chỉ có ở DRAFT/SCHEDULED/REJECTED", () => {
    const { rerender } = render(<ModerationStatusCard event={ev("DRAFT")} />);
    expect(screen.getByRole("link", { name: /Chỉnh sửa thông tin/ })).toBeInTheDocument();
    rerender(<ModerationStatusCard event={ev("ON_SALE")} />);
    expect(screen.queryByRole("link", { name: /Chỉnh sửa thông tin/ })).not.toBeInTheDocument();
  });

  it("F-1: ON_SALE có link 'Tăng số lượng vé'", () => {
    render(<ModerationStatusCard event={ev("ON_SALE")} />);
    expect(screen.getByRole("link", { name: /Tăng số lượng vé/ })).toHaveAttribute(
      "href",
      expect.stringMatching(/\/edit$/),
    );
  });
});
