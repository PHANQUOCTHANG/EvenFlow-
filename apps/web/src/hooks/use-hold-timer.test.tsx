import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as holdClient from "@/lib/hold-client";
import { useHoldTimer } from "./use-hold-timer";

describe("useHoldTimer (EVF-113, BR-O2, BR-O5)", () => {
  const eventId = "evt-grand-concert";
  // EV-198 (plan-review F1): khoa thong nhat voi phong cho (lib/queue-token.ts).
  const queueTokenKey = `ef:queue:token:${eventId}`;

  beforeEach(() => {
    sessionStorage.clear();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("khởi tạo ở trạng thái idle khi chưa có hold trong storage", () => {
    const { result } = renderHook(() => useHoldTimer({ eventId }));
    expect(result.current.holdState).toBe("idle");
    expect(result.current.hold).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it("khôi phục active hold từ sessionStorage nếu còn hạn (BR-O2)", () => {
    const futureExpiresAt = new Date(Date.now() + 600_000).toISOString();
    holdClient.saveActiveHold(eventId, {
      orderId: "ord-stored",
      holdId: "hold-stored",
      expiresAt: futureExpiresAt,
      ticketTypeId: "tier-ga",
      quantity: 2,
    });

    const { result } = renderHook(() => useHoldTimer({ eventId }));
    expect(result.current.holdState).toBe("active");
    expect(result.current.hold).toEqual({
      orderId: "ord-stored",
      holdId: "hold-stored",
      expiresAt: futureExpiresAt,
      ticketTypeId: "tier-ga",
      quantity: 2,
    });
  });

  it("đánh dấu expired nếu hold trong sessionStorage đã quá hạn", () => {
    const pastExpiresAt = new Date(Date.now() - 10_000).toISOString();
    holdClient.saveActiveHold(eventId, {
      orderId: "ord-past",
      holdId: "hold-past",
      expiresAt: pastExpiresAt,
      ticketTypeId: "tier-ga",
      quantity: 1,
    });

    const { result } = renderHook(() => useHoldTimer({ eventId }));
    expect(result.current.holdState).toBe("expired");
    expect(result.current.hold).toBeNull();
  });

  it("từ chối tạo hold nếu không có queueToken (chưa qua phòng chờ)", async () => {
    const { result } = renderHook(() => useHoldTimer({ eventId }));

    let success: boolean = false;
    await act(async () => {
      success = await result.current.createHold("tier-1", 1);
    });

    expect(success).toBe(false);
    expect(result.current.holdState).toBe("idle");
    expect(result.current.error?.type).toBe("NOT_ADMITTED");
  });

  it("tạo hold thành công: chuyển sang active và lưu storage (BR-O2, BR-O5)", async () => {
    sessionStorage.setItem(queueTokenKey, "mock-queue-token");

    const futureExpiresAt = new Date(Date.now() + 600_000).toISOString();
    const requestHoldSpy = vi.spyOn(holdClient, "requestHold").mockResolvedValue({
      orderId: "ord-new",
      holdId: "hold-new",
      expiresAt: futureExpiresAt,
      ticketTypeId: "tier-vip",
      quantity: 2,
    });

    const { result } = renderHook(() => useHoldTimer({ eventId }));

    let success: boolean = false;
    await act(async () => {
      success = await result.current.createHold("tier-vip", 2);
    });

    expect(success).toBe(true);
    expect(requestHoldSpy).toHaveBeenCalledWith(
      expect.objectContaining({
        eventId,
        ticketTypeId: "tier-vip",
        quantity: 2,
        queueToken: "mock-queue-token",
      }),
    );
    expect(result.current.holdState).toBe("active");
    expect(result.current.hold?.holdId).toBe("hold-new");

    // Kiem tra storage
    const stored = holdClient.getActiveHold(eventId);
    expect(stored?.holdId).toBe("hold-new");
  });

  it("chuyển sang sold_out khi nhận lỗi 409 từ server", async () => {
    sessionStorage.setItem(queueTokenKey, "mock-queue-token");

    vi.spyOn(holdClient, "requestHold").mockRejectedValue(
      new holdClient.HoldClientError("Hết vé", "SOLD_OUT", 409),
    );

    const { result } = renderHook(() => useHoldTimer({ eventId }));

    let success: boolean = false;
    await act(async () => {
      success = await result.current.createHold("tier-1", 1);
    });

    expect(success).toBe(false);
    expect(result.current.holdState).toBe("sold_out");
    expect(result.current.error?.type).toBe("SOLD_OUT");
  });

  it("chuyển sang ambiguous khi nhận lỗi mạng / không rõ kết quả (BR-O5)", async () => {
    sessionStorage.setItem(queueTokenKey, "mock-queue-token");

    vi.spyOn(holdClient, "requestHold").mockRejectedValue(
      new holdClient.HoldClientError("Mất mạng", "AMBIGUOUS"),
    );

    const { result } = renderHook(() => useHoldTimer({ eventId }));

    let success: boolean = false;
    await act(async () => {
      success = await result.current.createHold("tier-1", 1);
    });

    expect(success).toBe(false);
    expect(result.current.holdState).toBe("ambiguous");
  });

  it("resetHold giải phóng state về idle và xoá storage", () => {
    const futureExpiresAt = new Date(Date.now() + 600_000).toISOString();
    holdClient.saveActiveHold(eventId, {
      orderId: "ord-test",
      holdId: "hold-test",
      expiresAt: futureExpiresAt,
      ticketTypeId: "tier-ga",
      quantity: 1,
    });

    const { result } = renderHook(() => useHoldTimer({ eventId }));
    expect(result.current.holdState).toBe("active");

    act(() => {
      result.current.resetHold();
    });

    expect(result.current.holdState).toBe("idle");
    expect(result.current.hold).toBeNull();
    expect(holdClient.getActiveHold(eventId)).toBeNull();
  });

  it("Đổi giờ máy khách không kéo dài được thời gian giữ ghế (BR-O2)", () => {
    // Giả sử máy khách bị chỉnh lùi 10 phút (offsetMs = +600,000ms theo giờ server)
    // Server expiresAt chỉ còn 2 phút (120,000ms) tính theo giờ server
    const offsetMs = 600_000;
    const serverNowMs = Date.now() + offsetMs;
    const expiresAt = new Date(serverNowMs + 120_000).toISOString();

    holdClient.saveActiveHold(eventId, {
      orderId: "ord-time-sync",
      holdId: "hold-time-sync",
      expiresAt,
      ticketTypeId: "tier-ga",
      quantity: 1,
    });

    const { result } = renderHook(() => useHoldTimer({ eventId, offsetMs }));

    // remainingMs phải tính theo giờ server (~120s), không bị giờ máy khách kéo dài thành 720s
    expect(result.current.holdState).toBe("active");
    expect(result.current.remainingMs).toBeLessThanOrEqual(120_000);
    expect(result.current.remainingMs).toBeGreaterThan(110_000);

    // Trường hợp máy khách chỉnh lùi đồng hồ nhưng theo giờ server ĐÃ HẾT HẠN
    const expiredServerTimestamp = new Date(serverNowMs - 5_000).toISOString();
    holdClient.saveActiveHold(eventId, {
      orderId: "ord-expired-sync",
      holdId: "hold-expired-sync",
      expiresAt: expiredServerTimestamp,
      ticketTypeId: "tier-ga",
      quantity: 1,
    });

    const expiredResult = renderHook(() => useHoldTimer({ eventId, offsetMs }));
    expect(expiredResult.result.current.holdState).toBe("expired");
  });

  it("Bấm đặt nhiều lần không tạo hold trùng (BR-O5)", async () => {
    sessionStorage.setItem(queueTokenKey, "mock-queue-token");

    let resolvePromise: (value: holdClient.HoldResult) => void;
    const pendingPromise = new Promise<holdClient.HoldResult>((resolve) => {
      resolvePromise = resolve;
    });

    const requestHoldSpy = vi.spyOn(holdClient, "requestHold").mockImplementation(() => pendingPromise);

    const { result } = renderHook(() => useHoldTimer({ eventId }));

    // Gọi liên tiếp 2 lần đồng thời trước khi request thứ nhất hoàn thành
    const call1 = result.current.createHold("tier-1", 1);
    const call2 = result.current.createHold("tier-1", 1);

    // Lần 2 phải bị từ chối ngay lập tức và chỉ gọi requestHold đúng 1 lần
    expect(await call2).toBe(false);
    expect(requestHoldSpy).toHaveBeenCalledTimes(1);

    // Hoàn tất request 1 trong act() để flush state
    await act(async () => {
      resolvePromise!({
        orderId: "ord-once",
        holdId: "hold-once",
        expiresAt: new Date(Date.now() + 600_000).toISOString(),
        ticketTypeId: "tier-1",
        quantity: 1,
      });
      await call1;
    });

    expect(result.current.holdState).toBe("active");
    expect(requestHoldSpy).toHaveBeenCalledTimes(1);
  });
});


/** EV-198 B1: token phong cho ghi (khoa `ef:queue:token:{eventId}`, xem waiting-room.tsx va
 *  lib/queue-token.ts) phai duoc createHold doc ra va gui di trong header X-Queue-Token.
 *  Ghi bang khoa thuan (khong import queue-token.ts) de file nay van bien dich tren code cu. */
describe("useHoldTimer — đọc token phòng chờ (EV-198, B1)", () => {
  const eventId = "evt-tech-summit-2026";
  const originalFetch = globalThis.fetch;

  beforeEach(() => {
    sessionStorage.clear();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    globalThis.fetch = originalFetch;
    sessionStorage.clear();
    vi.restoreAllMocks();
  });

  it("token phòng chờ lưu ở khoá ef:queue:token:{eventId} được createHold gửi trong X-Queue-Token", async () => {
    // Dung y nhu phong cho ghi sau khi join thanh cong.
    sessionStorage.setItem(`ef:queue:token:${eventId}`, "tok-from-waiting-room");

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => ({
        order_id: "ord-b1",
        hold_id: "hold-b1",
        expires_at: new Date(Date.now() + 600_000).toISOString(),
      }),
    });
    globalThis.fetch = mockFetch;

    const { result } = renderHook(() => useHoldTimer({ eventId }));

    let success = false;
    await act(async () => {
      success = await result.current.createHold("tier-all-access", 2);
    });

    expect(result.current.error?.type).not.toBe("NOT_ADMITTED");
    expect(success).toBe(true);
    expect(mockFetch).toHaveBeenCalledTimes(1);
    const [url, init] = mockFetch.mock.calls[0];
    expect(String(url)).toMatch(/\/v1\/events\/evt-tech-summit-2026\/holds$/);
    expect(init.headers["X-Queue-Token"]).toBe("tok-from-waiting-room");
    expect(result.current.holdState).toBe("active");
    expect(result.current.hold?.holdId).toBe("hold-b1");
  });

  it("queueToken truyền qua prop vẫn được ưu tiên hơn token trong storage", async () => {
    sessionStorage.setItem(`ef:queue:token:${eventId}`, "tok-from-waiting-room");

    const mockFetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 201,
      json: async () => ({
        order_id: "ord-b1p",
        hold_id: "hold-b1p",
        expires_at: new Date(Date.now() + 600_000).toISOString(),
      }),
    });
    globalThis.fetch = mockFetch;

    const { result } = renderHook(() => useHoldTimer({ eventId, queueToken: "tok-from-prop" }));

    await act(async () => {
      await result.current.createHold("tier-all-access", 1);
    });

    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(mockFetch.mock.calls[0][1].headers["X-Queue-Token"]).toBe("tok-from-prop");
  });
});
