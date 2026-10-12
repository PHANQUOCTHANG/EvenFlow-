/**
 * Unit test cho queue-client.
 *
 * Hanh vi quan trong nhat (docs/02, Tang 2): join bi hoan ngau nhien trong
 * [0, 5000) ms. Nhip poll duoc test o lib/queue-status.test.ts va
 * hooks/use-queue-status.test.tsx.
 *
 * fetch duoc mock hoan toan; dung fake timer de khong phai cho that.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  QueueError,
  jitterDelay,
  joinQueue,
} from "./queue-client";

const EVENT = "evt-1";

function jsonResponse(
  body: unknown,
  init: { status?: number; headers?: Record<string, string> } = {},
): Response {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { "Content-Type": "application/json", ...init.headers },
  });
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("jitterDelay", () => {
  it("luon nam trong [0, max)", () => {
    for (let i = 0; i < 1000; i++) {
      const d = jitterDelay(5000);
      expect(d).toBeGreaterThanOrEqual(0);
      expect(d).toBeLessThan(5000);
      expect(Number.isInteger(d)).toBe(true);
    }
  });

  it("phan bo trai deu, khong don cuc o giay dau (issue 1412)", () => {
    const buckets = [0, 0, 0, 0, 0];
    for (let i = 0; i < 5000; i++) buckets[Math.floor(jitterDelay(5000) / 1000)]++;
    // Moi giay ~1000 mau; lech qua 30% la dau hieu random bi lech.
    for (const n of buckets) {
      expect(n).toBeGreaterThan(700);
      expect(n).toBeLessThan(1300);
    }
  });
});

describe("joinQueue", () => {
  it("goi dung endpoint va tra ve JoinResponse", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({
        state: "QUEUED",
        queue_token: "tok",
        rank: 5,
        is_new: true,
        poll_after_ms: 3000,
      }),
    );

    const res = await joinQueue(EVENT, { fp: "abc" }, { jitter: false });

    expect(res.queue_token).toBe("tok");
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toContain(`/v1/events/${EVENT}/queue/join`);
    expect(init?.method).toBe("POST");
    expect(JSON.parse(String(init?.body))).toEqual({ signals: { fp: "abc" } });
  });

  it("mac dinh co jitter: chua goi fetch truoc khi het thoi gian hoan", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0.5); // -> 2500ms
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ state: "QUEUED", queue_token: "t", rank: 1, is_new: true, poll_after_ms: 1 }),
    );

    const p = joinQueue(EVENT, {});
    await vi.advanceTimersByTimeAsync(2499);
    expect(fetchMock).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(1);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await p;
  });

  it("nem QueueError kem status va detail khi server tu choi", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      jsonResponse({ detail: "chua xac thuc OTP" }, { status: 403 }),
    );

    await expect(joinQueue(EVENT, {}, { jitter: false })).rejects.toMatchObject({
      name: "QueueError",
      status: 403,
      message: "chua xac thuc OTP",
    });
  });

  it("QueueError la instance cua Error", () => {
    const e = new QueueError(429, "qua nhieu");
    expect(e).toBeInstanceOf(Error);
    expect(e.status).toBe(429);
  });
});
