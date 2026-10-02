/**
 * Test cho useTheme — AC-2.
 *
 * Bao gom ca truong hop private mode: localStorage nem loi o CA getItem va setItem.
 * Khi do hook phai van render duoc va roi ve prefers-color-scheme, khong crash.
 *
 * jsdom khong bao dam co matchMedia nen moi test o day tu stub lay.
 */
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { THEME_STORAGE_KEY } from "../lib/theme";
import { useTheme } from "./use-theme";

const originalMatchMedia = window.matchMedia;

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

const mount = () => renderHook(() => useTheme());

beforeEach(() => {
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
  mockMatchMedia(false);
});

afterEach(() => {
  vi.restoreAllMocks();
  window.matchMedia = originalMatchMedia;
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("useTheme — doc trang thai khi mount", () => {
  it('lay theme tu data-theme="dark" da co tren <html>', () => {
    document.documentElement.setAttribute("data-theme", "dark");
    const { result } = mount();
    expect(result.current.theme).toBe("dark");
  });

  it('lay theme tu data-theme="light" da co tren <html>', () => {
    document.documentElement.setAttribute("data-theme", "light");
    const { result } = mount();
    expect(result.current.theme).toBe("light");
  });

  it("khong co lua chon thu cong nao thi theo prefers-color-scheme: dark", () => {
    mockMatchMedia(true);
    const { result } = mount();
    expect(result.current.theme).toBe("dark");
  });

  it("khong co lua chon thu cong va he thong sang thi la light", () => {
    mockMatchMedia(false);
    const { result } = mount();
    expect(result.current.theme).toBe("light");
  });

  it("uu tien lua chon da luu trong localStorage hon prefers-color-scheme", () => {
    window.localStorage.setItem(THEME_STORAGE_KEY, "light");
    mockMatchMedia(true);
    const { result } = mount();
    expect(result.current.theme).toBe("light");
  });
});

describe("useTheme — setTheme", () => {
  it("ghi ca data-theme tren <html> va localStorage", () => {
    const { result } = mount();

    act(() => {
      result.current.setTheme("dark");
    });

    expect(result.current.theme).toBe("dark");
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");
  });

  it("set ve light cung ghi day du", () => {
    document.documentElement.setAttribute("data-theme", "dark");
    const { result } = mount();

    act(() => {
      result.current.setTheme("light");
    });

    expect(result.current.theme).toBe("light");
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("light");
  });
});

describe("useTheme — toggle", () => {
  it("doi light -> dark", () => {
    document.documentElement.setAttribute("data-theme", "light");
    const { result } = mount();

    act(() => {
      result.current.toggle();
    });

    expect(result.current.theme).toBe("dark");
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
  });

  it("doi dark -> light", () => {
    document.documentElement.setAttribute("data-theme", "dark");
    const { result } = mount();

    act(() => {
      result.current.toggle();
    });

    expect(result.current.theme).toBe("light");
    expect(document.documentElement).toHaveAttribute("data-theme", "light");
  });

  it("toggle hai lan tro ve trang thai ban dau", () => {
    document.documentElement.setAttribute("data-theme", "light");
    const { result } = mount();

    act(() => {
      result.current.toggle();
    });
    act(() => {
      result.current.toggle();
    });

    expect(result.current.theme).toBe("light");
  });
});

describe("useTheme — localStorage bi chan (private mode, AC-2)", () => {
  function blockStorage(): void {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("SecurityError: storage bi chan");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("SecurityError: storage bi chan");
    });
  }

  it("khong crash khi mount va roi ve prefers-color-scheme: dark", () => {
    blockStorage();
    mockMatchMedia(true);

    let hook: ReturnType<typeof mount> | undefined;
    expect(() => {
      hook = mount();
    }).not.toThrow();

    expect(hook?.result.current.theme).toBe("dark");
  });

  it("khong crash khi mount va roi ve light neu he thong sang", () => {
    blockStorage();
    mockMatchMedia(false);

    let hook: ReturnType<typeof mount> | undefined;
    expect(() => {
      hook = mount();
    }).not.toThrow();

    expect(hook?.result.current.theme).toBe("light");
  });

  it("setTheme van doi duoc data-theme du khong ghi duoc localStorage", () => {
    blockStorage();
    const { result } = mount();

    expect(() => {
      act(() => {
        result.current.setTheme("dark");
      });
    }).not.toThrow();

    expect(result.current.theme).toBe("dark");
    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
  });
});
