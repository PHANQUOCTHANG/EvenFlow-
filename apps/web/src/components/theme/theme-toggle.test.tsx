/**
 * Test cho ThemeToggle — AC-2.
 *
 * Nut toggle la cach duy nhat nguoi dung chon theme thu cong, nen aria-pressed
 * phai phan anh dung theme hien tai (screen reader doc "da bam"/"chua bam").
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
