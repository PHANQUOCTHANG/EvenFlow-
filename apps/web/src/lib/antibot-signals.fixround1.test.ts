import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { readSignals, snapshotSignals, startSignalCollector } from "./antibot-signals";
import { clearSession, setSession } from "./session";

const EV = "evt-ab-fix1";
const FAKE = {
  toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval", "Date", "performance"],
} as Parameters<typeof vi.useFakeTimers>[0];

function keydown(repeat: boolean) {
  document.dispatchEvent(new KeyboardEvent("keydown", { key: "ArrowDown", repeat, bubbles: true }));
}

describe("antibot collector, fix round 1", () => {
  let stop: (() => void) | undefined;
  beforeEach(() => window.sessionStorage.clear());
  afterEach(() => {
    stop?.();
    stop = undefined;
    vi.useRealTimers();
    window.sessionStorage.clear();
    window.localStorage.clear();
  });

  it("X-8 / R-7: auto-repeat keydowns are counted but never enter iv (no machine rhythm for R1)", () => {
    vi.useFakeTimers(FAKE);
    stop = startSignalCollector(EV);
    vi.advanceTimersByTime(400);
    keydown(false); // the real press
    for (let i = 0; i < 40; i++) {
      vi.advanceTimersByTime(33);
      keydown(true); // OS auto-repeat while held
    }
    vi.advanceTimersByTime(700);
    const s = snapshotSignals(EV);
    expect(s.ke).toBe(41);
    // discrete events: the real press and the join click => exactly one interval, not 33 ms repeats
    expect(s.iv).toEqual([33 * 40 + 700]);
    expect(s.sq).toBeUndefined();
  });

  it("X-6: logout removes every anti-bot snapshot from sessionStorage", () => {
    stop = startSignalCollector(EV);
    snapshotSignals(EV);
    snapshotSignals("evt-ab-other");
    expect(readSignals(EV)).not.toBeNull();
    clearSession({ buyFlow: true });
    expect(readSignals(EV)).toBeNull();
    expect(readSignals("evt-ab-other")).toBeNull();
  });

  it("X-6: switching to another account removes the previous account's snapshot", () => {
    const sess = (id: string) => ({ accessToken: `t-${id}`, refreshToken: null, user: { id, role: "buyer" as const } });
    setSession(sess("11111111-1111-4111-8111-111111111111"));
    stop = startSignalCollector(EV);
    snapshotSignals(EV);
    expect(readSignals(EV)).not.toBeNull();
    setSession(sess("22222222-2222-4222-8222-222222222222"));
    expect(readSignals(EV)).toBeNull();
  });
});
