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

  /**
   * Bug M2. `title` duoc hardcode o layout cua route group, nen neu shell render no
   * bang <h1> thi MOI route trong group dung chung mot <h1> vo nghia; va page nao tu
   * dat <h1> theo pattern cua repo (xem `(marketing)/page.tsx`) se tao <h1> THU HAI.
   * Shell khong duoc chiem <h1> — day la cua page.
   */
  it("shell KHONG chiem <h1> (title la <p>, h1 danh cho page)", () => {
    const { container } = renderShell();
    expect(container.querySelectorAll("h1")).toHaveLength(0);
  });

  it("phan tu chua title khong phai the heading (h1..h6)", () => {
    renderShell();
    expect(["H1", "H2", "H3", "H4", "H5", "H6"]).not.toContain(
      screen.getByText(TITLE).tagName,
    );
  });

  it("khong co heading level 1 nao do shell sinh ra", () => {
    renderShell();
    expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
  });

  it("page tu dat <h1> thi van chi co DUNG MOT <h1> trong ca trang", () => {
    const { container } = render(
      <WorkspaceShell title={TITLE} items={ITEMS}>
        <h1>Sự kiện của tôi</h1>
      </WorkspaceShell>,
    );

    expect(container.querySelectorAll("h1")).toHaveLength(1);
    expect(screen.getByRole("heading", { level: 1 })).toHaveTextContent("Sự kiện của tôi");
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

  /**
   * Bug M4 o cap shell. Hai href long tien to nhau trong cung danh sach: neu shell
   * dat aria-current bang isActive cho tung item thi CA HAI cung sang va screen
   * reader bao hai "trang hien tai". Shell phai dung activeHref de chon mot.
   */
  it("hai href long tien to nhau: chi item KHOP DAI NHAT co aria-current", () => {
    const nested: NavItem[] = [
      { href: "/organizer/events", label: "Sự kiện", ready: true },
      { href: "/organizer/events/new", label: "Tạo sự kiện", ready: true },
    ];
    pathnameMock.mockReturnValue("/organizer/events/new");

    const { container } = render(
      <WorkspaceShell title={TITLE} items={nested}>
        <p>{CHILD}</p>
      </WorkspaceShell>,
    );

    expect(screen.getByRole("link", { name: "Tạo sự kiện" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Sự kiện" })).not.toHaveAttribute("aria-current");
    expect(container.querySelectorAll('[aria-current="page"]')).toHaveLength(1);
  });

  it("hai href long tien to nhau, o route cha: chi item cha co aria-current", () => {
    const nested: NavItem[] = [
      { href: "/organizer/events", label: "Sự kiện", ready: true },
      { href: "/organizer/events/new", label: "Tạo sự kiện", ready: true },
    ];
    pathnameMock.mockReturnValue("/organizer/events");

    const { container } = render(
      <WorkspaceShell title={TITLE} items={nested}>
        <p>{CHILD}</p>
      </WorkspaceShell>,
    );

    expect(screen.getByRole("link", { name: "Sự kiện" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Tạo sự kiện" })).not.toHaveAttribute("aria-current");
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

/**
 * Bug M1 — thu tu DOM.
 *
 * Neu <nav> dung TRUOC nut toggle thi o mobile: Tab tu nut di thang vao <main>,
 * khong toi duoc link vua hien (phai Shift+Tab nguoc lai), va panel mo ra o phia
 * tren topbar day chinh nut vua bam xuong duoi con tro. Thu tu dung la
 * topbar (toggle) -> nav -> main.
 */
describe("WorkspaceShell — M1 thu tu DOM topbar -> nav -> main", () => {
  it("nut toggle dung TRUOC <nav> trong DOM", () => {
    renderShell();

    const position = toggle().compareDocumentPosition(nav());
    expect(position & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("<nav> dung TRUOC <main> trong DOM", () => {
    renderShell();

    const position = nav().compareDocumentPosition(screen.getByRole("main"));
    expect(position & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("link dau tien trong <nav> dung TRUOC <main> (ly do cua M1: Tab khong nhay qua nav)", () => {
    renderShell();

    const firstNavLink = nav().querySelector("a");
    expect(firstNavLink).not.toBeNull();
    expect(
      (firstNavLink as Element).compareDocumentPosition(screen.getByRole("main")) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });

  it("menu mo thi thu tu DOM khong doi (nav van sau toggle, truoc main)", async () => {
    const user = userEvent.setup();
    renderShell();

    await user.click(toggle());

    expect(toggle().compareDocumentPosition(nav()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(
      nav().compareDocumentPosition(screen.getByRole("main")) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
  });
});

/**
 * Bug m4 — bam link trong menu phai dong menu.
 *
 * Effect theo pathname khong du: bam link TRUNG route dang mo thi pathname khong
 * doi, effect khong chay, menu nam do che ca trang. Vi vay <nav> co onClick dong
 * menu ngay khi co click ben trong no.
 */
describe("WorkspaceShell — m4 bam link trong nav thi dong menu", () => {
  it("bam link ready: true -> menu dong, KE CA khi pathname khong doi", async () => {
    const user = userEvent.setup();
    pathnameMock.mockReturnValue("/organizer/events");
    renderShell();

    await user.click(toggle());
    expect(toggle()).toHaveAttribute("aria-expanded", "true");
    expect(nav()).toHaveAttribute("data-state", "open");

    // pathname KHONG doi: bam dung link cua trang dang mo.
    await user.click(screen.getByRole("link", { name: "Sự kiện" }));

    expect(pathnameMock).toHaveReturnedWith("/organizer/events");
    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("bam link sang route KHAC cung dong menu", async () => {
    const user = userEvent.setup();
    pathnameMock.mockReturnValue("/organizer");
    renderShell();

    await user.click(toggle());
    await user.click(screen.getByRole("link", { name: "Báo cáo" }));

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("bam vao item ready: false (khong phai link) cung dong menu", async () => {
    const user = userEvent.setup();
    const { container } = renderShell();

    await user.click(toggle());
    const disabled = container.querySelector('[aria-disabled="true"]');
    expect(disabled).not.toBeNull();

    await user.click(disabled as Element);

    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("menu dang dong thi bam link khong gay loi, van dong", async () => {
    const user = userEvent.setup();
    renderShell();

    await user.click(screen.getByRole("link", { name: "Sự kiện" }));

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });
});
