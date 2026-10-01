/**
 * Unit test cho queue-client.
 *
 * Hai hanh vi quan trong nhat (docs/02, Tang 2 va Tang 4):
 *   - jitter: join bi hoan ngau nhien trong [0, 5000) ms
 *   - poll: client TUAN THU poll_after_ms / Retry-After / X-Poll-After-Ms
 *     tu server, khong tu chon chu ky.
 *
 * fetch duoc mock hoan toan; dung fake timer de khong phai cho that.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  QueueError,
  jitterDelay,
  joinQueue,
  watchQueue,
  type QueuePosition,
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

function position(over: Partial<QueuePosition> = {}): QueuePosition {
  return {
    state: "QUEUED",
    rank: 1200,
    queue_depth: 30000,
    admit_rate: 200,
    eta_seconds: 6,
    poll_after_ms: 3000,
    ...over,
  };
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

describe("watchQueue", () => {
  it("yield vi tri va dung khi ADMITTED", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(jsonResponse(position({ rank: 100, poll_after_ms: 3000 })))
      .mockResolvedValueOnce(jsonResponse(position({ state: "ADMITTED", rank: 0 })));

    const seen: QueuePosition[] = [];
    const run = (async () => {
      for await (const p of watchQueue(EVENT, "tok")) seen.push(p);
    })();

    await vi.advanceTimersByTimeAsync(0);
    expect(seen).toHaveLength(1);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // Chua het poll_after_ms thi KHONG duoc goi lai.
    await vi.advanceTimersByTimeAsync(2999);
    expect(fetchMock).toHaveBeenCalledTimes(1);

    await vi.advanceTimersByTimeAsync(1);
    await run;
    expect(seen).toHaveLength(2);
    expect(seen[1].state).toBe("ADMITTED");
  });

  it("gui X-Queue-Token va If-None-Match, ton trong X-Poll-After-Ms khi 304", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(
        jsonResponse(position({ poll_after_ms: 1000 }), { headers: { ETag: '"v1"' } }),
      )
      .mockResolvedValueOnce(
        new Response(null, { status: 304, headers: { "X-Poll-After-Ms": "7000" } }),
      )
      .mockResolvedValueOnce(jsonResponse(position({ state: "EXPIRED" })));

    const run = (async () => {
      const out: QueuePosition[] = [];
      for await (const p of watchQueue(EVENT, "tok")) out.push(p);
      return out;
    })();

    await vi.advanceTimersByTimeAsync(1000); // sau lan 1 -> goi lan 2 (304)
    expect(fetchMock).toHaveBeenCalledTimes(2);
    const headers2 = fetchMock.mock.calls[1][1]?.headers as Record<string, string>;
    expect(headers2["X-Queue-Token"]).toBe("tok");
    expect(headers2["If-None-Match"]).toBe('"v1"');

    await vi.advanceTimersByTimeAsync(6999);
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(1);
    const out = await run;
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(out.map((p) => p.state)).toEqual(["QUEUED", "EXPIRED"]);
  });

  it("ton trong Retry-After (giay) khi server xa tai 503", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(null, { status: 503, headers: { "Retry-After": "2" } }))
      .mockResolvedValueOnce(jsonResponse(position({ state: "ADMITTED" })));

    const run = (async () => {
      let n = 0;
      for await (const _ of watchQueue(EVENT, "tok")) n++;
      return n;
    })();

    await vi.advanceTimersByTimeAsync(1999);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(await run).toBe(1);
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("mat mang: lui theo cap so nhan roi thu lai, khong ket thuc", async () => {
    vi.spyOn(Math, "random").mockReturnValue(0); // bo jitter cua backoff
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockRejectedValueOnce(new TypeError("network"))
      .mockRejectedValueOnce(new TypeError("network"))
      .mockResolvedValueOnce(jsonResponse(position({ state: "ADMITTED" })));

    const run = (async () => {
      const out: QueuePosition[] = [];
      for await (const p of watchQueue(EVENT, "tok")) out.push(p);
      return out;
    })();

    await vi.advanceTimersByTimeAsync(0);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1000); // backoff 1s
    expect(fetchMock).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(2000); // backoff 2s
    const out = await run;
    expect(fetchMock).toHaveBeenCalledTimes(3);
    expect(out[0].state).toBe("ADMITTED");
  });

  it("dung ngay khi bi abort", async () => {
    const ctrl = new AbortController();
    vi.spyOn(globalThis, "fetch").mockImplementation(() => {
      ctrl.abort();
      return Promise.reject(new DOMException("aborted", "AbortError"));
    });

    const out: QueuePosition[] = [];
    for await (const p of watchQueue(EVENT, "tok", ctrl.signal)) out.push(p);
    expect(out).toHaveLength(0);
  });
});
