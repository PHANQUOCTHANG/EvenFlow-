import { readFileSync, writeFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { clearSignals, readSignals, snapshotSignals, startSignalCollector } from "./antibot-signals";
import { getDeviceFingerprint } from "./device";

const EV = "evt-ab-1";
const KEY = `ef:ab:${EV}`;

// Shared golden fixture, decoded by libs/go/antibot/golden_test.go (contracts 1, R2).
const GOLDEN = fileURLToPath(
  new URL("../../../../libs/go/antibot/testdata/signals/scripted_keyboard.json", import.meta.url),
);

function stored(): Record<string, unknown> {
  const raw = window.sessionStorage.getItem(KEY);
  expect(raw).not.toBeNull();
  return JSON.parse(raw as string) as Record<string, unknown>;
}

// Dispatched on document: the event bubbles to window, so a listener on either sees it once.
function pointer(type: string, x = 10, y = 20) {
  document.dispatchEvent(new MouseEvent(type, { bubbles: true, clientX: x, clientY: y }));
}

function key(k: string) {
  document.dispatchEvent(new KeyboardEvent("keydown", { key: k, code: `Key${k.toUpperCase()}`, bubbles: true }));
}

function scroll() {
  window.dispatchEvent(new Event("scroll"));
}

const FAKE = {
  toFake: ["setTimeout", "clearTimeout", "setInterval", "clearInterval", "Date", "performance"],
} as Parameters<typeof vi.useFakeTimers>[0];

describe("antibot signal collector (EV-173, AC-1, AC-2, contracts 1 R2)", () => {
  let stop: (() => void) | undefined;

  beforeEach(() => {
    window.sessionStorage.clear();
  });

  afterEach(() => {
    stop?.();
    stop = undefined;
    vi.useRealTimers();
    vi.restoreAllMocks();
    window.sessionStorage.clear();
  });

  it("AC-1: readSignals returns null when no snapshot was stored", () => {
    expect(readSignals(EV)).toBeNull();
  });

  it("AC-1: readSignals returns null when the stored snapshot is corrupt", () => {
    window.sessionStorage.setItem(KEY, "{not json");
    expect(readSignals(EV)).toBeNull();
    window.sessionStorage.setItem(KEY, JSON.stringify([1, 2, 3]));
    expect(readSignals(EV)).toBeNull();
  });

  it("AC-1: snapshot is stored under ef:ab:{eventId} as v1 and read back unchanged", () => {
    stop = startSignalCollector(EV);
    pointer("pointerdown");
    snapshotSignals(EV);
    const snap = stored();
    expect(snap.v).toBe(1);
    expect(readSignals(EV)).toEqual(snap);
  });

  it("AC-1: snapshot carries the documented v1 fields with valid types", () => {
    stop = startSignalCollector(EV);
    snapshotSignals(EV);
    const s = stored();
    expect(Number.isInteger(s.rt)).toBe(true);
    expect(s.rt as number).toBeGreaterThanOrEqual(0);
    expect(s.rt as number).toBeLessThanOrEqual(600000);
    for (const f of ["pe", "se", "ke"]) {
      expect(Number.isInteger(s[f])).toBe(true);
    }
    expect(s.fp).toBe(getDeviceFingerprint());
    expect(typeof s.wd).toBe("boolean");
    expect(typeof s.gl).toBe("boolean");
    expect(Number.isInteger(s.fc)).toBe(true);
    expect(s.fc as number).toBeGreaterThanOrEqual(0);
    expect(s.fc as number).toBeLessThanOrEqual(20);
    expect(typeof s.tz).toBe("string");
    expect(s.tzo).toBe(-new Date().getTimezoneOffset());
  });

  it("AC-1: the join click itself is counted, so a click-only buyer has pe >= 1", () => {
    stop = startSignalCollector(EV);
    snapshotSignals(EV);
    expect(stored().pe as number).toBeGreaterThanOrEqual(1);
  });

  it("AC-2: all collector listeners are registered passive", () => {
    const winSpy = vi.spyOn(window, "addEventListener");
    const docSpy = vi.spyOn(document, "addEventListener");
    stop = startSignalCollector(EV);
    const calls = [...winSpy.mock.calls, ...docSpy.mock.calls];
    expect(calls.length).toBeGreaterThan(0);
    for (const [, , opts] of calls) {
      expect(typeof opts).toBe("object");
      expect((opts as AddEventListenerOptions).passive).toBe(true);
    }
  });

  it("AC-2: the cleanup removes the listeners (later events are not counted)", () => {
    stop = startSignalCollector(EV);
    stop?.();
    stop = undefined;
    for (let i = 0; i < 50; i++) key("a");
    snapshotSignals(EV);
    expect(stored().ke as number).toBe(0);
  });

  it("AC-2: keydown is counted without keeping key values; no coordinates are kept", () => {
    stop = startSignalCollector(EV);
    for (const ch of "hunter2") key(ch);
    pointer("pointerdown", 1234.5, 987.25);
    snapshotSignals(EV);
    const raw = window.sessionStorage.getItem(KEY) as string;
    expect(stored().ke as number).toBe(7);
    expect(raw).not.toContain("hunter2");
    expect(raw).not.toMatch(/Key[A-Z]/);
    expect(raw).not.toContain("1234.5");
    expect(raw).not.toContain("987.25");
    expect(raw).not.toMatch(/"(key|code|x|y|clientX|clientY|text)"\s*:/);
  });

  it("AC-2: counts are capped at 10000 and intervals at 32 items, each 0..60000", () => {
    vi.useFakeTimers(FAKE);
    stop = startSignalCollector(EV);
    for (let i = 0; i < 10500; i++) {
      key("a");
      vi.advanceTimersByTime(1);
    }
    vi.advanceTimersByTime(120000); // one very long gap
    key("a");
    snapshotSignals(EV);
    const s = stored();
    expect(s.ke as number).toBe(10000);
    const iv = s.iv as number[];
    expect(Array.isArray(iv)).toBe(true);
    expect(iv.length).toBe(32);
    for (const d of iv) {
      expect(Number.isInteger(d)).toBe(true);
      expect(d).toBeGreaterThanOrEqual(0);
      expect(d).toBeLessThanOrEqual(60000);
    }
  });

  it("AC-2 (R2, D14): pointermove is sampled at most once per 100 ms and never enters iv", () => {
    vi.useFakeTimers(FAKE);
    stop = startSignalCollector(EV);
    for (let i = 0; i < 100; i++) pointer("pointermove"); // same millisecond
    snapshotSignals(EV);
    const s = stored();
    expect(s.pe as number).toBeLessThanOrEqual(2); // one sampled move + the join click
    expect((s.iv as number[] | undefined) ?? []).toHaveLength(0);
  });

  it("AC-2 (R2, D14): only the first scroll of a burst is a discrete event; repeats only count", () => {
    vi.useFakeTimers(FAKE);
    stop = startSignalCollector(EV);
    for (let i = 0; i < 30; i++) {
      scroll();
      vi.advanceTimersByTime(110); // a continuous burst: no 500 ms pause
    }
    snapshotSignals(EV);
    const s = stored();
    expect(s.se as number).toBeGreaterThan(1);
    // discrete events: first scroll + the join click => exactly one interval
    expect((s.iv as number[]).length).toBe(1);
  });

  it("AC-2 (R2): sq needs >= 12 discrete events and >= 4 distinct buckets; sn 12..32, sb 4..41", () => {
    vi.useFakeTimers(FAKE);
    stop = startSignalCollector(EV);
    // 11 keys + click = 12 events, but every interval is in one bucket => no sq
    for (let i = 0; i < 11; i++) {
      key("a");
      vi.advanceTimersByTime(60);
    }
    snapshotSignals(EV);
    expect(stored().sq).toBeUndefined();

    window.sessionStorage.clear();
    stop?.();
    stop = startSignalCollector(EV);
    for (let i = 0; i < 40; i++) {
      key("a");
      vi.advanceTimersByTime(130 + (i % 7) * 97);
    }
    snapshotSignals(EV);
    const s = stored();
    expect(s.sq).toMatch(/^[0-9a-f]{16}$/);
    expect(s.sn as number).toBeGreaterThanOrEqual(12);
    expect(s.sn as number).toBeLessThanOrEqual(32);
    expect(s.sb as number).toBeGreaterThanOrEqual(4);
    expect(s.sb as number).toBeLessThanOrEqual(41);
  });

  it("AC-2: fewer than 12 discrete events => no sq", () => {
    vi.useFakeTimers(FAKE);
    stop = startSignalCollector(EV);
    for (let i = 0; i < 5; i++) {
      key("a");
      vi.advanceTimersByTime(100 + i * 300);
    }
    snapshotSignals(EV);
    expect(stored().sq).toBeUndefined();
  });

  it("AC-2: serialised snapshot stays <= 2 KiB after heavy activity", () => {
    vi.useFakeTimers(FAKE);
    stop = startSignalCollector(EV);
    for (let i = 0; i < 3000; i++) {
      key("a");
      pointer("pointerdown", i, i);
      scroll();
      vi.advanceTimersByTime(101 + (i % 13) * 997);
    }
    snapshotSignals(EV);
    const raw = window.sessionStorage.getItem(KEY) as string;
    expect(new TextEncoder().encode(raw).length).toBeLessThanOrEqual(2048);
  });

  it("AC-2: font / WebGL probes run once per page", () => {
    const spy = vi.spyOn(HTMLCanvasElement.prototype, "getContext");
    stop = startSignalCollector(EV);
    snapshotSignals(EV);
    const first = spy.mock.calls.length;
    snapshotSignals(EV);
    snapshotSignals("evt-ab-2");
    expect(spy.mock.calls.length).toBe(first);
  });

  it("clearSignals removes the stored snapshot", () => {
    stop = startSignalCollector(EV);
    snapshotSignals(EV);
    clearSignals(EV);
    expect(window.sessionStorage.getItem(KEY)).toBeNull();
    expect(readSignals(EV)).toBeNull();
  });

  it("AC-3 (R2): a scripted keyboard session matches the shared Go golden fixture", () => {
    window.localStorage.setItem("ef:device-id", "golden-device-0001");
    vi.useFakeTimers(FAKE);
    stop = startSignalCollector(EV);
    vi.advanceTimersByTime(500);
    const gaps = [180, 240, 310, 95, 520, 130, 275, 410, 160, 220, 350, 600, 140];
    key("a");
    for (const g of gaps) {
      vi.advanceTimersByTime(g);
      key("a");
    }
    vi.advanceTimersByTime(200);
    snapshotSignals(EV); // records the join click (a pointer event) first
    const got = stored();
    const golden = JSON.parse(readFileSync(GOLDEN, "utf8")) as Record<string, unknown>;
    if (process.env.UPDATE_ANTIBOT_GOLDEN) {
      // environment-dependent probes keep the golden's human-like values
      const out = { ...got, gl: golden.gl, fc: golden.fc, tz: golden.tz, tzo: golden.tzo, wd: false };
      writeFileSync(GOLDEN, JSON.stringify(out) + "\n");
    }
    for (const f of ["v", "rt", "pe", "se", "ke", "iv", "fp"]) {
      expect(got[f], f).toEqual(golden[f]);
    }
    expect(got.sq).toMatch(/^[0-9a-f]{16}$/);
    expect(got.sn).toBe(15);
    expect(got.sb as number).toBeGreaterThanOrEqual(4);
    window.localStorage.removeItem("ef:device-id");
  });
});
