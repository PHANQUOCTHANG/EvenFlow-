/**
 * Test cho layout cua route group (organizer) — AC-2.
 *
 * (organizer) phai la WorkspaceShell: co landmark navigation + main, DUNG MOT <main>,
 * va nav phai duoc noi voi ORGANIZER_NAV (khong phai OPS_NAV — day la bug copy-paste
 * de xay ra nhat giua 2 group dung cung shell).
 *
 * Luu y (spec §2.2): ORGANIZER_NAV la cau hinh hien thi, KHONG phai ma tran quyen.
 */
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ORGANIZER_NAV } from "@/components/layout/nav-config";

import OrganizerLayout from "./layout";

const { pathnameMock } = vi.hoisted(() => ({ pathnameMock: vi.fn(() => "/organizer") }));

vi.mock("next/navigation", () => ({
  usePathname: () => pathnameMock(),
}));

const NAV_LABEL = "Điều hướng workspace";
const SKIP_LABEL = "Bỏ qua điều hướng, tới nội dung chính";
const CHILD = "Noi dung organizer";

function renderLayout() {
  return render(
    <OrganizerLayout>
      <p>{CHILD}</p>
    </OrganizerLayout>,
  );
}

beforeEach(() => {
  pathnameMock.mockReturnValue("/organizer");
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

afterEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("(organizer)/layout — AC-2 WorkspaceShell", () => {
  it("co landmark navigation", () => {
    renderLayout();
    expect(screen.getByRole("navigation", { name: NAV_LABEL })).toBeInTheDocument();
  });

  it("co landmark main", () => {
    renderLayout();
    expect(screen.getByRole("main")).toBeInTheDocument();
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

  it("nav LUON trong accessibility tree (khong hidden, khong aria-hidden)", () => {
    renderLayout();
    const nav = screen.getByRole("navigation", { name: NAV_LABEL });
    expect(nav).not.toHaveAttribute("hidden");
    expect(nav).not.toHaveAttribute("aria-hidden", "true");
  });

  it("co nut mo menu mobile voi aria-controls tro dung toi nav", () => {
    renderLayout();
    const nav = screen.getByRole("navigation", { name: NAV_LABEL });
    const toggle = screen.getByRole("button", { name: "Mở menu điều hướng" });

    const controls = toggle.getAttribute("aria-controls");
    expect(controls).toBeTruthy();
    expect(document.getElementById(controls as string)).toBe(nav);
  });
});

describe("(organizer)/layout — children va #main", () => {
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

describe("(organizer)/layout — noi dung nav theo vai tro", () => {
  it("moi label trong ORGANIZER_NAV deu hien trong nav", () => {
    renderLayout();
    const nav = screen.getByRole("navigation", { name: NAV_LABEL });

    for (const item of ORGANIZER_NAV) {
      expect(nav).toHaveTextContent(item.label);
    }
  });

  it("item ready: false KHONG render thanh <a> (AC-5)", () => {
    const { container } = renderLayout();

    for (const item of ORGANIZER_NAV.filter((nav) => !nav.ready)) {
      expect(container.querySelector(`a[href="${item.href}"]`)).toBeNull();
    }
  });

  it("item ready: true render thanh <a> dung href (AC-5)", () => {
    const { container } = renderLayout();

    for (const item of ORGANIZER_NAV.filter((nav) => nav.ready)) {
      expect(container.querySelector(`a[href="${item.href}"]`)).not.toBeNull();
    }
  });

  it('moi <a> deu co href khong rong va khac "#" (AC-5)', () => {
    const { container } = renderLayout();

    for (const anchor of Array.from(container.querySelectorAll("a"))) {
      const href = anchor.getAttribute("href");
      expect(href).toBeTruthy();
      expect(href).not.toBe("#");
    }
  });
});
