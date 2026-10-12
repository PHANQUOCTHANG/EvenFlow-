import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useServerTimeSync } from "./use-server-time-sync";

describe("useServerTimeSync (EVF-111, AC-1)", () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    global.fetch = originalFetch;
  });

  it("dong bo thanh cong va tinh dung offsetMs", async () => {
    const fixedClientNow = 1_000_000;
    const serverNowMs = 1_060_000; // Server nhanh hon 60s

    vi.spyOn(Date, "now").mockReturnValue(fixedClientNow);

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        server_time: serverNowMs,
        rfc3339: new Date(serverNowMs).toISOString(),
      }),
    } as Response);

    const { result } = renderHook(() => useServerTimeSync("/api/time"));

    expect(result.current.isLoading).toBe(true);

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.synced).toBe(true);
    expect(result.current.error).toBeNull();
    expect(result.current.offsetMs).toBe(60_000);
    expect(result.current.getServerNow()).toBe(serverNowMs);
  });

  it("fallback an toan ve gio client khi fetch that bai", async () => {
    const clientNowMs = 1_000_000;
    vi.spyOn(Date, "now").mockReturnValue(clientNowMs);

    global.fetch = vi.fn().mockRejectedValue(new Error("Network failed"));

    const { result } = renderHook(() => useServerTimeSync("/api/time"));

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.synced).toBe(false);
    expect(result.current.error).not.toBeNull();
    expect(result.current.offsetMs).toBe(0);
    expect(result.current.getServerNow()).toBe(clientNowMs);
  });

  it("fallback an toan khi HTTP response khong ok", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
    } as Response);

    const { result } = renderHook(() => useServerTimeSync("/api/time"));

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.synced).toBe(false);
    expect(result.current.error?.message).toContain("HTTP 500");
    expect(result.current.offsetMs).toBe(0);
  });
});
