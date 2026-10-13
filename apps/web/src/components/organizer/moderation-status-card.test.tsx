import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { OrganizerEvent } from "@/lib/organizer-service";

import { ModerationStatusCard } from "./moderation-status-card";

describe("ModerationStatusCard component (EV-186, EVF-116, Stitch 10.14, BR-E4, BR-E5)", () => {
  const rejectedEvent: OrganizerEvent = {
    id: "evt-rejected",
    slug: "rejected-event",
    title: "Sự Kiện Đêm Nhạc Không Giấy Phép",
    description: "Mô tả",
    venue: "Địa điểm chưa xác định",
    startsAt: "2026-12-01T20:00:00Z",
    saleStartAt: "2026-11-15T10:00:00Z",
    state: "REJECTED",
    organizerId: "org-1",
    tiers: [{ id: "t1", name: "Vé Vào Cửa", price: 200000, quota: 500 }],
    moderation: {
      overallRiskScore: 0.85,
      decision: "AUTO_REJECTED",
      scannedAt: "2026-10-10T12:00:00Z",
      reasons: [
        "Nội dung cấm: Địa điểm không rõ ràng, tiềm ẩn nguy cơ an ninh.",
        "Thông tin thiếu: Thiếu giấy phép tổ chức.",
      ],
      moderatorNotes: "Chờ xác minh với chính quyền địa phương.",
      axes: [
        {
          axis: "PROHIBITED_CONTENT",
          title: "Nội dung cấm",
          riskScore: 0.85,
          passed: false,
          notes: "Nguy cơ an ninh",
        },
        { axis: "MISLEADING_SCAM", title: "Sai lệch / Lừa đảo", riskScore: 0.2, passed: true },
        { axis: "IMAGE_COPYRIGHT", title: "Bản quyền hình ảnh", riskScore: 0.1, passed: true },
        {
          axis: "MISSING_INFO",
          title: "Thông tin thiếu",
          riskScore: 0.8,
          passed: false,
          notes: "Thiếu giấy phép",
        },
      ],
    },
    createdAt: "2026-10-10T10:00:00Z",
    updatedAt: "2026-10-10T12:00:00Z",
  };

  const approvedEvent: OrganizerEvent = {
    id: "evt-approved",
    slug: "approved-event",
    title: "Hội Thảo Công Nghệ Cloud 2026",
    description: "Mô tả",
    venue: "Hà Nội",
    startsAt: "2026-12-10T09:00:00Z",
    saleStartAt: "2026-11-20T10:00:00Z",
    state: "SCHEDULED",
    organizerId: "org-1",
    tiers: [{ id: "t2", name: "Vé Standard", price: 500000, quota: 1000 }],
    moderation: {
      overallRiskScore: 0.15,
      decision: "AUTO_APPROVED",
      scannedAt: "2026-10-10T14:00:00Z",
      reasons: ["Hồ sơ đầy đủ, không có vi phạm."],
      axes: [
        { axis: "PROHIBITED_CONTENT", title: "Nội dung cấm", riskScore: 0.05, passed: true },
        { axis: "MISLEADING_SCAM", title: "Sai lệch / Lừa đảo", riskScore: 0.1, passed: true },
        { axis: "IMAGE_COPYRIGHT", title: "Bản quyền hình ảnh", riskScore: 0.15, passed: true },
        { axis: "MISSING_INFO", title: "Thông tin thiếu", riskScore: 0.05, passed: true },
      ],
    },
    createdAt: "2026-10-10T13:00:00Z",
    updatedAt: "2026-10-10T14:00:00Z",
  };

  it("render trạng thái REJECTED, cảnh báo vi phạm và phân tích 4 trục AI (BR-E4)", () => {
    render(<ModerationStatusCard event={rejectedEvent} />);

    expect(screen.getByText("Sự Kiện Đêm Nhạc Không Giấy Phép")).toBeInTheDocument();
    expect(screen.getByText("❌ Bị từ chối kiểm duyệt")).toBeInTheDocument();
    expect(screen.getByText("Sự kiện không đạt tiêu chuẩn kiểm duyệt")).toBeInTheDocument();

    // Điểm rủi ro 85%
    expect(screen.getByText("85% (0.85 / 1.0)")).toBeInTheDocument();

    // 4 Trục kiểm duyệt
    expect(screen.getByText("Phân tích 4 trục kiểm duyệt tự động (BR-E4)")).toBeInTheDocument();
    expect(screen.getByText("Nội dung cấm")).toBeInTheDocument();
    expect(screen.getByText("Sai lệch / Lừa đảo")).toBeInTheDocument();
    expect(screen.getByText("Bản quyền hình ảnh")).toBeInTheDocument();
    expect(screen.getByText("Thông tin thiếu")).toBeInTheDocument();

    // Lý do
    expect(
      screen.getByText("Nội dung cấm: Địa điểm không rõ ràng, tiềm ẩn nguy cơ an ninh."),
    ).toBeInTheDocument();
  });

  it("render trạng thái SCHEDULED (AUTO_APPROVED) an toàn", () => {
    render(<ModerationStatusCard event={approvedEvent} />);

    expect(screen.getByText("✅ Đã duyệt — Sẵn sàng mở bán")).toBeInTheDocument();
    expect(screen.getByText("Sự kiện đã được phê duyệt")).toBeInTheDocument();
    expect(screen.getByText("15% (0.15 / 1.0)")).toBeInTheDocument();
  });

  it("gọi onResubmit khi người dùng bấm Nộp duyệt lại", async () => {
    const onResubmit = vi.fn().mockResolvedValue(undefined);
    render(<ModerationStatusCard event={rejectedEvent} onResubmit={onResubmit} />);

    const resubmitBtn = screen.getByRole("button", { name: "🔄 Nộp duyệt lại" });
    fireEvent.click(resubmitBtn);

    expect(onResubmit).toHaveBeenCalledWith("evt-rejected");
  });
});
