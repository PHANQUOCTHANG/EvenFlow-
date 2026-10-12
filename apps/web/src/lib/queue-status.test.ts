import { afterEach, describe, expect, it, vi } from "vitest";

import { QueueError } from "./queue-client";
import {
  DEFAULT_POLL_MS,
  DEFAULT_RETRY_AFTER_MS,
  MIN_POLL_MS,
  fetchQueueStatus,
  isTerminalState,
  normalizePosition,
  parseRetryAfterMs,
  sanitizePollAfterMs,
} from "./queue-status";

function json(body: unknown, init: { status?: number; headers?: Record<string, string> } = {}) {
  return new Response(JSON.stringify(body), {
    status: init.status ?? 200,
    headers: { "Content-Type": "application/json", ...init.headers },
  });
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("sanitizePollAfterMs — AC-1 khong bao gio poll nhanh hon server", () => {
  it("giu nguyen gia tri hop le", () => {
    expect(sanitizePollAfterMs(3000)).toBe(3000);
    expect(sanitizePollAfterMs(30000)).toBe(30000);
    expect(sanitizePollAfterMs("7000")).toBe(7000);
  });

  it("thieu / 0 / am / NaN / chuoi rong -> nhip du phong 10s", () => {
    for (const bad of [undefined, null, 0, -1, Number.NaN, Number.POSITIVE_INFINITY, "", "abc", {}]) {
      expect(sanitizePollAfterMs(bad)).toBe(DEFAULT_POLL_MS);
    }
  });

  it("qua nho (bug server) -> kep len 1s", () => {
    expect(sanitizePollAfterMs(1)).toBe(MIN_POLL_MS);
    expect(sanitizePollAfterMs(999)).toBe(MIN_POLL_MS);
  });

  it("fallback tuy chinh", () => {
    expect(sanitizePollAfterMs(undefined, 4000)).toBe(4000);
  });
});

describe("parseRetryAfterMs", () => {
  it("giay -> ms", () => {
    expect(parseRetryAfterMs("2")).toBe(2000);
  });
  it("thieu / khong hop le -> 5s", () => {
    for (const bad of [null, "", "0", "-4", "Wed, 21 Oct 2026 07:28:00 GMT"]) {
      expect(parseRetryAfterMs(bad)).toBe(DEFAULT_RETRY_AFTER_MS);
    }
  });
  it("le giay nho van khong duoi 1s", () => {
    expect(parseRetryAfterMs("0.2")).toBe(MIN_POLL_MS);
  });
});

describe("normalizePosition — AC-5 du lieu trung thuc", () => {
  it("rank/eta -1 cua server -> null (khong hien '-1' hay '~0 giay')", () => {
    const s = normalizePosition({ state: "QUEUED", rank: -1, eta_seconds: -1, poll_after_ms: 3000 });
    expect(s.rank).toBeNull();
    expect(s.etaSeconds).toBeNull();
  });

  it("BR-Q1: LOBBY khong co rank ke ca khi payload lo co so", () => {
    expect(normalizePosition({ state: "LOBBY", rank: 12, poll_after_ms: 15000 }).rank).toBeNull();
  });

  it("payload hop le duoc giu nguyen", () => {
    expect(
      normalizePosition({
        state: "ADMITTED",
        rank: 0,
        eta_seconds: 45,
        poll_after_ms: 3000,
        expires_at: 1_800_000_000_000,
      }),
    ).toEqual({
      state: "ADMITTED",
      rank: 0,
      etaSeconds: 45,
      pollAfterMs: 3000,
      expiresAt: 1_800_000_000_000,
    });
  });

  it("state la / payload hong -> UNKNOWN, khong nem", () => {
    expect(normalizePosition({ state: "WHATEVER" }).state).toBe("UNKNOWN");
    expect(normalizePosition(null).state).toBe("UNKNOWN");
    expect(normalizePosition("x").pollAfterMs).toBe(DEFAULT_POLL_MS);
  });

  it("rank khong nguyen -> null", () => {
    expect(normalizePosition({ state: "QUEUED", rank: 1.5 }).rank).toBeNull();
    expect(normalizePosition({ state: "QUEUED", rank: "12" }).rank).toBeNull();
  });
});

describe("isTerminalState", () => {
  it("ADMITTED / EXPIRED / SOLD_OUT / DROPPED la ket thuc", () => {
    for (const s of ["ADMITTED", "EXPIRED", "SOLD_OUT", "DROPPED"] as const) {
      expect(isTerminalState(s)).toBe(true);
    }
  });
  it("LOBBY / QUEUED / UNKNOWN thi tiep tuc", () => {
    for (const s of ["LOBBY", "QUEUED", "UNKNOWN", "RECONNECTING"] as const) {
      expect(isTerminalState(s)).toBe(false);
    }
  });
});

describe("fetchQueueStatus", () => {
  it("200 -> updated, gui X-Queue-Token va If-None-Match, doc ETag moi", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        json({ state: "QUEUED", rank: 120, eta_seconds: 30, poll_after_ms: 3000 }, { headers: { ETag: 'W/"Q-1"' } }),
      );

    const r = await fetchQueueStatus("evt 1", "tok", { etag: 'W/"Q-0"', baseUrl: "https://api" });

    expect(r).toMatchObject({ kind: "updated", etag: 'W/"Q-1"', pollAfterMs: 3000 });
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toBe("https://api/v1/events/evt%201/queue/status");
    const headers = init?.headers as Record<string, string>;
    expect(headers["X-Queue-Token"]).toBe("tok");
    expect(headers["If-None-Match"]).toBe('W/"Q-0"');
  });

  it("khong co ETag truoc do -> khong gui If-None-Match", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(json({ state: "LOBBY" }));
    await fetchQueueStatus("e", "tok");
    const headers = fetchMock.mock.calls[0][1]?.headers as Record<string, string>;
    expect(headers["If-None-Match"]).toBeUndefined();
  });

  it("304 -> not-modified, nhip lay tu X-Poll-After-Ms, giu ETag cu neu server khong gui lai", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(null, { status: 304, headers: { "X-Poll-After-Ms": "7000" } }),
    );
    const r = await fetchQueueStatus("e", "tok", { etag: 'W/"v1"' });
    expect(r).toEqual({ kind: "not-modified", etag: 'W/"v1"', pollAfterMs: 7000 });
  });

  it("304 thieu X-Poll-After-Ms -> 10s, khong nhanh hon", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(null, { status: 304 }));
    const r = await fetchQueueStatus("e", "tok");
    expect(r).toMatchObject({ kind: "not-modified", pollAfterMs: DEFAULT_POLL_MS });
  });

  it("503 -> throttled theo Retry-After", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(null, { status: 503, headers: { "Retry-After": "4" } }),
    );
    expect(await fetchQueueStatus("e", "tok")).toEqual({ kind: "throttled", retryAfterMs: 4000 });
  });

  it("4xx -> nem QueueError kem status va detail", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(json({ detail: "thieu X-Queue-Token" }, { status: 400 }));
    await expect(fetchQueueStatus("e", "tok")).rejects.toMatchObject({
      name: "QueueError",
      status: 400,
      message: "thieu X-Queue-Token",
    });
  });

  it("4xx body khong phai JSON -> van nem QueueError voi statusText", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("oops", { status: 404, statusText: "Not Found" }),
    );
    const err = await fetchQueueStatus("e", "tok").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(QueueError);
    expect((err as QueueError).status).toBe(404);
  });

  it("loi mang -> nem nguyen loi goc (hook se backoff)", async () => {
    vi.spyOn(globalThis, "fetch").mockRejectedValue(new TypeError("Failed to fetch"));
    await expect(fetchQueueStatus("e", "tok")).rejects.toBeInstanceOf(TypeError);
  });
});
