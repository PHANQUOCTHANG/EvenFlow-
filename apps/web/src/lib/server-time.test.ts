/**
 * Test cho `serverOffsetMs` + `serverNow` — AC-3 (spec-plan EVF-1803 §4).
 *
 * Module nay la cho duy nhat trong slice biet "gio server". Ba rang buoc:
 *
 *   1. Offset la mot phep tru THUAN, do caller truyen vao. Module KHONG duoc
 *      tu fetch gio server (AC-3 dong cuoi) — neu nó tu goi API thi slice
 *      nay thanh phu thuoc backend, trai voi muc dich cua task. Test chan
 *      bang cach stub `fetch`/`XMLHttpRequest` roi assert khong ai goi.
 *   2. Offset khong huu hien (NaN/Infinity, vi du server tra header rac) phai
 *      coi la 0 — tin dong ho client — chu KHONG duoc lam ca phep tinh thanh
 *      NaN. Mot NaN o day se chay xuong tan dong ho dem nguoc va hien "NaN".
 *   3. Dau cua offset: server NHANH hon client -> offset duong. Test ca hai
 *      chieu, vi dao dau la bug im lang (dong ho lech dung 2x do lech thuc).
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { serverNow, serverOffsetMs } from "./server-time";

const FIXED_NOW = Date.UTC(2026, 0, 15, 3, 0, 0);

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("serverOffsetMs (AC-3)", () => {
  it("la hieu serverNowMs - clientNowMs", () => {
    expect(serverOffsetMs(1_000, 400)).toBe(600);
  });

  it("server nhanh hon client -> offset duong", () => {
    expect(serverOffsetMs(FIXED_NOW + 5_000, FIXED_NOW)).toBe(5_000);
  });

  it("server cham hon client -> offset am (khong dao dau)", () => {
    expect(serverOffsetMs(FIXED_NOW - 5_000, FIXED_NOW)).toBe(-5_000);
  });

  it("hai dong ho trung nhau -> 0", () => {
    expect(serverOffsetMs(FIXED_NOW, FIXED_NOW)).toBe(0);
  });

  it("khong throw khi dau vao khong huu hien", () => {
    expect(() => serverOffsetMs(Number.NaN, FIXED_NOW)).not.toThrow();
    expect(() => serverOffsetMs(FIXED_NOW, Number.NaN)).not.toThrow();
    expect(() => serverOffsetMs(Number.POSITIVE_INFINITY, FIXED_NOW)).not.toThrow();
  });

  it("tra ve so (khong tra undefined/string)", () => {
    expect(typeof serverOffsetMs(1_000, 400)).toBe("number");
  });
});

describe("serverNow (AC-3)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(FIXED_NOW);
  });

  it("= Date.now() + offsetMs", () => {
    expect(serverNow(5_000)).toBe(FIXED_NOW + 5_000);
  });

  it("offset am tru dung", () => {
    expect(serverNow(-5_000)).toBe(FIXED_NOW - 5_000);
  });

  it("offset 0 = Date.now()", () => {
    expect(serverNow(0)).toBe(FIXED_NOW);
  });

  it("khong truyen offset -> coi la 0, tin dong ho client", () => {
    expect(serverNow()).toBe(FIXED_NOW);
  });

  it("offset undefined tuong minh -> coi la 0", () => {
    expect(serverNow(undefined)).toBe(FIXED_NOW);
  });

  it.each([Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY])(
    "offset khong huu hien (%s) -> coi la 0, KHONG tra NaN",
    (offset) => {
      const out = serverNow(offset);
      expect(Number.isFinite(out)).toBe(true);
      expect(out).toBe(FIXED_NOW);
    },
  );

  it("theo dong ho he thong: doi system time thi ket qua doi theo", () => {
    expect(serverNow(1_000)).toBe(FIXED_NOW + 1_000);
    vi.setSystemTime(FIXED_NOW + 60_000);
    expect(serverNow(1_000)).toBe(FIXED_NOW + 61_000);
  });
});

describe("server-time khong tu goi API lay gio server (AC-3)", () => {
  it("serverNow / serverOffsetMs khong goi fetch hay XMLHttpRequest", () => {
    const fetchSpy = vi.fn();
    const xhrOpen = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    vi.stubGlobal(
      "XMLHttpRequest",
      class {
        open = xhrOpen;
        send = vi.fn();
        setRequestHeader = vi.fn();
      },
    );

    serverOffsetMs(FIXED_NOW + 1_000, FIXED_NOW);
    serverNow(1_000);
    serverNow();

    expect(fetchSpy).not.toHaveBeenCalled();
    expect(xhrOpen).not.toHaveBeenCalled();
  });

  it("nap module cung khong gay request nao (khong co side effect luc import)", async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    vi.resetModules();

    const mod = await import("./server-time");

    expect(typeof mod.serverNow).toBe("function");
    expect(typeof mod.serverOffsetMs).toBe("function");
    expect(fetchSpy).not.toHaveBeenCalled();
  });

  it("khong tra ve Promise (API dong bo, khong an giau I/O)", () => {
    expect(serverNow(0)).not.toBeInstanceOf(Promise);
    expect(serverOffsetMs(1, 0)).not.toBeInstanceOf(Promise);
  });
});
