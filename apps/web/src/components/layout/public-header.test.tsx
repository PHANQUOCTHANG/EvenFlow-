/**
 * Test cho PublicHeader — AC-2 (landmark banner), AC-5 (khong link gia),
 * AC-6 (menu mobile).
 *
 * Hop dong da chot cho menu mobile (vi jsdom khong danh gia CSS):
 *   - DUNG MOT <nav>, LUON nam trong DOM va trong accessibility tree
 *     (khong `hidden`, khong `aria-hidden`) — an tren mobile la thuan CSS
 *   - trang thai doc qua HAI cho: `aria-expanded` cua nut toggle va
 *     `data-state="closed" | "open"` cua chinh <nav>
 *   - `aria-controls` cua nut = `id` cua <nav>
 * Vi vay o day KHONG assert "nav xuat hien / bien mat" theo queryByRole —
 * assert nhu vay se luon pass va khong bat duoc bug nao.
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { NavItem } from "./nav-config";
import { PublicHeader } from "./public-header";

const { pathnameMock } = vi.hoisted(() => ({ pathnameMock: vi.fn(() => "/") }));

vi.mock("next/navigation", () => ({
  usePathname: () => pathnameMock(),
}));

const NAV_LABEL = "Điều hướng chính";
const TOGGLE_LABEL = "Mở menu điều hướng";
const THEME_TOGGLE_LABEL = "Chuyển giao diện sáng/tối";

const ITEMS: NavItem[] = [
  { href: "/events", label: "Sự kiện", ready: true },
  { href: "/organizer", label: "Tổ chức", ready: true },
  { href: "/ve-cua-toi", label: "Vé của tôi", ready: false },
];

function nav(): HTMLElement {
  return screen.getByRole("navigation", { name: NAV_LABEL });
}

function toggle(): HTMLElement {
  return screen.getByRole("button", { name: TOGGLE_LABEL });
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

describe("PublicHeader — AC-2 cau truc landmark", () => {
  it("render landmark banner", () => {
    render(<PublicHeader items={ITEMS} />);
    expect(screen.getByRole("banner")).toBeInTheDocument();
  });

  it("render DUNG MOT phan tu <nav> (khong nhan doi desktop/mobile)", () => {
    const { container } = render(<PublicHeader items={ITEMS} />);
    expect(container.querySelectorAll("nav")).toHaveLength(1);
  });

  it('<nav> co aria-label "Điều hướng chính"', () => {
    render(<PublicHeader items={ITEMS} />);
    expect(nav()).toHaveAttribute("aria-label", NAV_LABEL);
  });

  it("<nav> LUON nam trong accessibility tree (khong hidden, khong aria-hidden)", () => {
    render(<PublicHeader items={ITEMS} />);
    const el = nav();
    expect(el).not.toHaveAttribute("hidden");
    expect(el).not.toHaveAttribute("aria-hidden", "true");
    expect(screen.queryAllByRole("navigation")).toHaveLength(1);
  });

  it("<nav> nam ben trong banner", () => {
    render(<PublicHeader items={ITEMS} />);
    expect(screen.getByRole("banner")).toContainElement(nav());
  });

  it("KHONG render <main> (main thuoc ve shell, khong thuoc header)", () => {
    const { container } = render(<PublicHeader items={ITEMS} />);
    expect(container.querySelectorAll("main")).toHaveLength(0);
  });
});

describe("PublicHeader — logo va ThemeToggle", () => {
  it('co link logo tro ve "/"', () => {
    const { container } = render(<PublicHeader items={ITEMS} />);
    const homeLinks = container.querySelectorAll('a[href="/"]');
    expect(homeLinks.length).toBeGreaterThan(0);
  });

  it("logo co accessible name (khong phai link rong)", () => {
    const { container } = render(<PublicHeader items={ITEMS} />);
    const logo = container.querySelector('a[href="/"]');
    expect(logo).not.toBeNull();
    expect((logo?.textContent ?? "").trim().length).toBeGreaterThan(0);
  });

  it("co ThemeToggle", () => {
    render(<PublicHeader items={ITEMS} />);
    expect(screen.getByRole("button", { name: THEME_TOGGLE_LABEL })).toBeInTheDocument();
  });
});

describe("PublicHeader — AC-5 render item theo ready", () => {
  it("item ready: true render <a> dung href", () => {
    render(<PublicHeader items={ITEMS} />);
    expect(screen.getByRole("link", { name: "Sự kiện" })).toHaveAttribute("href", "/events");
    expect(screen.getByRole("link", { name: "Tổ chức" })).toHaveAttribute("href", "/organizer");
  });

  it("item ready: false KHONG render <a>", () => {
    render(<PublicHeader items={ITEMS} />);
    expect(screen.queryByRole("link", { name: /Vé của tôi/ })).toBeNull();
  });

  it('item ready: false render phan tu aria-disabled kem nhan "Sắp có"', () => {
    const { container } = render(<PublicHeader items={ITEMS} />);
    const disabled = container.querySelector('[aria-disabled="true"]');
    expect(disabled).not.toBeNull();
    expect(disabled).toHaveTextContent("Vé của tôi");
    expect(disabled).toHaveTextContent("Sắp có");
  });

  it("href cua item chua san sang khong xuat hien trong DOM", () => {
    const { container } = render(<PublicHeader items={ITEMS} />);
    expect(container.innerHTML).not.toContain('href="/ve-cua-toi"');
  });

  it("khong truyen items thi dung PUBLIC_NAV mac dinh (nav co noi dung)", () => {
    render(<PublicHeader />);
    expect((nav().textContent ?? "").trim().length).toBeGreaterThan(0);
  });
});

describe("PublicHeader — AC-4 danh dau route dang mo", () => {
  it("item khop pathname co aria-current, item ben canh thi khong", () => {
    pathnameMock.mockReturnValue("/events");
    render(<PublicHeader items={ITEMS} />);

    expect(screen.getByRole("link", { name: "Sự kiện" })).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("link", { name: "Tổ chức" })).not.toHaveAttribute("aria-current");
  });

  it("route con van lam sang item cha", () => {
    pathnameMock.mockReturnValue("/events/le-hoi-am-nhac");
    render(<PublicHeader items={ITEMS} />);

    expect(screen.getByRole("link", { name: "Sự kiện" })).toHaveAttribute("aria-current", "page");
  });

  it('o "/" thi khong co item nav nao trong ITEMS active', () => {
    pathnameMock.mockReturnValue("/");
    render(<PublicHeader items={ITEMS} />);

    expect(screen.getByRole("link", { name: "Sự kiện" })).not.toHaveAttribute("aria-current");
    expect(screen.getByRole("link", { name: "Tổ chức" })).not.toHaveAttribute("aria-current");
  });

  /**
   * Bug M4 o cap shell: hai href long tien to nhau thi neu dat aria-current bang
   * isActive cho tung item, CA HAI cung sang. Shell phai dung activeHref.
   */
  it("hai href long tien to nhau: chi item KHOP DAI NHAT co aria-current", () => {
    const nested: NavItem[] = [
      { href: "/events", label: "Sự kiện", ready: true },
      { href: "/events/sap-dien-ra", label: "Sắp diễn ra", ready: true },
    ];
    pathnameMock.mockReturnValue("/events/sap-dien-ra");

    const { container } = render(<PublicHeader items={nested} />);

    expect(screen.getByRole("link", { name: "Sắp diễn ra" })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(screen.getByRole("link", { name: "Sự kiện" })).not.toHaveAttribute("aria-current");
    expect(container.querySelectorAll('[aria-current="page"]')).toHaveLength(1);
  });
});

describe("PublicHeader — AC-6 nut mo menu mobile", () => {
  it("co nut toggle voi accessible name tieng Viet theo hop dong", () => {
    render(<PublicHeader items={ITEMS} />);
    expect(toggle()).toBeInTheDocument();
    expect(toggle()).toHaveAccessibleName(TOGGLE_LABEL);
  });

  it('mac dinh aria-expanded = "false"', () => {
    render(<PublicHeader items={ITEMS} />);
    expect(toggle()).toHaveAttribute("aria-expanded", "false");
  });

  it('mac dinh <nav> co data-state = "closed"', () => {
    render(<PublicHeader items={ITEMS} />);
    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("aria-controls tro toi id cua <nav>, va phan tu do ton tai thuc su", () => {
    render(<PublicHeader items={ITEMS} />);

    const controls = toggle().getAttribute("aria-controls");
    expect(controls).toBeTruthy();
    expect(document.getElementById(controls as string)).not.toBeNull();
    expect(document.getElementById(controls as string)).toBe(nav());
  });
});

describe("PublicHeader — AC-6 mo / dong menu", () => {
  it('bam nut: aria-expanded doi "false" -> "true" va nav sang data-state "open"', async () => {
    const user = userEvent.setup();
    render(<PublicHeader items={ITEMS} />);

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");

    await user.click(toggle());

    expect(toggle()).toHaveAttribute("aria-expanded", "true");
    expect(nav()).toHaveAttribute("data-state", "open");
  });

  it("bam lan thu hai thi dong lai", async () => {
    const user = userEvent.setup();
    render(<PublicHeader items={ITEMS} />);

    await user.click(toggle());
    await user.click(toggle());

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("ten nut KHONG doi khi mo (trang thai da truyen bang aria-expanded)", async () => {
    const user = userEvent.setup();
    render(<PublicHeader items={ITEMS} />);

    await user.click(toggle());

    expect(screen.getByRole("button", { name: TOGGLE_LABEL })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("menu mo xong van chi co MOT <nav>", async () => {
    const user = userEvent.setup();
    const { container } = render(<PublicHeader items={ITEMS} />);

    await user.click(toggle());

    expect(container.querySelectorAll("nav")).toHaveLength(1);
  });

  it("Escape dong menu VA tra focus ve nut toggle", async () => {
    const user = userEvent.setup();
    render(<PublicHeader items={ITEMS} />);

    await user.click(toggle());
    expect(toggle()).toHaveAttribute("aria-expanded", "true");

    await user.keyboard("{Escape}");

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
    expect(document.activeElement).toBe(toggle());
  });

  it("Escape khi menu dang dong thi khong gay loi va van dong", async () => {
    const user = userEvent.setup();
    render(<PublicHeader items={ITEMS} />);

    await user.keyboard("{Escape}");

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("pathname doi (dieu huong) thi menu tu dong", async () => {
    const user = userEvent.setup();
    const view = render(<PublicHeader items={ITEMS} />);

    await user.click(toggle());
    expect(toggle()).toHaveAttribute("aria-expanded", "true");

    pathnameMock.mockReturnValue("/events");
    view.rerender(<PublicHeader items={ITEMS} />);

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("re-render ma pathname KHONG doi thi menu van mo (khong dong oan)", async () => {
    const user = userEvent.setup();
    const view = render(<PublicHeader items={ITEMS} />);

    await user.click(toggle());
    view.rerender(<PublicHeader items={ITEMS} />);

    expect(toggle()).toHaveAttribute("aria-expanded", "true");
    expect(nav()).toHaveAttribute("data-state", "open");
  });
});

/**
 * Bug M1 — thu tu DOM. Cho nay von da dung, nhung khong co test nao giu nen de bi
 * pha ve sau. Neu <nav> dung TRUOC nut toggle thi o mobile Tab tu nut di thang qua
 * nav (hoac vao main), va panel mo ra day chinh nut vua bam xuong duoi con tro.
 */
describe("PublicHeader — M1 thu tu DOM: toggle truoc <nav>", () => {
  it("nut toggle dung TRUOC <nav> trong DOM", () => {
    render(<PublicHeader items={ITEMS} />);

    const position = toggle().compareDocumentPosition(nav());
    expect(position & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("menu mo thi thu tu DOM khong doi (nav van sau toggle)", async () => {
    const user = userEvent.setup();
    render(<PublicHeader items={ITEMS} />);

    await user.click(toggle());

    expect(toggle().compareDocumentPosition(nav()) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

/**
 * Bug m4 — bam link trong menu phai dong menu.
 *
 * Effect theo pathname khong du: bam link TRUNG route dang mo thi pathname khong doi,
 * effect khong chay, menu nam do che ca trang.
 */
describe("PublicHeader — m4 bam link trong nav thi dong menu", () => {
  it("bam link ready: true -> menu dong, KE CA khi pathname khong doi", async () => {
    const user = userEvent.setup();
    pathnameMock.mockReturnValue("/events");
    render(<PublicHeader items={ITEMS} />);

    await user.click(toggle());
    expect(toggle()).toHaveAttribute("aria-expanded", "true");
    expect(nav()).toHaveAttribute("data-state", "open");

    // pathname KHONG doi: bam dung link cua trang dang mo.
    await user.click(screen.getByRole("link", { name: "Sự kiện" }));

    expect(pathnameMock).toHaveReturnedWith("/events");
    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("bam link sang route KHAC cung dong menu", async () => {
    const user = userEvent.setup();
    pathnameMock.mockReturnValue("/");
    render(<PublicHeader items={ITEMS} />);

    await user.click(toggle());
    await user.click(screen.getByRole("link", { name: "Tổ chức" }));

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("bam vao item ready: false (khong phai link) cung dong menu", async () => {
    const user = userEvent.setup();
    const { container } = render(<PublicHeader items={ITEMS} />);

    await user.click(toggle());
    const disabled = container.querySelector('[aria-disabled="true"]');
    expect(disabled).not.toBeNull();

    await user.click(disabled as Element);

    expect(nav()).toHaveAttribute("data-state", "closed");
  });

  it("menu dang dong thi bam link khong gay loi, van dong", async () => {
    const user = userEvent.setup();
    render(<PublicHeader items={ITEMS} />);

    await user.click(screen.getByRole("link", { name: "Sự kiện" }));

    expect(toggle()).toHaveAttribute("aria-expanded", "false");
    expect(nav()).toHaveAttribute("data-state", "closed");
  });
});
