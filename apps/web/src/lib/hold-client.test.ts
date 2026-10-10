import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  clearActiveHold,
  generateIdempotencyKey,
  getActiveHold,
  requestHold,
  saveActiveHold,
  type HoldResult,
} from "./hold-client";

describe("hold-client (EVF-113, BR-O2, BR-O5)", () => {
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    sessionStorage.clear();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  describe("generateIdempotencyKey", () => {
    it("sinh chuỗi uuid hợp lệ", () => {
      const key1 = generateIdempotencyKey();
      const key2 = generateIdempotencyKey();
      expect(key1).toBeTruthy();
      expect(key2).toBeTruthy();
      expect(key1).not.toBe(key2);
      expect(key1).toMatch(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);
    });
  });

  describe("sessionStorage helpers", () => {
    const mockHold: HoldResult = {
      orderId: "ord-123",
      holdId: "hold-456",
      expiresAt: "2026-10-10T12:00:00Z",
      ticketTypeId: "tier-ga",
      quantity: 2,
    };

    it("lưu và đọc lại active hold theo eventId", () => {
      expect(getActiveHold("evt-1")).toBeNull();
      saveActiveHold("evt-1", mockHold);
      expect(getActiveHold("evt-1")).toEqual(mockHold);
    });

    it("xoá active hold theo eventId", () => {
      saveActiveHold("evt-1", mockHold);
      clearActiveHold("evt-1");
      expect(getActiveHold("evt-1")).toBeNull();
    });
  });

  describe("requestHold", () => {
    it("gửi đúng endpoint, headers Idempotency-Key, X-Queue-Token, và body", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => ({
          order_id: "ord-test",
          hold_id: "hold-test",
          expires_at: "2026-10-10T10:10:00Z",
        }),
      });
      globalThis.fetch = mockFetch;

      const result = await requestHold({
        eventId: "evt-grand-concert",
        ticketTypeId: "tier-vip",
        quantity: 3,
        queueToken: "token-secret-123",
        identityId: "user-456",
        idempotencyKey: "fixed-uuid-key",
      });

      expect(mockFetch).toHaveBeenCalledTimes(1);
      const [url, init] = mockFetch.mock.calls[0];
      expect(url).toBe("/v1/events/evt-grand-concert/holds");
      expect(init.method).toBe("POST");
      expect(init.headers["Content-Type"]).toBe("application/json");
      expect(init.headers["Idempotency-Key"]).toBe("fixed-uuid-key");
      expect(init.headers["X-Queue-Token"]).toBe("token-secret-123");
      expect(init.headers["X-Identity-Id"]).toBe("user-456");
      expect(JSON.parse(init.body)).toEqual({
        ticket_type_id: "tier-vip",
        quantity: 3,
      });

      expect(result).toEqual({
        orderId: "ord-test",
        holdId: "hold-test",
        expiresAt: "2026-10-10T10:10:00Z",
        ticketTypeId: "tier-vip",
        quantity: 3,
      });
    });

    it("tự sinh Idempotency-Key nếu caller không truyền (BR-O5)", async () => {
      const mockFetch = vi.fn().mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => ({
          order_id: "ord-1",
          hold_id: "hold-1",
          expires_at: "2026-10-10T10:10:00Z",
        }),
      });
      globalThis.fetch = mockFetch;

      await requestHold({
        eventId: "evt-1",
        ticketTypeId: "tier-1",
        quantity: 1,
        queueToken: "token-1",
      });

      const init = mockFetch.mock.calls[0][1];
      expect(init.headers["Idempotency-Key"]).toBeTruthy();
      expect(init.headers["Idempotency-Key"]).toMatch(
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i,
      );
    });

    it("chuyển lỗi 403 thành NOT_ADMITTED", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 403,
      });

      await expect(
        requestHold({
          eventId: "evt-1",
          ticketTypeId: "tier-1",
          quantity: 1,
          queueToken: "token-invalid",
        }),
      ).rejects.toThrowError(
        expect.objectContaining({
          type: "NOT_ADMITTED",
          status: 403,
        }),
      );
    });

    it("chuyển lỗi 409 thành SOLD_OUT", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 409,
      });

      await expect(
        requestHold({
          eventId: "evt-1",
          ticketTypeId: "tier-1",
          quantity: 1,
          queueToken: "token-valid",
        }),
      ).rejects.toThrowError(
        expect.objectContaining({
          type: "SOLD_OUT",
          status: 409,
        }),
      );
    });

    it("chuyển lỗi 422 thành LIMIT_EXCEEDED", async () => {
      globalThis.fetch = vi.fn().mockResolvedValue({
        ok: false,
        status: 422,
      });

      await expect(
        requestHold({
          eventId: "evt-1",
          ticketTypeId: "tier-1",
          quantity: 10,
          queueToken: "token-valid",
        }),
      ).rejects.toThrowError(
        expect.objectContaining({
          type: "LIMIT_EXCEEDED",
          status: 422,
        }),
      );
    });

    it("chuyển lỗi mạng hoặc 503 thành AMBIGUOUS (BR-O5)", async () => {
      globalThis.fetch = vi.fn().mockRejectedValue(new Error("Network connection dropped"));

      await expect(
        requestHold({
          eventId: "evt-1",
          ticketTypeId: "tier-1",
          quantity: 1,
          queueToken: "token-valid",
        }),
      ).rejects.toThrowError(
        expect.objectContaining({
          type: "AMBIGUOUS",
        }),
      );
    });
  });
});
