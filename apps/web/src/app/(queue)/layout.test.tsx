/**
 * Test cho layout cua route group (queue) — AC-2, phan NEGATIVE.
 *
 * Spec §2.3 la mot quyet dinh san pham, khong phai trang tri: nguoi dung o trang
 * xep hang dang giu suat co han. Them nav vao day la moi ho roi luong dung luc
 * te nhat. Vi vay test o day kiem KHONG CO nav, va kiem ca "khong co link sang
 * khu vuc khac" — chi cho phep link ve "/" (logo) va anchor "#main" (SkipLink).
 *
 * Day la loai test de bi xoa oan khi ai do "them nav cho tien". Comment nay la
 * ly do de khong xoa.
 */
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import QueueLayout from "./layout";

const { pathnameMock } = vi.hoisted(() => ({ pathnameMock: vi.fn(() => "/waiting/abc") }));

vi.mock("next/navigation", () => ({
  usePathname: () => pathnameMock(),
}));

const SKIP_LABEL = "Bỏ qua điều hướng, tới nội dung chính";
const CHILD = "Dang xep hang";
const ALLOWED_HREFS = ["/", "#main"];

function renderLayout() {
  return render(
    <QueueLayout>
      <p>{CHILD}</p>
    </QueueLayout>,
  );
}

beforeEach(() => {
  pathnameMock.mockReturnValue("/waiting/abc");
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

afterEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("(queue)/layout — AC-2 shell toi gian", () => {
  it("render DUNG MOT <main>", () => {
    const { container } = renderLayout();
    expect(container.querySelectorAll("main")).toHaveLength(1);
  });

  it("co landmark main", () => {
    renderLayout();
    expect(screen.getByRole("main")).toBeInTheDocument();
  });

  it("render children", () => {
    renderLayout();
    expect(screen.getByText(CHILD)).toBeInTheDocument();
  });

  it("children nam BEN TRONG <main>", () => {
    renderLayout();
    expect(screen.getByRole("main")).toContainElement(screen.getByText(CHILD));
  });

  it('<main> co id="main" va tabindex="-1" (AC-3)', () => {
    renderLayout();
    const main = screen.getByRole("main");
    expect(main).toHaveAttribute("id", "main");
    expect(main).toHaveAttribute("tabindex", "-1");
  });
});

describe("(queue)/layout — AC-2 KHONG co dieu huong (spec §2.3)", () => {
  it("KHONG co landmark navigation nao", () => {
    renderLayout();
    expect(screen.queryAllByRole("navigation")).toHaveLength(0);
  });

  it("KHONG co the <nav> nao", () => {
    const { container } = renderLayout();
    expect(container.querySelectorAll("nav")).toHaveLength(0);
  });

  it('KHONG co nut mo menu (khong co button[aria-expanded])', () => {
    const { container } = renderLayout();
    expect(container.querySelector("button[aria-expanded]")).toBeNull();
  });

  it('KHONG co link sang khu vuc khac (chi cho phep "/" va "#main")', () => {
    const { container } = renderLayout();

    for (const anchor of Array.from(container.querySelectorAll("a"))) {
      expect(ALLOWED_HREFS).toContain(anchor.getAttribute("href"));
    }
  });

  it("KHONG co link tro vao /organizer, /ops hay /events", () => {
    const { container } = renderLayout();

    expect(container.querySelector('a[href^="/organizer"]')).toBeNull();
    expect(container.querySelector('a[href^="/ops"]')).toBeNull();
    expect(container.querySelector('a[href^="/events"]')).toBeNull();
  });

  it('neu co SkipLink thi phai tro tới "#main"', () => {
    renderLayout();
    const skip = screen.queryByRole("link", { name: SKIP_LABEL });

    if (skip) {
      expect(skip).toHaveAttribute("href", "#main");
    } else {
      expect(skip).toBeNull();
    }
  });
});
