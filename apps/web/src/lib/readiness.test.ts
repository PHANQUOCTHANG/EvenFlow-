/**
 * Test cho `readiness` — AC-2 (spec muc 4), contract C1.
 *
 * Module nay la cho DUY NHAT trong slice giu "app da san sang nhan traffic
 * chua" va "da chay bao lau". Route `/api/readyz` chi dich trang thai do thanh
 * ma HTTP.
 *
 * BE MAT API DUOC GIA DINH (C1 chot hinh dang response nhung KHONG chot cach
 * readiness quyet dinh minh da san sang — xem bao cao buoc Test Design):
 *
 *   - `uptimeMs(): number`  — mili-giay ke tu khi module khoi tao, so nguyen
 *     khong am (nguyen van C1: "uptimeMs la so nguyen khong am, mili-giay ke tu
 *     khi module khoi tao").
 *   - `isReady(): boolean`  — trang thai hien tai, doc dong bo tai thoi diem goi.
 *   - `setReady(ready: boolean): void` — duong duy nhat doi trang thai.
 *
 * Vi sao phai co `setReady`: C1 bat buoc co nhanh `503`. Neu khong co cach nao
 * ha trang thai xuong thi nhanh `503` la code chet, khong bao gio chay duoc va
 * khong kiem duoc — contract se khong duoc bao ve o phan quan trong nhat.
 *
 * Vi sao mac dinh phai la READY: pod web khong co buoc khoi tao bat dong bo nao
 * truoc khi phuc vu duoc; neu mac dinh la not-ready thi pod KHONG BAO GIO vao
 * endpoints cua Service va deployment treo mai o 0/N ready.
 *
 * Trang thai dung o buoc Test Design: `./readiness` CHUA TON TAI, test se bao
 * "module not found" cho den khi WP-A hien thuc.
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { isReady, setReady, uptimeMs } from "./readiness";

const FIXED_NOW = Date.UTC(2026, 0, 15, 3, 0, 0);

afterEach(() => {
  setReady(true);
  vi.useRealTimers();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("uptimeMs — so nguyen khong am, dem tu luc module khoi tao (AC-2, C1)", () => {
  it("tra ve number, khong phai Promise (API dong bo, khong an giau I/O)", () => {
    const out = uptimeMs();

    expect(typeof out).toBe("number");
    expect(out).not.toBeInstanceOf(Promise);
  });

  it("la so nguyen", () => {
    expect(Number.isInteger(uptimeMs())).toBe(true);
  });

  it("khong am", () => {
    expect(uptimeMs()).toBeGreaterThanOrEqual(0);
  });

  it("huu hien, khong bao gio NaN", () => {
    expect(Number.isFinite(uptimeMs())).toBe(true);
    expect(Number.isNaN(uptimeMs())).toBe(false);
  });

  it("bang 0 ngay sau khi module khoi tao", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(FIXED_NOW);
    vi.resetModules();

    const mod = await import("./readiness");

    expect(mod.uptimeMs()).toBe(0);
  });

  it("tang dung bang luong thoi gian da troi", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(FIXED_NOW);
    vi.resetModules();

    const mod = await import("./readiness");
    const before = mod.uptimeMs();
    vi.advanceTimersByTime(5_000);
    const after = mod.uptimeMs();

    expect(after - before).toBe(5_000);
    expect(after).toBeGreaterThan(before);
  });

  it("khong di lui khi dong ho tien tiep (hai nhip lien tiep)", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(FIXED_NOW);
    vi.resetModules();

    const mod = await import("./readiness");
    const first = mod.uptimeMs();
    vi.advanceTimersByTime(1_000);
    const second = mod.uptimeMs();
    vi.advanceTimersByTime(60_000);
    const third = mod.uptimeMs();

    expect(second).toBeGreaterThanOrEqual(first);
    expect(third).toBeGreaterThanOrEqual(second);
    expect(third - first).toBe(61_000);
  });

  it("khong di lui giua hai lan goi lien tiep (dong ho that)", () => {
    const first = uptimeMs();
    const second = uptimeMs();

    expect(second).toBeGreaterThanOrEqual(first);
  });

  it("van dem tiep khi app da bi ha xuong not-ready (dang drain van la dang chay)", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(FIXED_NOW);
    vi.resetModules();

    const mod = await import("./readiness");
    mod.setReady(false);
    const before = mod.uptimeMs();
    vi.advanceTimersByTime(2_000);

    expect(mod.uptimeMs() - before).toBe(2_000);
  });
});

describe("isReady / setReady — trang thai san sang (AC-2, C1)", () => {
  it("mac dinh la san sang ngay sau khi module khoi tao", async () => {
    vi.resetModules();

    const mod = await import("./readiness");

    expect(mod.isReady()).toBe(true);
  });

  it("tra ve boolean that, khong phai gia tri truthy/falsy mo ho", () => {
    expect(typeof isReady()).toBe("boolean");

    setReady(false);
    expect(typeof isReady()).toBe("boolean");
  });

  it("khong tra ve Promise (readyz phai doc duoc dong bo)", () => {
    expect(isReady()).not.toBeInstanceOf(Promise);
  });

  it("setReady(false) lam isReady() thanh false", () => {
    setReady(false);

    expect(isReady()).toBe(false);
  });

  it("setReady(true) lam isReady() thanh true", () => {
    setReady(false);
    setReady(true);

    expect(isReady()).toBe(true);
  });

  it("doi trang thai qua lai nhieu lan deu phan anh dung", () => {
    const seen: boolean[] = [];

    for (const next of [false, true, false, false, true]) {
      setReady(next);
      seen.push(isReady());
    }

    expect(seen).toEqual([false, true, false, false, true]);
  });

  it("goi setReady cung gia tri hai lan khong doi ket qua (idempotent)", () => {
    setReady(false);
    setReady(false);

    expect(isReady()).toBe(false);
  });

  it("doc lai nhieu lan khong tu dong doi trang thai", () => {
    setReady(false);

    expect(isReady()).toBe(false);
    expect(isReady()).toBe(false);
    expect(isReady()).toBe(false);
  });
});

describe("readiness KHONG cham dependency ngoai (AC-1 3.1, AC-2)", () => {
  it("isReady / setReady / uptimeMs khong goi fetch hay XMLHttpRequest", () => {
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

    uptimeMs();
    isReady();
    setReady(false);
    isReady();

    expect(fetchSpy).not.toHaveBeenCalled();
    expect(xhrOpen).not.toHaveBeenCalled();
  });

  it("nap module khong gay request nao (khong co side effect luc import)", async () => {
    const fetchSpy = vi.fn();
    vi.stubGlobal("fetch", fetchSpy);
    vi.resetModules();

    const mod = await import("./readiness");

    expect(typeof mod.isReady).toBe("function");
    expect(typeof mod.setReady).toBe("function");
    expect(typeof mod.uptimeMs).toBe("function");
    expect(fetchSpy).not.toHaveBeenCalled();
  });
});
