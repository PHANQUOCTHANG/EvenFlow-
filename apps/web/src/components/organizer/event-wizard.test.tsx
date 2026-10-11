import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { OrganizerEvent } from "@/lib/organizer-service";

import { EventWizard } from "./event-wizard";

function toLocalDatetimeString(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  const year = date.getFullYear();
  const month = pad(date.getMonth() + 1);
  const day = pad(date.getDate());
  const hours = pad(date.getHours());
  const minutes = pad(date.getMinutes());
  return `${year}-${month}-${day}T${hours}:${minutes}`;
}

describe("EventWizard component (EV-186, EVF-116, Stitch 10.13, BR-E1, BR-E2, BR-E3)", () => {
  it("bắt buộc nhập thông tin cơ bản ở Bước 1 trước khi sang Bước 2", () => {
    const onSubmit = vi.fn();
    render(<EventWizard onSubmit={onSubmit} />);

    expect(screen.getByText("Tạo sự kiện mới")).toBeInTheDocument();
    expect(screen.getByLabelText(/Tên sự kiện/)).toBeInTheDocument();

    // Bấm tiếp tục khi chưa nhập gì
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));
    expect(screen.getByText("Vui lòng nhập tên sự kiện.")).toBeInTheDocument();

    // Nhập tên sự kiện
    fireEvent.change(screen.getByLabelText(/Tên sự kiện/), {
      target: { value: "Đại Nhạc Hội Mùa Thu" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));
    expect(screen.getByText("Vui lòng nhập địa điểm tổ chức.")).toBeInTheDocument();

    // Nhập địa điểm
    fireEvent.change(screen.getByLabelText(/Địa điểm tổ chức/), {
      target: { value: "Sân vận động Mỹ Đình" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));

    // Đã chuyển sang Bước 2
    expect(screen.getByText("Danh sách hạng vé")).toBeInTheDocument();
  });

  it("cho phép thêm / sửa hạng vé và kiểm tra BR-E1 ở Bước 2", () => {
    const onSubmit = vi.fn();
    render(<EventWizard onSubmit={onSubmit} />);

    // Nhập bước 1
    fireEvent.change(screen.getByLabelText(/Tên sự kiện/), {
      target: { value: "Sự Kiện A" },
    });
    fireEvent.change(screen.getByLabelText(/Địa điểm tổ chức/), {
      target: { value: "Hà Nội" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));

    // Bước 2: Thêm hạng vé mới
    fireEvent.click(screen.getByRole("button", { name: "+ Thêm hạng vé" }));
    expect(screen.getByText("Hạng vé #2")).toBeInTheDocument();

    // Sửa quota của hạng 1 thành 0 (vi phạm BR-E1)
    const quotaInputs = screen.getAllByLabelText(/Số lượng vé \(Quota\)/);
    fireEvent.change(quotaInputs[0], { target: { value: "0" } });

    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));
    expect(screen.getByText(/phải lớn hơn 0/)).toBeInTheDocument();

    // Sửa lại hợp lệ
    fireEvent.change(quotaInputs[0], { target: { value: "500" } });
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));

    // Đã chuyển sang Bước 3
    expect(screen.getByText(/Thời điểm mở bán \(T0\)/)).toBeInTheDocument();
  });

  it("kiểm tra validation BR-E2 (sale_start_at >= now + 30m) ở Bước 3", () => {
    const onSubmit = vi.fn();
    render(<EventWizard onSubmit={onSubmit} />);

    // Bước 1
    fireEvent.change(screen.getByLabelText(/Tên sự kiện/), {
      target: { value: "Sự Kiện B" },
    });
    fireEvent.change(screen.getByLabelText(/Địa điểm tổ chức/), {
      target: { value: "Đà Nẵng" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));

    // Bước 2
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));

    // Bước 3: Đặt thời gian mở bán chỉ cách 5 phút (vi phạm BR-E2)
    const tooSoon = toLocalDatetimeString(new Date(Date.now() + 5 * 60 * 1000));
    fireEvent.change(screen.getByLabelText(/Thời gian bắt đầu mở bán \(T0\)/), {
      target: { value: tooSoon },
    });

    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));
    expect(screen.getAllByText(/BR-E2/).length).toBeGreaterThanOrEqual(1);

    // Đặt thời gian mở bán 2 giờ sau (hợp lệ)
    const validTime = toLocalDatetimeString(new Date(Date.now() + 120 * 60 * 1000));
    fireEvent.change(screen.getByLabelText(/Thời gian bắt đầu mở bán \(T0\)/), {
      target: { value: validTime },
    });
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));

    // Đã chuyển sang Bước 4 (Review)
    expect(screen.getByText("Xác nhận thông tin sự kiện")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Gửi kiểm duyệt/ })).toBeInTheDocument();
  });

  it("gọi onSubmit với submitForReview=true khi bấm Gửi kiểm duyệt ở Bước 4", async () => {
    const onSubmit = vi.fn().mockResolvedValue(undefined);
    render(<EventWizard onSubmit={onSubmit} />);

    // Đi qua 3 bước
    fireEvent.change(screen.getByLabelText(/Tên sự kiện/), {
      target: { value: "Concert Đêm Nhạc Ánh Sáng" },
    });
    fireEvent.change(screen.getByLabelText(/Địa điểm tổ chức/), {
      target: { value: "Sân vận động Mỹ Đình" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" })); // Sang 2
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" })); // Sang 3

    const validTime = toLocalDatetimeString(new Date(Date.now() + 120 * 60 * 1000));
    fireEvent.change(screen.getByLabelText(/Thời gian bắt đầu mở bán \(T0\)/), {
      target: { value: validTime },
    });
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" })); // Sang 4

    // Bấm gửi kiểm duyệt
    fireEvent.click(screen.getByRole("button", { name: /Gửi kiểm duyệt/ }));

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        title: "Concert Đêm Nhạc Ánh Sáng",
        venue: "Sân vận động Mỹ Đình",
        submitForReview: true,
      }),
    );
  });

  it("chặn giảm quota của sự kiện đang ON_SALE (BR-E3)", () => {
    const onSaleEvent: OrganizerEvent = {
      id: "evt-on-sale",
      slug: "on-sale-event",
      title: "Sự Kiện Đang Mở Bán",
      description: "Mô tả",
      venue: "Hà Nội",
      startsAt: new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString(),
      saleStartAt: new Date(Date.now() - 60 * 60 * 1000).toISOString(),
      state: "ON_SALE",
      organizerId: "org-1",
      tiers: [{ id: "t1", name: "Vé Tiêu Chuẩn", price: 300000, quota: 1000 }],
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString(),
    };

    const onSubmit = vi.fn();
    render(<EventWizard initialEvent={onSaleEvent} onSubmit={onSubmit} />);

    // Sang Bước 2
    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));

    // Cố tình giảm quota từ 1000 xuống 600
    const quotaInput = screen.getByLabelText(/Số lượng vé \(Quota\)/);
    fireEvent.change(quotaInput, { target: { value: "600" } });

    fireEvent.click(screen.getByRole("button", { name: "Tiếp tục →" }));

    // Bị chặn lại bởi quy tắc BR-E3
    expect(screen.getByText(/Tuyệt đối cấm giảm số lượng vé/)).toBeInTheDocument();
    expect(screen.getAllByText(/BR-E3/).length).toBeGreaterThanOrEqual(1);
  });
});
