import { act, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as holdClient from "@/lib/hold-client";
import type { EventDetailSnapshot } from "@/lib/types/event";
import { buildPaymentUrl, CheckoutView } from "./checkout-view";

const mockEvent: EventDetailSnapshot = {
  id: "evt-test-1",
  slug: "evt-test-1",
  title: "Đêm Nhạc Ánh Sáng 2026",
  description: "Mô tả sự kiện",
  venue: "Sân vận động Mỹ Đình",
  startsAt: "2026-11-20T19:00:00+07:00",
  saleStartAt: "2026-10-15T10:00:00+07:00",
  state: "ON_SALE",
  maxPerOrder: 4,
  tiers: [
    {
      id: "tier-ga",
      name: "Vé Tiêu Chuẩn (GA)",
      price: 650000,
      currency: "VND",
      availability: "AVAILABLE",
    },
    {
      id: "tier-vip",
      name: "Vé VIP",
      price: 1800000,
      currency: "VND",
      availability: "FEW_LEFT",
    },
    {
      id: "tier-svip",
      name: "Vé SVIP",
      price: 3200000,
      currency: "VND",
      availability: "SOLD_OUT",
    },
  ],
};

describe("CheckoutView (EVF-113, EV-183)", () => {
  beforeEach(() => {
    sessionStorage.clear();
    // EV-198 (plan-review F1): khoa thong nhat voi phong cho (lib/queue-token.ts).
    sessionStorage.setItem("ef:queue:token:evt-test-1", "mock-queue-token");
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

  it("bấm đặt nhiều lần nhanh trên UI không tạo hold trùng (BR-O5)", async () => {
    let resolvePromise: (value: holdClient.HoldResult) => void;
    const pendingPromise = new Promise<holdClient.HoldResult>((resolve) => {
      resolvePromise = resolve;
    });

    const requestHoldSpy = vi.spyOn(holdClient, "requestHold").mockImplementation(() => pendingPromise);

    await act(async () => {
      render(<CheckoutView event={mockEvent} />);
    });

    const submitBtn = screen.getByRole("button", { name: "Giữ vé và thanh toán" });

    // Bấm liên tiếp nhiều lần nhanh
    await act(async () => {
      fireEvent.click(submitBtn);
      fireEvent.click(submitBtn);
      fireEvent.click(submitBtn);
    });

    // Chỉ gọi duy nhất 1 lần
    expect(requestHoldSpy).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolvePromise!({
        orderId: "ord-ui-once",
        holdId: "hold-ui-once",
        expiresAt: new Date(Date.now() + 600_000).toISOString(),
        ticketTypeId: "tier-ga",
        quantity: 1,
      });
    });

    expect(requestHoldSpy).toHaveBeenCalledTimes(1);
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

/** EV-198 B3: khi da co hold active va khong co onProceedToPayment, CheckoutView phai dua khach
 *  sang trang thanh toan voi URL kem tierId + quantity cua hold (payment/page.tsx doc 2 tham so
 *  nay). Assert theo search params, khong phu thuoc thu tu tham so hay cach ma hoa. */
describe("CheckoutView — URL sang thanh toán mang tierId và quantity của hold (EV-198, B3)", () => {
  let navigatedTo: string[] = [];

  function stubLocation() {
    navigatedTo = [];
    const loc = {
      origin: "http://localhost:3000",
      protocol: "http:",
      host: "localhost:3000",
      hostname: "localhost",
      port: "3000",
      pathname: "/checkout/evt-test-1",
      search: "",
      hash: "",
      reload: vi.fn(),
      assign: vi.fn((url: string) => {
        navigatedTo.push(String(url));
      }),
      replace: vi.fn((url: string) => {
        navigatedTo.push(String(url));
      }),
      toString: () => "http://localhost:3000/checkout/evt-test-1",
    } as Record<string, unknown>;
    let href = "http://localhost:3000/checkout/evt-test-1";
    Object.defineProperty(loc, "href", {
      get: () => href,
      set: (value: string) => {
        href = String(value);
        navigatedTo.push(String(value));
      },
      enumerable: true,
    });
    vi.stubGlobal("location", loc);
  }

  beforeEach(() => {
    sessionStorage.clear();
    vi.restoreAllMocks();
    stubLocation();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it("bấm 'Tiếp tục thanh toán' chuyển sang /checkout/{id}/payment kèm orderId, holdId, tierId, quantity của hold", async () => {
    holdClient.saveActiveHold("evt-test-1", {
      orderId: "ord-b3",
      holdId: "hold-b3",
      expiresAt: new Date(Date.now() + 600_000).toISOString(),
      ticketTypeId: "tier-vip",
      quantity: 3,
    });

    await act(async () => {
      render(<CheckoutView event={mockEvent} />);
    });

    const continueBtn = screen.getByRole("button", { name: "Tiếp tục thanh toán" });
    await act(async () => {
      fireEvent.click(continueBtn);
    });

    expect(navigatedTo).toHaveLength(1);
    const url = new URL(navigatedTo[0], "http://localhost:3000");
    expect(url.pathname).toBe("/checkout/evt-test-1/payment");
    expect(url.searchParams.get("orderId")).toBe("ord-b3");
    expect(url.searchParams.get("holdId")).toBe("hold-b3");
    expect(url.searchParams.get("tierId")).toBe("tier-vip");
    expect(url.searchParams.get("quantity")).toBe("3");
  });
});

/** EV-198 B3 (plan-review F13): helper thuan `buildPaymentUrl` tra ve URL tuong doi sang trang
 *  thanh toan kem orderId, holdId, tierId, quantity. */
describe("buildPaymentUrl (EV-198, B3)", () => {
  it("trả về /checkout/{eventId}/payment kèm orderId, holdId, tierId, quantity", () => {
    const url = new URL(
      buildPaymentUrl("evt-tech-summit-2026", {
        orderId: "ord-1",
        holdId: "hold-1",
        ticketTypeId: "tier-all-access",
        quantity: 2,
      }),
      "http://localhost:3000",
    );

    expect(url.origin).toBe("http://localhost:3000");
    expect(url.pathname).toBe("/checkout/evt-tech-summit-2026/payment");
    expect(url.searchParams.get("orderId")).toBe("ord-1");
    expect(url.searchParams.get("holdId")).toBe("hold-1");
    expect(url.searchParams.get("tierId")).toBe("tier-all-access");
    expect(url.searchParams.get("quantity")).toBe("2");
  });

  it("là URL tương đối (bắt đầu bằng /checkout/), không gắn origin", () => {
    const raw = buildPaymentUrl("evt-1", {
      orderId: "o",
      holdId: "h",
      ticketTypeId: "t",
      quantity: 1,
    });
    expect(raw.startsWith("/checkout/evt-1/payment?")).toBe(true);
  });

  it("mã hoá an toàn giá trị có ký tự đặc biệt (đọc lại đúng, không chèn thêm tham số)", () => {
    const url = new URL(
      buildPaymentUrl("evt-1", {
        orderId: "ord b3&x=1",
        holdId: "hold/b3?#",
        ticketTypeId: "tier-ga",
        quantity: 3,
      }),
      "http://localhost:3000",
    );

    expect(url.pathname).toBe("/checkout/evt-1/payment");
    expect(url.searchParams.get("orderId")).toBe("ord b3&x=1");
    expect(url.searchParams.get("holdId")).toBe("hold/b3?#");
    expect(url.searchParams.get("tierId")).toBe("tier-ga");
    expect(url.searchParams.get("quantity")).toBe("3");
    expect(url.searchParams.get("x")).toBeNull();
    expect(url.hash).toBe("");
  });
});
