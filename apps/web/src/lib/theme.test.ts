/**
 * Unit test cho lib/theme.ts — AC-2.
 *
 * Hai hanh vi de vo nhat:
 *   - localStorage bi chan (private mode / Safari) thi getItem/setItem NEM loi.
 *     readStoredTheme phai tra null va storeTheme phai no-op, khong duoc crash trang.
 *   - jsdom khong bao dam co matchMedia -> systemTheme phai fallback "light".
 */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  THEME_STORAGE_KEY,
  readStoredTheme,
  storeTheme,
  systemTheme,
  type Theme,
} from "./theme";

const originalMatchMedia = window.matchMedia;

/** Stub matchMedia: chi query chua "dark" moi match khi prefersDark = true. */
function mockMatchMedia(prefersDark: boolean): void {
  window.matchMedia = vi.fn((query: string) => ({
    matches: prefersDark && query.includes("dark"),
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia;
}

beforeEach(() => {
  window.localStorage.clear();
});

afterEach(() => {
  vi.restoreAllMocks();
  window.matchMedia = originalMatchMedia;
  window.localStorage.clear();
});

describe("THEME_STORAGE_KEY", () => {
  it("la khoa da chot trong hop dong", () => {
    expect(THEME_STORAGE_KEY).toBe("eventflow-theme");
  });
});

describe("readStoredTheme", () => {
  it("tra null khi chua co gi trong localStorage", () => {
    expect(readStoredTheme()).toBeNull();
  });

  it("doc lai dung theme da luu", () => {
    window.localStorage.setItem(THEME_STORAGE_KEY, "dark");
    expect(readStoredTheme()).toBe("dark");

    window.localStorage.setItem(THEME_STORAGE_KEY, "light");
    expect(readStoredTheme()).toBe("light");
  });

  it("tra null khi gia tri luu khong hop le", () => {
    window.localStorage.setItem(THEME_STORAGE_KEY, "tim-than-thanh");
    expect(readStoredTheme()).toBeNull();
  });

  it("tra null thay vi nem loi khi localStorage bi chan", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("SecurityError: storage bi chan");
    });

    expect(() => readStoredTheme()).not.toThrow();
    expect(readStoredTheme()).toBeNull();
  });
});

describe("storeTheme", () => {
  it("ghi theme vao localStorage duoi dung khoa", () => {
    const t: Theme = "dark";
    storeTheme(t);
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");
  });

  it("ghi de lua chon truoc do", () => {
    storeTheme("dark");
    storeTheme("light");
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("light");
  });

  it("no-op thay vi nem loi khi localStorage bi chan", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("SecurityError: storage bi chan");
    });

    expect(() => storeTheme("dark")).not.toThrow();
  });
});

describe("systemTheme", () => {
  it('tra "dark" khi prefers-color-scheme: dark', () => {
    mockMatchMedia(true);
    expect(systemTheme()).toBe("dark");
  });

  it('tra "light" khi he thong khong uu tien dark', () => {
    mockMatchMedia(false);
    expect(systemTheme()).toBe("light");
  });

  it('tra "light" khi moi truong khong co matchMedia', () => {
    window.matchMedia = undefined as unknown as typeof window.matchMedia;
    expect(() => systemTheme()).not.toThrow();
    expect(systemTheme()).toBe("light");
  });
});
