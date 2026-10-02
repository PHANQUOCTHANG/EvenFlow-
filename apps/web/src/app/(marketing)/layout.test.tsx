/**
 * Test cho layout cua route group (marketing) — AC-2.
 *
 * Route group khong co page nao thi Next khong build-time render layout, nen
 * coverage phai den tu test truc tiep nhu o day (spec §6, bang Rui ro).
 *
 * (marketing) phai la PublicShell: banner + main + contentinfo, va DUNG MOT <main>.
 */
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { PUBLIC_NAV } from "@/components/layout/nav-config";

import MarketingLayout from "./layout";

const { pathnameMock } = vi.hoisted(() => ({ pathnameMock: vi.fn(() => "/") }));

vi.mock("next/navigation", () => ({
  usePathname: () => pathnameMock(),
}));

const SKIP_LABEL = "Bỏ qua điều hướng, tới nội dung chính";
const CHILD = "Noi dung trang marketing";

function renderLayout() {
  return render(
    <MarketingLayout>
      <p>{CHILD}</p>
    </MarketingLayout>,
  );
}

beforeEach(() => {
  pathnameMock.mockReturnValue("/");
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

afterEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("(marketing)/layout — AC-2 PublicShell", () => {
  it("co landmark banner", () => {
    renderLayout();
    expect(screen.getByRole("banner")).toBeInTheDocument();
  });

  it("co landmark main", () => {
    renderLayout();
    expect(screen.getByRole("main")).toBeInTheDocument();
  });

  it("co landmark contentinfo", () => {
    renderLayout();
    expect(screen.getByRole("contentinfo")).toBeInTheDocument();
  });

  it("render DUNG MOT <main>", () => {
    const { container } = renderLayout();
    expect(container.querySelectorAll("main")).toHaveLength(1);
  });

  it("render DUNG MOT <nav>", () => {
    const { container } = renderLayout();
    expect(container.querySelectorAll("nav")).toHaveLength(1);
    expect(screen.getAllByRole("navigation")).toHaveLength(1);
  });

  it("nav dung PUBLIC_NAV (moi label trong config deu hien)", () => {
    renderLayout();
    const nav = screen.getByRole("navigation");

    for (const item of PUBLIC_NAV) {
      expect(nav).toHaveTextContent(item.label);
    }
  });
});

describe("(marketing)/layout — children va #main", () => {
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

  it('co SkipLink tro tới "#main" (AC-3)', () => {
    renderLayout();
    expect(screen.getByRole("link", { name: SKIP_LABEL })).toHaveAttribute("href", "#main");
  });
});

describe("(marketing)/layout — AC-5 khong link gia", () => {
  it('moi <a> deu co href khong rong va khac "#"', () => {
    const { container } = renderLayout();

    for (const anchor of Array.from(container.querySelectorAll("a"))) {
      const href = anchor.getAttribute("href");
      expect(href).toBeTruthy();
      expect(href).not.toBe("#");
    }
  });

  it("item PUBLIC_NAV co ready: false KHONG render thanh <a>", () => {
    const { container } = renderLayout();

    for (const item of PUBLIC_NAV.filter((nav) => !nav.ready)) {
      expect(container.querySelector(`a[href="${item.href}"]`)).toBeNull();
    }
  });

  it("item PUBLIC_NAV co ready: true render thanh <a> dung href", () => {
    const { container } = renderLayout();

    for (const item of PUBLIC_NAV.filter((nav) => nav.ready)) {
      expect(container.querySelector(`a[href="${item.href}"]`)).not.toBeNull();
    }
  });
});
