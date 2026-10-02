/**
 * Test cho WorkspaceShell — AC-2 (navigation + main), AC-3 (SkipLink + main),
 * AC-4 (active route), AC-5 (khong link gia), AC-6 (sidebar thu gon tren mobile).
 *
 * Hop dong da chot, giong PublicHeader:
 *   - DUNG MOT <nav>, aria-label "Điều hướng workspace", LUON trong accessibility
 *     tree (khong `hidden`, khong `aria-hidden`); an tren mobile la thuan CSS
 *   - trang thai doc qua `aria-expanded` cua nut toggle + `data-state` cua <nav>
 *   - `aria-controls` cua nut = `id` cua <nav>
 *
 * Luu y (spec §2.2): `items` la CAU HINH HIEN THI theo vai tro, KHONG phai ma tran
 * quyen. Test o day khong duoc hieu la kiem authorization.
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { NavItem } from "./nav-config";
import { WorkspaceShell } from "./workspace-shell";

const { pathnameMock } = vi.hoisted(() => ({ pathnameMock: vi.fn(() => "/organizer") }));

vi.mock("next/navigation", () => ({
  usePathname: () => pathnameMock(),
}));

const NAV_LABEL = "Điều hướng workspace";
const TOGGLE_LABEL = "Mở menu điều hướng";
const SKIP_LABEL = "Bỏ qua điều hướng, tới nội dung chính";
const TITLE = "Bảng điều khiển tổ chức";
const CHILD = "Noi dung workspace";

const ITEMS: NavItem[] = [
  { href: "/", label: "Trang chủ", ready: true },
  { href: "/organizer/events", label: "Sự kiện", ready: true },
  { href: "/organizer/reports", label: "Báo cáo", ready: true },
  { href: "/organizer/payouts", label: "Giải ngân", ready: false },
];

function renderShell() {
  return render(
    <WorkspaceShell title={TITLE} items={ITEMS}>
      <p>{CHILD}</p>
    </WorkspaceShell>,
  );
}

function nav(): HTMLElement {
  return screen.getByRole("navigation", { name: NAV_LABEL });
}

function toggle(): HTMLElement {
  return screen.getByRole("button", { name: TOGGLE_LABEL });
}

beforeEach(() => {
  pathnameMock.mockReturnValue("/organizer");
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("WorkspaceShell — AC-2 landmark", () => {
  it("co landmark navigation va main", () => {
    renderShell();

    expect(nav()).toBeInTheDocument();
    expect(screen.getByRole("main")).toBeInTheDocument();
  });

  it("render DUNG MOT <main>", () => {
    const { container } = renderShell();
    expect(container.querySelectorAll("main")).toHaveLength(1);
  });

  it("render DUNG MOT <nav> (khong nhan doi sidebar/mobile)", () => {
    const { container } = renderShell();
    expect(container.querySelectorAll("nav")).toHaveLength(1);
    expect(screen.getAllByRole("navigation")).toHaveLength(1);
  });

  it('<nav> co aria-label "Điều hướng workspace" (phan biet voi nav public)', () => {
    renderShell();
    expect(nav()).toHaveAttribute("aria-label", NAV_LABEL);
  });

  it("<nav> LUON trong accessibility tree (khong hidden, khong aria-hidden)", () => {
    renderShell();
    expect(nav()).not.toHaveAttribute("hidden");
    expect(nav()).not.toHaveAttribute("aria-hidden", "true");
  });

  it("<nav> KHONG nam trong <main>", () => {
    renderShell();
    expect(screen.getByRole("main")).not.toContainElement(nav());
  });
});

describe("WorkspaceShell — title o topbar va children", () => {
  it("hien title truyen vao", () => {
    renderShell();
    expect(screen.getByText(TITLE)).toBeInTheDocument();
  });

  it("title nam o topbar, KHONG nam trong <main>", () => {
    renderShell();
    expect(screen.getByRole("main")).not.toContainElement(screen.getByText(TITLE));
  });

  it("render children", () => {
    renderShell();
    expect(screen.getByText(CHILD)).toBeInTheDocument();
  });

  it("children nam BEN TRONG <main>", () => {
    renderShell();
    expect(screen.getByRole("main")).toContainElement(screen.getByText(CHILD));
  });
});

describe("WorkspaceShell — AC-3 SkipLink va muc tieu #main", () => {
  it('co SkipLink tro tới "#main"', () => {
    renderShell();
    expect(screen.getByRole("link", { name: SKIP_LABEL })).toHaveAttribute("href", "#main");
  });

  it('<main> co id="main" va tabindex="-1"', () => {
    renderShell();
    const main = screen.getByRole("main");
    expect(main).toHaveAttribute("id", "main");
    expect(main).toHaveAttribute("tabindex", "-1");
  });

  it("Tab lan dau thi SkipLink nhan focus (truoc ca nut toggle va sidebar)", async () => {
    const user = userEvent.setup();
    renderShell();

    await user.tab();

    expect(screen.getByRole("link", { name: SKIP_LABEL })).toHaveFocus();
  });
});

describe("WorkspaceShell — AC-5 render item theo ready", () => {
  it("item ready: true render <a> dung href", () => {
    renderShell();

    expect(screen.getByRole("link", { name: "Sự kiện" })).toHaveAttribute(
      "href",
      "/organizer/events",
    );
    expect(screen.getByRole("link", { name: "Báo cáo" })).toHaveAttribute(
      "href",
      "/organizer/reports",
    );
  });

  it("item ready: false KHONG render <a>", () => {
    renderShell();
    expect(screen.queryByRole("link", { name: /Giải ngân/ })).toBeNull();
  });

  it('item ready: false render aria-disabled kem nhan "Sắp có"', () => {
    const { container } = renderShell();

    const disabled = container.querySelector('[aria-disabled="true"]');
    expect(disabled).not.toBeNull();
    expect(disabled).toHaveTextContent("Giải ngân");
    expect(disabled).toHaveTextContent("Sắp có");
  });

  it("href cua item chua san sang khong xuat hien trong DOM", () => {
    const { container } = renderShell();
    expect(container.innerHTML).not.toContain('href="/organizer/payouts"');
  });

  it("moi item trong items deu duoc hien (ready hay chua)", () => {
    renderShell();

    for (const item of ITEMS) {
      expect(nav()).toHaveTextContent(item.label);
    }
  });
});

describe("WorkspaceShell — AC-4 danh dau route dang mo", () => {
  it("item khop pathname co aria-current, item ben canh thi khong", () => {
    pathnameMock.mockReturnValue("/organizer/events");
    renderShell();

    expect(screen.getByRole("link", { name: "Sự kiện" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Báo cáo" })).not.toHaveAttribute("aria-current");
  });

  it("route con van lam sang item cha", () => {
    pathnameMock.mockReturnValue("/organizer/events/123");
    renderShell();

    expect(screen.getByRole("link", { name: "Sự kiện" })).toHaveAttribute("aria-current", "page");
  });

  it('item href="/" KHONG active khi dang o /organizer', () => {
    pathnameMock.mockReturnValue("/organizer");
    renderShell();

    expect(screen.getByRole("link", { name: "Trang chủ" })).not.toHaveAttribute("aria-current");
  });

  it("chi dung mot item co aria-current", () => {
    pathnameMock.mockReturnValue("/organizer/events/123");
    const { container } = renderShell();

    expect(container.querySelectorAll('[aria-current="page"]')).toHaveLength(1);
  });
});

describe("WorkspaceShell — AC-6 sidebar thu gon tren mobile", () => {
  it("co nut toggle voi accessible name theo hop dong", () => {
    renderShell();
    expect(toggle()).toHaveAccessibleName(TOGGLE_LABEL);
  });

  it('mac dinh aria-expanded = "false" va nav data-state = "closed"', () => {
    renderShell();
    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("aria-controls tro dung toi <nav> va phan tu do ton tai", () => {
    renderShell();

    const controls = toggle().getAttribute("aria-controls");
    expect(controls).toBeTruthy();
    expect(document.getElementById(controls as string)).toBe(nav());
  });

  it('bam nut: aria-expanded "false" -> "true", nav data-state -> "open"', async () => {
    const user = userEvent.setup();
    renderShell();

    await user.click(toggle());

    expect(toggle()).toHaveAttribute("aria-expanded", "true");
    expect(nav()).toHaveAttribute("data-state", "open");
  });

  it("bam lan thu hai thi dong lai", async () => {
    const user = userEvent.setup();
    renderShell();

    await user.click(toggle());
    await user.click(toggle());

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("Escape dong menu VA tra focus ve nut toggle", async () => {
    const user = userEvent.setup();
    renderShell();

    await user.click(toggle());
    await user.keyboard("{Escape}");

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
    expect(document.activeElement).toBe(toggle());
  });

  it("pathname doi (dieu huong) thi menu tu dong", async () => {
    const user = userEvent.setup();
    const view = render(
      <WorkspaceShell title={TITLE} items={ITEMS}>
        <p>{CHILD}</p>
      </WorkspaceShell>,
    );

    await user.click(toggle());
    expect(toggle()).toHaveAttribute("aria-expanded", "true");

    pathnameMock.mockReturnValue("/organizer/events");
    view.rerender(
      <WorkspaceShell title={TITLE} items={ITEMS}>
        <p>{CHILD}</p>
      </WorkspaceShell>,
    );

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("menu mo xong van chi co MOT <nav>", async () => {
    const user = userEvent.setup();
    const { container } = renderShell();

    await user.click(toggle());

    expect(container.querySelectorAll("nav")).toHaveLength(1);
  });
});
