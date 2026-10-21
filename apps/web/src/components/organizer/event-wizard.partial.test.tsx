import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { OrganizerEvent } from "@/lib/organizer-service";

import { EventWizard } from "./event-wizard";

function local(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

const next = () => fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));

function fillToReview() {
  fireEvent.change(screen.getByLabelText(/Tên sự kiện/), { target: { value: "Rock Fest" } });
  fireEvent.change(screen.getByLabelText(/Địa điểm tổ chức/), { target: { value: "Mỹ Đình" } });
  next();
  next();
  fireEvent.change(screen.getByLabelText(/Thời gian bắt đầu mở bán \(T0\)/), {
    target: { value: local(new Date(Date.now() + 120 * 60 * 1000)) },
  });
  next();
}

const existing: OrganizerEvent = {
  id: "evt-1",
  slug: "rock-fest",
  title: "Rock Fest",
  description: "Mô tả",
  venue: "Mỹ Đình",
  startsAt: new Date(Date.now() + 10 * 24 * 3600 * 1000).toISOString(),
  saleStartAt: new Date(Date.now() + 5 * 24 * 3600 * 1000).toISOString(),
  state: "DRAFT",
  organizerId: "org-1",
  tiers: [{ id: "srv-tier-1", name: "GA", price: 650000, quota: 100 }],
  createdAt: "2026-10-01T00:00:00Z",
  updatedAt: "2026-10-01T00:00:00Z",
};

describe("EventWizard — lỗi một phần khi tạo (AC-21)", () => {
  it("bước hạng vé lỗi: nêu rõ bước nào lỗi và có link tới bản nháp đã tạo", async () => {
    const onSubmit = vi.fn().mockResolvedValue({
      error: "tên hạng vé bị trùng",
      failedStep: "tiers",
      draftHref: "/organizer/events/evt-9/edit",
    });
    render(<EventWizard onSubmit={onSubmit} />);
    fillToReview();
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));

    expect(await screen.findByText("Đã tạo bản nháp nhưng bước tạo hạng vé thất bại")).toBeInTheDocument();
    expect(screen.getByText("tên hạng vé bị trùng")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Mở bản nháp/ })).toHaveAttribute(
      "href",
      "/organizer/events/evt-9/edit",
    );
  });

  it("bước gửi kiểm duyệt lỗi 422: hiển thị detail và link bản nháp", async () => {
    const onSubmit = vi.fn().mockResolvedValue({
      error: "cần ít nhất 1 hạng vé",
      failedStep: "submit",
      draftHref: "/organizer/events/evt-9/edit",
    });
    render(<EventWizard onSubmit={onSubmit} />);
    fillToReview();
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));

    expect(await screen.findByText("Đã tạo bản nháp nhưng bước gửi kiểm duyệt thất bại")).toBeInTheDocument();
    expect(screen.getByText("cần ít nhất 1 hạng vé")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Mở bản nháp/ })).toBeInTheDocument();
  });

  it("onSubmit ném lỗi (ví dụ bước 1): hiện lỗi thường, không có link bản nháp", async () => {
    const onSubmit = vi.fn().mockRejectedValue(new Error("máy chủ lỗi"));
    render(<EventWizard onSubmit={onSubmit} />);
    fillToReview();
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));

    expect(await screen.findByText("máy chủ lỗi")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /Mở bản nháp/ })).not.toBeInTheDocument();
  });

  it("thành công (void): không hiện lỗi", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<EventWizard onSubmit={onSubmit} />);
    fillToReview();
    fireEvent.click(screen.getByRole("button", { name: "Lưu nháp" }));
    await waitFor(() => expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ submitForReview: false })));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

describe("EventWizard — chỉnh sửa (AC-22)", () => {
  it("hạng vé đã có: tên và giá chỉ đọc, quota sửa được, không có nút xoá", () => {
    render(
      <EventWizard
        initialEvent={{
          ...existing,
          tiers: [...existing.tiers, { id: "srv-tier-2", name: "VIP", price: 1, quota: 5 }],
        }}
        onSubmit={vi.fn()}
      />,
    );
    next();
    expect(screen.getAllByLabelText(/Tên hạng vé/)[0]).toBeDisabled();
    expect(screen.getAllByLabelText(/Giá vé \(VND\)/)[0]).toBeDisabled();
    expect(screen.getAllByLabelText(/Số lượng vé \(Quota\)/)[0]).toBeEnabled();
    expect(screen.queryByRole("button", { name: "Xoá hạng vé" })).not.toBeInTheDocument();
  });

  it("hạng vé mới thêm khi chỉnh sửa vẫn sửa được toàn bộ và xoá được", () => {
    render(<EventWizard initialEvent={existing} onSubmit={vi.fn()} />);
    next();
    fireEvent.click(screen.getByRole("button", { name: "+ Thêm hạng vé" }));
    expect(screen.getAllByLabelText(/Tên hạng vé/)[1]).toBeEnabled();
    expect(screen.getAllByRole("button", { name: "Xoá hạng vé" })).toHaveLength(1);
  });

  it("giờ diễn ra được hiển thị theo giờ địa phương của mốc thời gian RFC 3339", () => {
    render(<EventWizard initialEvent={existing} onSubmit={vi.fn()} />);
    expect(screen.getByLabelText(/Thời gian diễn ra sự kiện/)).toHaveValue(local(new Date(existing.startsAt)));
  });

  it("starts_at không sau sale_start_at bị chặn ở bước 3 trước khi gọi mạng", () => {
    const onSubmit = vi.fn();
    render(<EventWizard initialEvent={existing} onSubmit={onSubmit} />);
    next();
    next();
    fireEvent.change(screen.getByLabelText(/Thời gian bắt đầu mở bán \(T0\)/), {
      target: { value: local(new Date(Date.now() + 20 * 24 * 3600 * 1000)) },
    });
    next();
    expect(screen.getByText(/phải sau thời gian bắt đầu mở bán/)).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
