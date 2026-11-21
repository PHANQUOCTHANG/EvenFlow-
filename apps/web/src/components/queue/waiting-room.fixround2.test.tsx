import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { WaitingRoom } from "./waiting-room";

vi.mock("next/navigation", () => ({ useRouter: () => ({ push: vi.fn() }) }));
vi.mock("@/hooks/use-server-time-sync", () => ({
  useServerTimeSync: () => ({ offsetMs: 0, synced: true, isLoading: false, error: null, getServerNow: () => Date.now() }),
}));

function jsonResponse(data: unknown, status = 200) {
  return new Response(JSON.stringify(data), { status, headers: { "Content-Type": "application/json" } });
}

const SNAP = { v: 1, rt: 4330, pe: 3, se: 1, ke: 2, iv: [120, 340], wd: false, gl: true, fc: 12, tzo: 420 };

describe("WaitingRoom clears the anti-bot snapshot when the status poll turns SOLD_OUT (M61, AC-1)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    window.sessionStorage.clear();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    window.sessionStorage.clear();
  });

  it("a held token whose status becomes SOLD_OUT removes ef:ab:{eventId}", async () => {
    window.sessionStorage.setItem("ef:queue:token:evt-1", "0123456789abcdef0123456789abcdef");
    window.sessionStorage.setItem("ef:ab:evt-1", JSON.stringify(SNAP));
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(jsonResponse({ state: "SOLD_OUT", rank: -1, queue_depth: 0, admit_rate: 0, eta_seconds: -1, poll_after_ms: 30000 }));
    render(<WaitingRoom eventId="evt-1" />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/queue/join"))).toBe(false);
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/queue/status"))).toBe(true);
    expect(window.sessionStorage.getItem("ef:ab:evt-1")).toBeNull();
  });
});
