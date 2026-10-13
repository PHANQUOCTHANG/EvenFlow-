/**
 * Test cho ThemeToggle — AC-2.
 *
 * Nut toggle la cach duy nhat nguoi dung chon theme thu cong, nen aria-pressed
 * phai phan anh dung theme hien tai (screen reader doc "da bam"/"chua bam").
 */
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as i18nProvider from "@/components/i18n/locale-provider";
import * as i18nMessages from "@/lib/i18n/messages";

import { THEME_STORAGE_KEY } from "../../lib/theme";
import { ThemeToggle } from "./theme-toggle";

const TOGGLE_LABEL = "Chuyển giao diện sáng/tối";
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

function toggleButton(): HTMLElement {
  return screen.getByRole("button", { name: TOGGLE_LABEL });
}

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

describe("ThemeToggle", () => {
  it("render nut co aria-label tieng Viet theo hop dong", () => {
    render(<ThemeToggle />);
    expect(toggleButton()).toBeInTheDocument();
    expect(toggleButton()).toHaveAccessibleName(TOGGLE_LABEL);
  });

  it('aria-pressed = "false" khi dang o theme light', () => {
    document.documentElement.setAttribute("data-theme", "light");
    render(<ThemeToggle />);
    expect(toggleButton()).toHaveAttribute("aria-pressed", "false");
  });

  it('aria-pressed = "true" khi dang o theme dark', () => {
    document.documentElement.setAttribute("data-theme", "dark");
    render(<ThemeToggle />);
    expect(toggleButton()).toHaveAttribute("aria-pressed", "true");
  });

  it("bam mot lan: doi sang dark, ghi attribute va localStorage", async () => {
    const user = userEvent.setup();
    document.documentElement.setAttribute("data-theme", "light");
    render(<ThemeToggle />);

    await user.click(toggleButton());

    expect(document.documentElement).toHaveAttribute("data-theme", "dark");
    expect(toggleButton()).toHaveAttribute("aria-pressed", "true");
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("dark");
  });

  it("bam hai lan: tro ve light", async () => {
    const user = userEvent.setup();
    document.documentElement.setAttribute("data-theme", "light");
    render(<ThemeToggle />);

    await user.click(toggleButton());
    await user.click(toggleButton());

    expect(document.documentElement).toHaveAttribute("data-theme", "light");
    expect(toggleButton()).toHaveAttribute("aria-pressed", "false");
    expect(window.localStorage.getItem(THEME_STORAGE_KEY)).toBe("light");
  });

  it("la <button> that nen focus duoc bang ban phim", async () => {
    const user = userEvent.setup();
    render(<ThemeToggle />);

    await user.tab();

    expect(toggleButton()).toHaveFocus();
  });

  it("className truyen vao duoc merge, khong ghi de class goc", () => {
    render(<ThemeToggle className="lop-rieng-cua-toi" />);
    const btn = toggleButton();

    expect(btn).toHaveClass("lop-rieng-cua-toi");
    expect(btn.className.trim().split(/\s+/).length).toBeGreaterThan(1);
  });
});

// ---------------------------------------------------------------------------
// EV-188 (AC-8): ThemeToggle dich; ten truy cap chua chu hien thi o ca hai ngon ngu (WCAG 2.5.3).
// ---------------------------------------------------------------------------
const LOCALE_KEY = "eventflow-locale";

/** Tu dien + provider gop lai cho cac helper ben duoi. */
const i18n = { ...i18nMessages, ...i18nProvider };

type I18n = typeof i18n;

/** Ban dich en cua chuoi vi (tim khoa theo gia tri). */
function enFor(m: I18n, viText: string): string {
  const keys = (Object.keys(m.vi) as Array<keyof typeof m.vi>).filter((k) => m.vi[k] === viText);
  expect(keys.length, `khong co khoa vi = ${viText}`).toBeGreaterThan(0);
  const values = new Set(keys.map((k) => m.en[k]));
  expect(values.size, `cac khoa vi = ${viText} co ban dich en khac nhau`).toBe(1);
  const [english] = Array.from(values);
  expect(english).not.toBe(viText);
  return english;
}

/** Render trong LocaleProvider voi "en" da luu; cho toi khi locale da ap dung sau mount. */
async function renderEn(m: I18n, ui: React.ReactElement) {
  window.localStorage.setItem(LOCALE_KEY, "en");
  const result = render(<m.LocaleProvider>{ui}</m.LocaleProvider>);
  await waitFor(() => expect(document.documentElement.lang).toBe("en"));
  return result;
}

function resetLocale(): void {
  window.localStorage.removeItem(LOCALE_KEY);
  document.documentElement.setAttribute("lang", "vi");
}

describe("ThemeToggle — i18n (EV-188 AC-8)", () => {
  afterEach(resetLocale);

  it("ten truy cap chua chu hien thi (khong phan biet hoa thuong) o vi", () => {
    document.documentElement.setAttribute("data-theme", "light");
    render(<ThemeToggle />);
    const visible = (toggleButton().textContent ?? "").replace(/[☀☾]/g, "").trim();
    expect(TOGGLE_LABEL.toLowerCase()).toContain(visible.toLowerCase());
  });

  it("locale en + light: aria-label va chu hien thi la ban dich en", async () => {
    const m = i18n;
    document.documentElement.setAttribute("data-theme", "light");
    await renderEn(m, <ThemeToggle />);

    const btn = await screen.findByRole("button", { name: enFor(m, TOGGLE_LABEL) });
    expect(btn).toHaveTextContent(enFor(m, "Sáng"));
    expect(btn).not.toHaveTextContent("Sáng");
    expect(btn).toHaveAttribute("aria-pressed", "false");
    expect(enFor(m, TOGGLE_LABEL).toLowerCase()).toContain(enFor(m, "Sáng").toLowerCase());
  });

  it("locale en + dark: chu hien thi la ban dich en cua Tối", async () => {
    const m = i18n;
    document.documentElement.setAttribute("data-theme", "dark");
    await renderEn(m, <ThemeToggle />);

    const btn = await screen.findByRole("button", { name: enFor(m, TOGGLE_LABEL) });
    await waitFor(() => expect(btn).toHaveAttribute("aria-pressed", "true"));
    expect(btn).toHaveTextContent(enFor(m, "Tối"));
    expect(enFor(m, TOGGLE_LABEL).toLowerCase()).toContain(enFor(m, "Tối").toLowerCase());
  });
});
