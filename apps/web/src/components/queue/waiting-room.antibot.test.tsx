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

const SNAP = { v: 1, rt: 4330, pe: 3, se: 1, ke: 2, iv: [120, 340, 560], fp: "device-abc-12345", wd: false, gl: true, fc: 12, tz: "Asia/Ho_Chi_Minh", tzo: 420 };

function joinBodies(fetchMock: { mock: { calls: unknown[][] } }): Array<Record<string, unknown>> {
  return fetchMock.mock.calls
    .filter(([u]) => String(u).includes("/queue/join"))
    .map(([, init]) => JSON.parse(String((init as RequestInit).body)) as Record<string, unknown>);
}

describe("WaitingRoom sends the anti-bot snapshot (EV-173, AC-1, Amendment 1 revision)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    window.sessionStorage.clear();
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
    window.sessionStorage.clear();
  });

  it("join sends the stored snapshot as signals", async () => {
    window.sessionStorage.setItem("ef:ab:evt-1", JSON.stringify(SNAP));
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(jsonResponse({ state: "QUEUED", queue_token: "tok-1", rank: 5, is_new: true, poll_after_ms: 5000 }))
      .mockResolvedValue(jsonResponse({ state: "QUEUED", rank: 5, poll_after_ms: 5000 }));
    render(<WaitingRoom eventId="evt-1" />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    const bodies = joinBodies(fetchMock);
    expect(bodies).toHaveLength(1);
    expect(bodies[0].signals).toEqual(SNAP);
  });

  it('no snapshot: still auto-joins, with signals {"v":1} (not {})', async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(jsonResponse({ state: "QUEUED", queue_token: "tok-1", rank: 5, is_new: true, poll_after_ms: 5000 }))
      .mockResolvedValue(jsonResponse({ state: "QUEUED", rank: 5, poll_after_ms: 5000 }));
    render(<WaitingRoom eventId="evt-1" />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    const bodies = joinBodies(fetchMock);
    expect(bodies).toHaveLength(1);
    expect(bodies[0].signals).toEqual({ v: 1 });
  });

  it("401 re-join sends the same snapshot again and keeps it stored", async () => {
    window.sessionStorage.setItem("ef:queue:token:evt-1", "0123456789abcdef0123456789abcdef");
    window.sessionStorage.setItem("ef:ab:evt-1", JSON.stringify(SNAP));
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(jsonResponse({ type: "/problems/queue-token-invalid" }, 401))
      .mockResolvedValueOnce(jsonResponse({ state: "QUEUED", queue_token: "q1.moi", rank: 7, is_new: false, poll_after_ms: 5000 }))
      .mockResolvedValue(jsonResponse({ state: "QUEUED", rank: 7, poll_after_ms: 5000 }));
    render(<WaitingRoom eventId="evt-1" />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    const bodies = joinBodies(fetchMock);
    expect(bodies).toHaveLength(1);
    expect(bodies[0].signals).toEqual(SNAP);
    expect(JSON.parse(window.sessionStorage.getItem("ef:ab:evt-1") as string)).toEqual(SNAP);
  });

  it("sold-out screen removes the snapshot (the buyer leaves the event)", async () => {
    window.sessionStorage.setItem("ef:ab:evt-1", JSON.stringify(SNAP));
    vi.spyOn(globalThis, "fetch").mockResolvedValueOnce(
      jsonResponse({ state: "SOLD_OUT", queue_token: "", rank: -1, is_new: false, poll_after_ms: 30000 }),
    );
    render(<WaitingRoom eventId="evt-1" />);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(window.sessionStorage.getItem("ef:ab:evt-1")).toBeNull();
  });
});
