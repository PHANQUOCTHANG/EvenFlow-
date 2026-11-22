import { afterEach, describe, expect, it, vi } from "vitest";

import { resetSignalCollector, snapshotSignals, startSignalCollector } from "./antibot-signals";
import { clearSession, setSession } from "./session";

const EV = "evt-ab-fix2";
const FAKE = { toFake: ["Date", "performance"] } as Parameters<typeof vi.useFakeTimers>[0];

function keys(n: number) {
  for (let i = 0; i < n; i++) {
    vi.advanceTimersByTime(150 + (i % 5) * 70);
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "a", bubbles: true }));
  }
}

describe("antibot collector reset (R2-5)", () => {
  let stop: (() => void) | undefined;
  afterEach(() => {
    stop?.();
    stop = undefined;
    vi.useRealTimers();
    window.sessionStorage.clear();
    window.localStorage.clear();
  });

  it("resetSignalCollector drops the in-memory counters and intervals", () => {
    vi.useFakeTimers(FAKE);
    stop = startSignalCollector(EV);
    keys(20);
    resetSignalCollector();
    const s = snapshotSignals(EV);
    expect(s.ke).toBe(0);
    expect(s.pe).toBe(1); // only the join click
    expect(s.iv).toEqual([]);
    expect(s.sq).toBeUndefined();
  });

  it("logout resets the in-memory collector state, not only sessionStorage", () => {
    vi.useFakeTimers(FAKE);
    stop = startSignalCollector(EV);
    keys(20);
    clearSession({ buyFlow: true });
    expect(snapshotSignals(EV).ke).toBe(0);
  });

  it("switching account resets the in-memory collector state", () => {
    vi.useFakeTimers(FAKE);
    const sess = (id: string) => ({ accessToken: `t-${id}`, refreshToken: null, user: { id, role: "buyer" as const } });
    setSession(sess("11111111-1111-4111-8111-111111111111"));
    stop = startSignalCollector(EV);
    keys(20);
    setSession(sess("22222222-2222-4222-8222-222222222222"));
    const s = snapshotSignals(EV);
    expect(s.ke).toBe(0);
    expect(s.iv).toEqual([]);
  });
});
