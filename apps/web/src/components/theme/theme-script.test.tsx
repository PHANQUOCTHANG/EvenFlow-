/**
 * Test cho ThemeScript — AC-2, phan CHONG FOUC.
 *
 * Day la test duy nhat chung minh duoc AC-2 phan "khong FOUC": script inline phai
 * chay DONG BO trong <head> truoc hydration va set data-theme ngay, neu khong thi
 * user chon dark se thay mot nhay trang truoc khi React kip chay.
 *
 * Cach test: khong so chuoi suong — lay noi dung script ra roi THUC SU chay no
 * trong jsdom voi localStorage / matchMedia da stub, sau do assert attribute
 * data-theme tren <html>.
 */
import { render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { THEME_STORAGE_KEY } from "../../lib/theme";
import { ThemeScript } from "./theme-script";

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

/** localStorage.getItem tra `value` cho dung khoa theme, null cho khoa khac. */
function stubStoredTheme(value: string | null): void {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation((key: string) =>
    key === THEME_STORAGE_KEY ? value : null,
  );
}

/** localStorage bi chan hoan toan (private mode): moi truy cap nem loi. */
function blockStorage(): void {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("SecurityError: storage bi chan");
  });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("SecurityError: storage bi chan");
  });
}

/**
 * Render ThemeScript, tra ve ma nguon ben trong <script>.
 * Reset data-theme sau khi render de khong lan voi ket qua cua lan chay thu cong.
 */
function scriptSource(): string {
  const { container } = render(<ThemeScript />);
  const scripts = container.querySelectorAll("script");
  expect(scripts).toHaveLength(1);
  const src = scripts[0].textContent ?? "";
  document.documentElement.removeAttribute("data-theme");
  return src;
}

/** Chay ma nguon script trong jsdom, giong luc browser chay script inline o <head>. */
function runScriptSource(src: string): void {
  new Function(src)();
}

function appliedTheme(): string | null {
  return document.documentElement.getAttribute("data-theme");
}

beforeEach(() => {
  document.documentElement.removeAttribute("data-theme");
  mockMatchMedia(false);
});

afterEach(() => {
  vi.restoreAllMocks();
  window.matchMedia = originalMatchMedia;
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("ThemeScript — cau truc", () => {
  it("render ra dung mot the <script>", () => {
    const { container } = render(<ThemeScript />);
    expect(container.querySelectorAll("script")).toHaveLength(1);
  });

  it("script khong rong", () => {
    expect(scriptSource().trim().length).toBeGreaterThan(0);
  });

  it("script doc localStorage bang dung khoa eventflow-theme", () => {
    const src = scriptSource();
    expect(src).toContain(THEME_STORAGE_KEY);
    expect(src).toMatch(/localStorage/);
  });

  it("script boc truy cap localStorage trong try/catch", () => {
    const src = scriptSource();
    expect(src).toMatch(/try/);
    expect(src).toMatch(/catch/);
  });

  it("script co nhanh prefers-color-scheme: dark", () => {
    expect(scriptSource()).toMatch(/prefers-color-scheme/);
  });

  it("script set data-theme len document.documentElement", () => {
    const src = scriptSource();
    expect(src).toMatch(/documentElement/);
    expect(src).toMatch(/data-theme|dataset\.theme/);
  });
});

describe("ThemeScript — chay thuc te: da co lua chon thu cong", () => {
  it('da luu "dark" thi set data-theme="dark"', () => {
    const src = scriptSource();
    stubStoredTheme("dark");

    runScriptSource(src);

    expect(appliedTheme()).toBe("dark");
  });

  it('da luu "light" thi set data-theme="light" (du he thong dang dark)', () => {
    const src = scriptSource();
    stubStoredTheme("light");
    mockMatchMedia(true);

    runScriptSource(src);

    // Day chinh la case FOUC nang nhat: he thong dark nhung user chon light.
    expect(appliedTheme()).toBe("light");
  });
});

describe("ThemeScript — chay thuc te: chua co lua chon thu cong", () => {
  it("chua luu gi + prefers-color-scheme: dark -> dark", () => {
    const src = scriptSource();
    stubStoredTheme(null);
    mockMatchMedia(true);

    runScriptSource(src);

    expect(appliedTheme()).toBe("dark");
  });

  it("chua luu gi + he thong sang -> KHONG duoc ra dark", () => {
    const src = scriptSource();
    stubStoredTheme(null);
    mockMatchMedia(false);

    runScriptSource(src);

    // Light la gia tri mac dinh cua :root nen script co the set "light" hoac bo trong,
    // nhung tuyet doi khong duoc ra "dark".
    expect(appliedTheme()).not.toBe("dark");
  });
});

describe("ThemeScript — localStorage bi chan (private mode, AC-2)", () => {
  it("khong nem loi khi localStorage bi chan", () => {
    const src = scriptSource();
    blockStorage();

    expect(() => runScriptSource(src)).not.toThrow();
  });

  it("localStorage bi chan + prefers-color-scheme: dark -> van ra dark", () => {
    const src = scriptSource();
    blockStorage();
    mockMatchMedia(true);

    runScriptSource(src);

    expect(appliedTheme()).toBe("dark");
  });

  it("localStorage bi chan + he thong sang -> KHONG duoc ra dark", () => {
    const src = scriptSource();
    blockStorage();
    mockMatchMedia(false);

    runScriptSource(src);

    expect(appliedTheme()).not.toBe("dark");
  });
});
