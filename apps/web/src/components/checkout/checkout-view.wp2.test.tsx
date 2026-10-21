import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as holdClient from "@/lib/hold-client";
import type { EventDetailSnapshot } from "@/lib/types/event";
import { CheckoutView } from "./checkout-view";

/** WP2 (AC-10, BR-A4): loi giu ve dua nguoi dung ve phong cho; availability UNKNOWN khong bao gio la "Con ve". */

const event: EventDetailSnapshot = {
  id: "evt-wp2",
  slug: "evt-wp2",
  title: "Đêm Nhạc WP2",
  description: "",
  venue: "Mỹ Đình",
  startsAt: "2026-11-20T19:00:00+07:00",
  saleStartAt: "2026-10-15T10:00:00+07:00",
  state: "ON_SALE",
  maxPerOrder: 4,
  tiers: [
    { id: "tier-known", name: "Vé Biết Tồn Kho", price: 100000, currency: "VND", availability: "AVAILABLE" },
    { id: "tier-unknown", name: "Vé Chưa Rõ", price: 200000, currency: "VND", availability: "UNKNOWN" },
  ],
};

async function holdWithError(type: holdClient.HoldClientErrorType, status = 403) {
  vi.spyOn(holdClient, "requestHold").mockRejectedValue(new holdClient.HoldClientError("loi", type, status));
  await act(async () => {
    render(<CheckoutView event={event} />);
  });
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Giữ vé và thanh toán" }));
  });
}

describe("CheckoutView — WP2", () => {
  beforeEach(() => {
    sessionStorage.clear();
    sessionStorage.setItem("ef:queue:token:evt-wp2", "stale-token");
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it("availability UNKNOWN KHONG hien nhan 'Còn vé' (chi hang AVAILABLE moi co)", async () => {
    await act(async () => {
      render(<CheckoutView event={event} />);
    });
    expect(screen.getAllByText("Còn vé")).toHaveLength(1);
    expect(screen.getByRole("heading", { name: "Vé Chưa Rõ" })).toBeInTheDocument();
  });

  it("QUEUE_TOKEN_INVALID: hien thong bao, link quay lai phong cho, bo queue token cu", async () => {
    await holdWithError("QUEUE_TOKEN_INVALID");

    expect(screen.getByText("Lượt vào mua vé không còn hiệu lực")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Quay lại phòng chờ" })).toHaveAttribute("href", "/waiting/evt-wp2");
    expect(sessionStorage.getItem("ef:queue:token:evt-wp2")).toBeNull();
  });

  it("NOT_ADMITTED van chi co banner cu, khong xoa token", async () => {
    await holdWithError("NOT_ADMITTED");

    expect(screen.queryByText("Lượt vào mua vé không còn hiệu lực")).toBeNull();
    expect(sessionStorage.getItem("ef:queue:token:evt-wp2")).toBe("stale-token");
  });

  it("PER_IDENTITY_LIMIT va QUANTITY_NOT_ALLOWED hien thong bao rieng", async () => {
    await holdWithError("PER_IDENTITY_LIMIT", 422);
    expect(screen.getByText("Bạn đã đạt giới hạn số vé cho sự kiện này.")).toBeInTheDocument();
  });

  it("AUTH_REQUIRED: mo duong dan dang nhap lai", async () => {
    await holdWithError("AUTH_REQUIRED", 401);
    expect(screen.getByRole("link", { name: "Đăng nhập" })).toHaveAttribute("href", "/auth/sign-in");
  });
});
