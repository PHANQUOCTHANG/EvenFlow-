import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { EventLifecycleState, OrganizerEvent } from "@/lib/organizer-service";

import { EventWizard } from "./event-wizard";

function local(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

const next = () => fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));

const existing = (state: EventLifecycleState = "DRAFT"): OrganizerEvent => ({
  id: "evt-1",
  slug: "rock-fest",
  title: "Rock Fest",
  description: "Mô tả",
  venue: "Mỹ Đình",
  startsAt: new Date(Date.now() + 10 * 24 * 3600 * 1000).toISOString(),
  saleStartAt: new Date(Date.now() + 5 * 24 * 3600 * 1000).toISOString(),
  state,
  organizerId: "org-1",
  tiers: [{ id: "srv-tier-1", name: "GA", price: 650000, quota: 100 }],
  createdAt: "",
  updatedAt: "",
});

describe("EventWizard — fix round 1", () => {
  it("F-9: chế độ sửa -> slug chỉ đọc (PUT không có slug)", () => {
    render(<EventWizard initialEvent={existing()} onSubmit={vi.fn()} />);
    expect(screen.getByLabelText(/Đường dẫn tĩnh/)).toBeDisabled();
  });

  it("F-9: chế độ tạo -> slug nhập được", () => {
    render(<EventWizard onSubmit={vi.fn()} />);
    expect(screen.getByLabelText(/Đường dẫn tĩnh/)).toBeEnabled();
  });

  it.each(["SCHEDULED"] as const)("F-10: %s -> không có nút Gửi kiểm duyệt", (state) => {
    render(<EventWizard initialEvent={existing(state)} onSubmit={vi.fn()} />);
    next();
    next();
    next();
    expect(screen.getByRole("button", { name: "Lưu nháp" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Gửi kiểm duyệt/ })).not.toBeInTheDocument();
  });

  it.each(["DRAFT", "REJECTED"] as const)("F-10: %s -> có nút Gửi kiểm duyệt", (state) => {
    render(<EventWizard initialEvent={existing(state)} onSubmit={vi.fn()} />);
    next();
    next();
    next();
    expect(screen.getByRole("button", { name: /Gửi kiểm duyệt/ })).toBeInTheDocument();
  });

  it("F-19: mở bán < 30 phút vẫn sang được bước 4 và Lưu nháp gọi onSubmit", () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<EventWizard initialEvent={existing()} onSubmit={onSubmit} />);
    next();
    next();
    fireEvent.change(screen.getByLabelText(/Thời gian bắt đầu mở bán \(T0\)/), {
      target: { value: local(new Date(Date.now() + 5 * 60 * 1000)) },
    });
    next();
    expect(screen.getByText("Xác nhận thông tin sự kiện")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Lưu nháp" }));
    expect(onSubmit).toHaveBeenCalledWith(expect.objectContaining({ submitForReview: false }));
  });

  it("F-19: mở bán < 30 phút chỉ chặn Gửi kiểm duyệt (BR-E2)", () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<EventWizard initialEvent={existing()} onSubmit={onSubmit} />);
    next();
    next();
    fireEvent.change(screen.getByLabelText(/Thời gian bắt đầu mở bán \(T0\)/), {
      target: { value: local(new Date(Date.now() + 5 * 60 * 1000)) },
    });
    next();
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));
    expect(onSubmit).not.toHaveBeenCalled();
    expect(screen.getAllByText(/BR-E2/).length).toBeGreaterThanOrEqual(1);
  });

  it("F-2: kết quả một phần có blockResubmit -> khoá nút lưu, giữ link bản nháp", async () => {
    const onSubmit = vi
      .fn()
      .mockResolvedValue({ error: "lỗi hạng vé", failedStep: "tiers", draftHref: "/x/edit", blockResubmit: true });
    render(<EventWizard onSubmit={onSubmit} />);
    fireEvent.change(screen.getByLabelText(/Tên sự kiện/), { target: { value: "A" } });
    fireEvent.change(screen.getByLabelText(/Địa điểm tổ chức/), { target: { value: "B" } });
    next();
    next();
    fireEvent.change(screen.getByLabelText(/Thời gian bắt đầu mở bán \(T0\)/), {
      target: { value: local(new Date(Date.now() + 120 * 60 * 1000)) },
    });
    next();
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));
    await screen.findByRole("link", { name: /Mở bản nháp/ });
    expect(screen.getByRole("button", { name: "Lưu nháp" })).toBeDisabled();
    expect(screen.getByRole("button", { name: /Gửi kiểm duyệt/ })).toBeDisabled();
  });

  it("F-1: ON_SALE -> chỉ form tăng số lượng vé, trường sự kiện chỉ đọc, chặn giảm", () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<EventWizard initialEvent={existing("ON_SALE")} onSubmit={onSubmit} />);
    expect(screen.queryByLabelText(/Tên sự kiện/)).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Gửi kiểm duyệt/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "+ Thêm hạng vé" })).not.toBeInTheDocument();
    expect(screen.getByLabelText(/Tên hạng vé/)).toBeDisabled();
    const quota = screen.getByLabelText(/Số lượng vé \(Quota\)/);
    fireEvent.change(quota, { target: { value: "50" } });
    fireEvent.click(screen.getByRole("button", { name: "Lưu số lượng vé" }));
    expect(screen.getByText(/Tuyệt đối cấm giảm số lượng vé/)).toBeInTheDocument();
    expect(onSubmit).not.toHaveBeenCalled();
    fireEvent.change(quota, { target: { value: "300" } });
    fireEvent.click(screen.getByRole("button", { name: "Lưu số lượng vé" }));
    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ tiers: [expect.objectContaining({ id: "srv-tier-1", quota: 300 })] }),
    );
  });
});
