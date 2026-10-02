/**
 * Test cho layout cua route group (ops) — AC-2.
 *
 * (ops) dung cung WorkspaceShell voi (organizer), nen bug de xay ra nhat la copy
 * nguyen file va quen doi ORGANIZER_NAV -> OPS_NAV. Test "moi label trong OPS_NAV
 * deu hien" + "khong hien label rieng cua ORGANIZER_NAV" bat duoc viec do.
 *
 * Luu y (spec §2.2): OPS_NAV la cau hinh hien thi, KHONG phai ma tran quyen.
 */
import { render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { OPS_NAV, ORGANIZER_NAV } from "@/components/layout/nav-config";

import OpsLayout from "./layout";

const { pathnameMock } = vi.hoisted(() => ({ pathnameMock: vi.fn(() => "/ops") }));

vi.mock("next/navigation", () => ({
  usePathname: () => pathnameMock(),
}));

const NAV_LABEL = "Điều hướng workspace";
const SKIP_LABEL = "Bỏ qua điều hướng, tới nội dung chính";
const CHILD = "Noi dung ops";

function renderLayout() {
  return render(
    <OpsLayout>
      <p>{CHILD}</p>
    </OpsLayout>,
  );
}

beforeEach(() => {
  pathnameMock.mockReturnValue("/ops");
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

afterEach(() => {
  vi.clearAllMocks();
  window.localStorage.clear();
  document.documentElement.removeAttribute("data-theme");
});

describe("(ops)/layout — AC-2 WorkspaceShell", () => {
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

describe("(ops)/layout — children va #main", () => {
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

describe("(ops)/layout — nav dung OPS_NAV, khong phai ORGANIZER_NAV", () => {
  it("moi label trong OPS_NAV deu hien trong nav", () => {
    renderLayout();
    const nav = screen.getByRole("navigation", { name: NAV_LABEL });

    for (const item of OPS_NAV) {
      expect(nav).toHaveTextContent(item.label);
    }
  });

  it("KHONG render link chi co rieng o ORGANIZER_NAV (bat loi copy-paste)", () => {
    const { container } = renderLayout();
    const opsHrefs = new Set(OPS_NAV.map((item) => item.href));
    const onlyOrganizer = ORGANIZER_NAV.filter((item) => item.ready && !opsHrefs.has(item.href));

    for (const item of onlyOrganizer) {
      expect(container.querySelector(`a[href="${item.href}"]`)).toBeNull();
    }
  });

  it("item ready: false KHONG render thanh <a> (AC-5)", () => {
    const { container } = renderLayout();

    for (const item of OPS_NAV.filter((nav) => !nav.ready)) {
      expect(container.querySelector(`a[href="${item.href}"]`)).toBeNull();
    }
  });

  it("item ready: true render thanh <a> dung href (AC-5)", () => {
    const { container } = renderLayout();

    for (const item of OPS_NAV.filter((nav) => nav.ready)) {
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
