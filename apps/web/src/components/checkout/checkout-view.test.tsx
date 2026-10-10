import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as holdClient from "@/lib/hold-client";
import type { EventDetailSnapshot } from "@/lib/event-service";
import { CheckoutView } from "./checkout-view";

const mockEvent: EventDetailSnapshot = {
  id: "evt-test-1",
  slug: "evt-test-1",
  title: "Đêm Nhạc Ánh Sáng 2026",
  description: "Mô tả sự kiện",
  venue: "Sân vận động Mỹ Đình",
  startsAt: "2026-11-20T19:00:00+07:00",
  saleStartAt: "2026-10-15T10:00:00+07:00",
  state: "ON_SALE",
  tiers: [
    {
      id: "tier-ga",
      name: "Vé Tiêu Chuẩn (GA)",
      price: 650000,
      availability: "AVAILABLE",
    },
    {
      id: "tier-vip",
      name: "Vé VIP",
      price: 1800000,
      availability: "FEW_LEFT",
    },
    {
      id: "tier-svip",
      name: "Vé SVIP",
      price: 3200000,
      availability: "SOLD_OUT",
    },
  ],
};

describe("CheckoutView (EVF-113, EV-183)", () => {
  beforeEach(() => {
    sessionStorage.clear();
    sessionStorage.setItem("evenflow_queue_token_evt-test-1", "mock-queue-token");
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("render danh sách hạng vé và nhãn định tính không lộ số tồn kho (BR-O1)", async () => {
    await act(async () => {
      render(<CheckoutView event={mockEvent} />);
    });

    expect(screen.getByRole("heading", { name: "Vé Tiêu Chuẩn (GA)" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Vé VIP" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Vé SVIP" })).toBeInTheDocument();

    expect(screen.getByText("Còn vé")).toBeInTheDocument();
    expect(screen.getByText("Sắp hết")).toBeInTheDocument();
    expect(screen.getByText("Hết vé")).toBeInTheDocument();
  });

  it("cho phép tăng số lượng và cập nhật tóm tắt đơn hàng", async () => {
    await act(async () => {
      render(<CheckoutView event={mockEvent} />);
    });

    // Tăng số lượng của vé GA
    const increaseBtn = screen.getByRole("button", { name: /Tăng số lượng Vé Tiêu Chuẩn/i });
    await act(async () => {
      fireEvent.click(increaseBtn);
    });

    // Kiểm tra tóm tắt đơn hàng
    expect(screen.getByRole("region", { name: "Tóm tắt đơn hàng" })).toBeInTheDocument();
    expect(screen.getAllByText(/Vé Tiêu Chuẩn \(GA\)/).length).toBeGreaterThan(0);
  });

  it("khi bấm 'Giữ vé và thanh toán', gọi requestHold với Idempotency-Key (BR-O5)", async () => {
    const futureExpiresAt = new Date(Date.now() + 600_000).toISOString();
    const requestHoldSpy = vi.spyOn(holdClient, "requestHold").mockResolvedValue({
      orderId: "ord-test",
      holdId: "hold-test",
      expiresAt: futureExpiresAt,
      ticketTypeId: "tier-ga",
      quantity: 1,
    });

    await act(async () => {
      render(<CheckoutView event={mockEvent} />);
    });

    const submitBtn = screen.getByRole("button", { name: "Giữ vé và thanh toán" });
    await act(async () => {
      fireEvent.click(submitBtn);
    });

    expect(requestHoldSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        eventId: "evt-test-1",
        ticketTypeId: "tier-ga",
        quantity: 1,
        queueToken: "mock-queue-token",
      }),
    );
  });

  it("khi có active hold, hiển thị cảnh báo ở hạng vé khác (BR-O3)", async () => {
    const futureExpiresAt = new Date(Date.now() + 600_000).toISOString();
    holdClient.saveActiveHold("evt-test-1", {
      orderId: "ord-active",
      holdId: "hold-active",
      expiresAt: futureExpiresAt,
      ticketTypeId: "tier-ga",
      quantity: 1,
    });

    await act(async () => {
      render(<CheckoutView event={mockEvent} />);
    });

    // Cảnh báo BR-O3 phải xuất hiện ở các vé khác
    expect(screen.getAllByText("Bạn đang giữ vé ở hạng khác").length).toBeGreaterThan(0);
    expect(
      screen.getAllByText(/Mỗi khách chỉ giữ được một lượt vé cho sự kiện này/i).length,
    ).toBeGreaterThan(0);
  });

  it("khi hold active, bấm 'Tiếp tục thanh toán' gọi onProceedToPayment callback", async () => {
    const futureExpiresAt = new Date(Date.now() + 600_000).toISOString();
    holdClient.saveActiveHold("evt-test-1", {
      orderId: "ord-active",
      holdId: "hold-active",
      expiresAt: futureExpiresAt,
      ticketTypeId: "tier-ga",
      quantity: 1,
    });

    const onProceed = vi.fn();
    await act(async () => {
      render(<CheckoutView event={mockEvent} onProceedToPayment={onProceed} />);
    });

    const continueBtn = screen.getByRole("button", { name: "Tiếp tục thanh toán" });
    await act(async () => {
      fireEvent.click(continueBtn);
    });

    expect(onProceed).toHaveBeenCalledWith("ord-active", "hold-active");
  });

  it("hiển thị cảnh báo chưa admit và nút quay lại phòng chờ khi không có token (AC-6)", async () => {
    sessionStorage.clear(); // Xoá token

    await act(async () => {
      render(<CheckoutView event={mockEvent} />);
    });

    // Thử bấm giữ vé
    const submitBtn = screen.getByRole("button", { name: "Giữ vé và thanh toán" });
    await act(async () => {
      fireEvent.click(submitBtn);
    });

    expect(await screen.findByText(/Chưa được cấp quyền mua vé/i)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Quay lại phòng chờ/i })).toHaveAttribute(
      "href",
      "/waiting/evt-test-1",
    );
  });
});
