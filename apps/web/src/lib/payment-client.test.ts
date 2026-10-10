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

      const result = await initiatePayment({
        eventId: "evt-rock-2026",
        orderId: "ord_456",
        holdId: "hld_789",
        amount: 500000,
        method: "vnpay",
        buyerName: "Nguyen Van A",
        buyerEmail: "a@example.com",
        buyerPhone: "0901234567",
        apiBaseUrl: "https://api.evenflow.test",
      });

      expect(mockFetch).toHaveBeenCalledTimes(1);
      const [url, options] = mockFetch.mock.calls[0];
      expect(url).toBe("https://api.evenflow.test/v1/events/evt-rock-2026/payments");
      expect(options.method).toBe("POST");
      expect(options.headers["Idempotency-Key"]).toBeDefined();
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

    it("chế độ sandbox mock trả về kết quả redirect thành công khi không có backend", async () => {
      global.fetch = vi.fn().mockRejectedValue(new Error("Network failed"));

      const result = await initiatePayment({
        eventId: "evt-rock-2026",
        orderId: "ord_sandbox_1",
        holdId: "hld_sandbox_1",
        amount: 250000,
        method: "sandbox",
        buyerName: "Tester",
        buyerEmail: "test@example.com",
        buyerPhone: "0911222333",
      });

      expect(result.orderId).toBe("ord_sandbox_1");
      expect(result.status).toBe("PENDING");
      expect(result.redirectUrl).toContain("/checkout/evt-rock-2026/result?orderId=ord_sandbox_1");
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
            qrPayload: "DEMO-TICKET-EF-VIP-001",
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

      const details = await getOrderDetails("ord_999", "evt-rock-2026", "https://api.test");
      expect(details.orderId).toBe("ord_999");
      expect(details.status).toBe("PAID");
      expect(details.issuedTickets).toHaveLength(1);

      // Cache trong sessionStorage đã được cập nhật
      const cached = getRecentOrder("ord_999");
      expect(cached).toEqual(details);
    });

    it("lấy từ sessionStorage khi fetch bị lỗi mạng", async () => {
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

      const res = await getOrderDetails("ord_offline", "evt-rock-2026", "https://api.test");
      expect(res.orderId).toBe("ord_offline");
      expect(res.status).toBe("PENDING");

      clearRecentOrder("ord_offline");
      expect(getRecentOrder("ord_offline")).toBeNull();
    });
  });
});
