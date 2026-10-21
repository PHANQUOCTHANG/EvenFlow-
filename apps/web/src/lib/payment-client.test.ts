import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  clearRecentOrder,
  generateIdempotencyKey,
  getOrderDetails,
  getRecentOrder,
  initiatePayment,
  PaymentClientError,
  saveRecentOrder,
  type OrderDetails,
} from "./payment-client";

describe("payment-client (EVF-114, BR-O2, BR-O5, BR-O6)", () => {
  beforeEach(() => {
    sessionStorage.clear();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    sessionStorage.clear();
    vi.unstubAllEnvs();
    vi.restoreAllMocks();
  });

  it("generateIdempotencyKey sinh chuỗi UUID v4 hợp lệ (BR-O5)", () => {
    const key1 = generateIdempotencyKey();
    const key2 = generateIdempotencyKey();

    expect(key1).toBeDefined();
    expect(key2).toBeDefined();
    expect(key1).not.toBe(key2);
    expect(key1).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i);
  });

  describe("initiatePayment", () => {
    it("gửi header Idempotency-Key và body chuẩn khi thanh toán thành công", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          payment_id: "pay_123",
          order_id: "ord_456",
          redirect_url: "https://payment-gateway.example/pay",
          status: "PENDING",
          provider_txn_id: "txn_789",
        }),
      });
      global.fetch = mockFetch;
      vi.stubEnv("NEXT_PUBLIC_API_BASE", "https://api.evenflow.test");

      const result = await initiatePayment({
        eventId: "evt-rock-2026",
        orderId: "ord_456",
        holdId: "hld_789",
        amount: 500000,
        method: "vnpay",
        buyerName: "Nguyen Van A",
        buyerEmail: "a@example.com",
        buyerPhone: "0901234567",
      });

      expect(mockFetch).toHaveBeenCalledTimes(1);
      const [url, options] = mockFetch.mock.calls[0];
      expect(url).toBe("https://api.evenflow.test/v1/events/evt-rock-2026/payments");
      expect(options.method).toBe("POST");
      expect(new Headers(options.headers).get("Idempotency-Key")).toBeTruthy();
      expect(JSON.parse(options.body)).toEqual({
        order_id: "ord_456",
        hold_id: "hld_789",
        amount: 500000,
        method: "vnpay",
        buyer_name: "Nguyen Van A",
        buyer_email: "a@example.com",
        buyer_phone: "0901234567",
        return_url: "/checkout/evt-rock-2026/result?orderId=ord_456",
      });

      expect(result.paymentId).toBe("pay_123");
      expect(result.orderId).toBe("ord_456");
      expect(result.status).toBe("PENDING");
    });

    it("ném lỗi HOLD_EXPIRED khi backend trả về HTTP 409 (BR-O2)", async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 409,
        json: async () => ({ detail: "hold expired" }),
      });

      await expect(
        initiatePayment({
          eventId: "evt-rock-2026",
          orderId: "ord_456",
          holdId: "hld_789",
          amount: 500000,
          method: "momo",
          buyerName: "Nguyen Van B",
          buyerEmail: "b@example.com",
          buyerPhone: "0901234567",
          apiBaseUrl: "https://api.evenflow.test",
        }),
      ).rejects.toThrowError(PaymentClientError);
    });

    it("ném lỗi AMBIGUOUS khi máy chủ trả HTTP 500 (BR-O5)", async () => {
      global.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 503,
      });

      try {
        await initiatePayment({
          eventId: "evt-rock-2026",
          orderId: "ord_456",
          holdId: "hld_789",
          amount: 500000,
          method: "vnpay",
          buyerName: "Nguyen Van C",
          buyerEmail: "c@example.com",
          buyerPhone: "0901234567",
          apiBaseUrl: "https://api.evenflow.test",
        });
        expect.unreachable("Phải ném lỗi");
      } catch (err) {
        expect(err).toBeInstanceOf(PaymentClientError);
        expect((err as PaymentClientError).type).toBe("AMBIGUOUS");
      }
    });

    it("WP2 (AC-12): chế độ sandbox khi không có backend KHÔNG giả thành công — reject NETWORK_ERROR", async () => {
      global.fetch = vi.fn().mockRejectedValue(new Error("Network failed"));

      await expect(
        initiatePayment({
          eventId: "evt-rock-2026",
          orderId: "ord_sandbox_1",
          holdId: "hld_sandbox_1",
          amount: 250000,
          method: "sandbox",
          buyerName: "Tester",
          buyerEmail: "test@example.com",
          buyerPhone: "0911222333",
        }),
      ).rejects.toMatchObject({ name: "PaymentClientError", type: "NETWORK_ERROR" });
    });
  });

  describe("getOrderDetails & Storage", () => {
    it("lấy chi tiết đơn hàng thành công từ API và lưu vào cache", async () => {
      const mockOrder: OrderDetails = {
        orderId: "ord_999",
        eventId: "evt-rock-2026",
        eventTitle: "Rock Fest 2026",
        venue: "Sân vận động Mỹ Đình",
        tierName: "VIP",
        quantity: 2,
        totalAmount: 1000000,
        status: "PAID",
        paidAt: "2026-10-10T20:30:00Z",
        paymentMethod: "vnpay",
        issuedTickets: [
          {
            ticketId: "tkt_1",
            ticketCode: "EF-VIP-001",
            tierName: "VIP",
            attendeeName: "Nguyen Van A",
          },
        ],
      };

      global.fetch = vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({
          order_id: mockOrder.orderId,
          event_id: mockOrder.eventId,
          event_title: mockOrder.eventTitle,
          venue: mockOrder.venue,
          tier_name: mockOrder.tierName,
          quantity: mockOrder.quantity,
          total_amount: mockOrder.totalAmount,
          status: "PAID",
          paid_at: mockOrder.paidAt,
          payment_method: mockOrder.paymentMethod,
          issued_tickets: mockOrder.issuedTickets,
        }),
      });

      const details = await getOrderDetails("ord_999", "evt-rock-2026");
      expect(details.orderId).toBe("ord_999");
      expect(details.status).toBe("PAID");
      expect(details.issuedTickets).toHaveLength(1);

      // Cache trong sessionStorage đã được cập nhật
      const cached = getRecentOrder("ord_999");
      expect(cached).toEqual(details);
    });

    it("WP2 (AC-14): lỗi mạng KHÔNG phục vụ cache sessionStorage như dữ liệu sống — reject NETWORK_ERROR", async () => {
      const mockOrder: OrderDetails = {
        orderId: "ord_offline",
        eventId: "evt-rock-2026",
        eventTitle: "Rock Fest",
        venue: "Mỹ Đình",
        tierName: "GA",
        quantity: 1,
        totalAmount: 300000,
        status: "PENDING",
      };
      saveRecentOrder(mockOrder);

      global.fetch = vi.fn().mockRejectedValue(new Error("Offline"));

      await expect(getOrderDetails("ord_offline", "evt-rock-2026")).rejects.toMatchObject({
        name: "PaymentClientError",
        type: "NETWORK_ERROR",
      });

      clearRecentOrder("ord_offline");
      expect(getRecentOrder("ord_offline")).toBeNull();
    });
  });
});

/** EV-198 B2: initiatePayment / getOrderDetails khong truyen apiBaseUrl phai goi
 *  `${NEXT_PUBLIC_API_BASE}/v1/...`. Nap lai module sau khi stub env. */
describe("payment-client — mặc định apiBaseUrl theo NEXT_PUBLIC_API_BASE (EV-198, B2)", () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    sessionStorage.clear();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.unstubAllEnvs();
    vi.resetModules();
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  async function loadFresh() {
    vi.resetModules();
    return import("./payment-client");
  }

  const paymentParams = {
    eventId: "evt-tech-summit-2026",
    orderId: "ord-b2",
    holdId: "hold-b2",
    amount: 5_000_000,
    method: "vnpay" as const,
    buyerName: "Nguyen Van A",
    buyerEmail: "a@example.com",
    buyerPhone: "0901234567",
  };

  it("initiatePayment không truyền apiBaseUrl gọi ${NEXT_PUBLIC_API_BASE}/v1/events/{id}/payments", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE", "http://localhost:8080");
    const fresh = await loadFresh();

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ payment_id: "pay-b2", order_id: "ord-b2", status: "PENDING" }),
    });
    globalThis.fetch = mockFetch;

    await fresh.initiatePayment(paymentParams);

    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(mockFetch.mock.calls[0][0]).toBe(
      "http://localhost:8080/v1/events/evt-tech-summit-2026/payments",
    );
  });

  it("WP2: initiatePayment bỏ qua apiBaseUrl truyền vào — gốc API luôn từ NEXT_PUBLIC_API_BASE", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE", "http://localhost:8080");
    const fresh = await loadFresh();

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ payment_id: "pay-b2", order_id: "ord-b2", status: "PENDING" }),
    });
    globalThis.fetch = mockFetch;

    await fresh.initiatePayment({ ...paymentParams, apiBaseUrl: "https://explicit.example" });

    expect(mockFetch.mock.calls[0][0]).toBe(
      "http://localhost:8080/v1/events/evt-tech-summit-2026/payments",
    );
  });

  it("getOrderDetails không truyền apiBaseUrl gọi ${NEXT_PUBLIC_API_BASE}/v1/orders/{orderId}", async () => {
    vi.stubEnv("NEXT_PUBLIC_API_BASE", "http://localhost:8080");
    const fresh = await loadFresh();

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ order_id: "ord-b2", status: "PAID" }),
    });
    globalThis.fetch = mockFetch;

    await fresh.getOrderDetails("ord-b2", "evt-tech-summit-2026");

    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(mockFetch.mock.calls[0][0]).toBe(
      "http://localhost:8080/v1/orders/ord-b2",
    );
  });

  it("getOrderDetails gửi Authorization khi đã đăng nhập, không gửi khi chưa", async () => {
    const fresh = await loadFresh();
    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ order_id: "ord-b2", status: "PAID" }),
    });
    globalThis.fetch = mockFetch;

    window.localStorage.clear();
    window.sessionStorage.clear();
    await fresh.getOrderDetails("ord-b2", "evt-1");
    expect(new Headers(mockFetch.mock.calls[0][1].headers).has("Authorization")).toBe(false);

    window.localStorage.setItem("ef_token", "tok-abc");
    window.localStorage.setItem("ef_user", JSON.stringify({ id: "u-1", role: "buyer" }));
    await fresh.getOrderDetails("ord-b2", "evt-1");
    expect(new Headers(mockFetch.mock.calls[1][1].headers).get("Authorization")).toBe("Bearer tok-abc");
    window.localStorage.clear();
  });
});
