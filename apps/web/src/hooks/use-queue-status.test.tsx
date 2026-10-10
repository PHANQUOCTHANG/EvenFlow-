import { renderHook, act } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { QueueError } from "@/lib/queue-client";
import { useQueueStatus } from "./use-queue-status";

function jsonResponse(data: unknown, init: { status?: number; headers?: Record<string, string> } = {}) {
  return new Response(JSON.stringify(data), {
    status: init.status ?? 200,
    headers: { "Content-Type": "application/json", ...init.headers },
  });
}

describe("useQueueStatus — Hook điều phối trạng thái phòng chờ (EV-182)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    window.sessionStorage.clear();
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    window.sessionStorage.clear();
  });

  it("chưa có token thì trả về trạng thái ban đầu, không fetch", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch");
    const { result } = renderHook(() =>
      useQueueStatus({ eventId: "evt-1", token: null })
    );

    expect(result.current.snapshot).toBeNull();
    expect(result.current.transport).toBe("idle");
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("AC-1: gọi fetch khi có token và cập nhật snapshot từ 200", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({
        state: "QUEUED",
        rank: 1500,
        queue_depth: 10000,
        admit_rate: 50,
        eta_seconds: 30,
        poll_after_ms: 5000,
      })
    );

    const { result } = renderHook(() =>
      useQueueStatus({ eventId: "evt-1", token: "tok-1", enableSse: false })
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(result.current.snapshot?.state).toBe("QUEUED");
    expect(result.current.snapshot?.rank).toBe(1500);
    expect(result.current.initialRank).toBe(1500);
    expect(result.current.connection).toBe("live");
    expect(result.current.transport).toBe("polling");
  });

  it("AC-1: tôn trọng poll_after_ms, chưa đến hẹn không poll lại", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        jsonResponse({
          state: "QUEUED",
          rank: 1500,
          poll_after_ms: 5000,
        })
      )
      .mockResolvedValueOnce(
        jsonResponse({
          state: "QUEUED",
          rank: 1200,
          poll_after_ms: 5000,
        })
      );

    const { result } = renderHook(() =>
      useQueueStatus({ eventId: "evt-1", token: "tok-1", enableSse: false })
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // Chưa hết 5000ms: không gọi lần 2
    await act(async () => {
      await vi.advanceTimersByTimeAsync(4999);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // Đúng 5000ms: gọi lần 2
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.current.snapshot?.rank).toBe(1200);
  });

  it("AC-1: 304 Not Modified giữ ETag và đợi theo X-Poll-After-Ms", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        jsonResponse(
          { state: "QUEUED", rank: 1000, poll_after_ms: 4000 },
          { headers: { ETag: '"tag-v1"' } }
        )
      )
      .mockResolvedValueOnce(
        new Response(null, {
          status: 304,
          headers: { ETag: '"tag-v1"', "X-Poll-After-Ms": "8000" },
        })
      );

    const { result } = renderHook(() =>
      useQueueStatus({ eventId: "evt-1", token: "tok-1", enableSse: false })
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(4000);
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.current.snapshot?.rank).toBe(1000);

    // Lần 3 phải đợi 8000ms
    await act(async () => {
      await vi.advanceTimersByTimeAsync(7999);
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("AC-1: 503 Retry-After đợi theo chỉ định của server", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        new Response(null, { status: 503, headers: { "Retry-After": "6" } })
      )
      .mockResolvedValueOnce(
        jsonResponse({ state: "QUEUED", rank: 800, poll_after_ms: 5000 })
      );

    const { result } = renderHook(() =>
      useQueueStatus({ eventId: "evt-1", token: "tok-1", enableSse: false })
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5999);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.current.snapshot?.rank).toBe(800);
  });

  it("AC-2: Lỗi mạng backoff exponential và đổi trạng thái kết nối", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0);
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockRejectedValueOnce(new TypeError("Network error"))
      .mockRejectedValueOnce(new TypeError("Network error"))
      .mockResolvedValueOnce(
        jsonResponse({ state: "QUEUED", rank: 500, poll_after_ms: 5000 })
      );

    const { result } = renderHook(() =>
      useQueueStatus({ eventId: "evt-1", token: "tok-1", enableSse: false })
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(result.current.connection).toBe("reconnecting");

    // Lần 1 backoff 1s
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.current.connection).toBe("reconnecting");

    // Lần 2 backoff 2s
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(result.current.connection).toBe("live");
    expect(result.current.snapshot?.rank).toBe(500);
  });

  it("AC-3: Tab ẩn hay focus KHÔNG tạo thêm request ngoài nhịp server", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ state: "QUEUED", rank: 100, poll_after_ms: 10000 })
    );

    renderHook(() =>
      useQueueStatus({ eventId: "evt-1", token: "tok-1", enableSse: false })
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // Giả lập visibilitychange hoặc window focus
    act(() => {
      Object.defineProperty(document, "visibilityState", {
        value: "hidden",
        configurable: true,
      });
      document.dispatchEvent(new Event("visibilitychange"));
      window.dispatchEvent(new Event("focus"));
    });

    // Vẫn chỉ có 1 request
    expect(fetchMock).toHaveBeenCalledTimes(1);

    act(() => {
      Object.defineProperty(document, "visibilityState", {
        value: "visible",
        configurable: true,
      });
      document.dispatchEvent(new Event("visibilitychange"));
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("AC-4: Tự động nâng cấp lên SSE khi poll_after_ms <= 3000", async () => {
    const enc = new TextEncoder();
    const stream = new ReadableStream({
      start(controller) {
        controller.enqueue(
          enc.encode('event: position\ndata: {"state":"QUEUED","rank":40,"poll_after_ms":2000}\n\n')
        );
      },
    });

    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        jsonResponse({ state: "QUEUED", rank: 100, poll_after_ms: 3000 })
      )
      .mockResolvedValueOnce(new Response(stream));

    const { result } = renderHook(() =>
      useQueueStatus({ eventId: "evt-1", token: "tok-1", enableSse: true, sseThresholdMs: 3000 })
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(result.current.transport).toBe("sse");
    expect(result.current.snapshot?.rank).toBe(40);
  });

  it("AC-7: 4xx dừng polling, đánh dấu error", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ detail: "token expired" }, { status: 401 })
    );

    const { result } = renderHook(() =>
      useQueueStatus({ eventId: "evt-1", token: "bad-tok", enableSse: false })
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(result.current.error).toBeInstanceOf(QueueError);
    expect(result.current.error?.status).toBe(401);
    expect(result.current.transport).toBe("idle");

    // Không poll lại sau đó
    await act(async () => {
      await vi.advanceTimersByTimeAsync(30000);
    });
    expect(result.current.transport).toBe("idle");
  });

  it("Trạng thái terminal (ADMITTED) dừng chu kỳ cập nhật", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ state: "ADMITTED", rank: 0, expires_at: 1800000000000 })
    );

    const { result } = renderHook(() =>
      useQueueStatus({ eventId: "evt-1", token: "tok-1", enableSse: false })
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(result.current.snapshot?.state).toBe("ADMITTED");
    expect(result.current.transport).toBe("idle");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(60000);
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
