/**
 * Test cho NavLink — AC-4 (danh dau route dang mo) va AC-5 (khong link gia).
 *
 * Day la component it dong code nhung nhieu bug nhat trong task:
 *   1. `aria-current` dat bang `startsWith` -> item "Trang chu" (href "/") sang o
 *      moi trang. Test "href / khong active khi pathname /organizer" bat duoc.
 *   2. `startsWith(href)` khong kem "/" -> "/organizer/events" sang khi dang o
 *      "/organizer/events-archive".
 *   3. `ready: false` van render <Link> (chi doi mau) -> nguoi dung bam -> 404.
 *      Spec §2.1 noi thang: KHONG render <a>. Test kiem bang queryByRole("link").
 *   4. `aria-current="false"` thay vi bo han attribute -> screen reader doc sai.
 *
 * `usePathname` duoc mock qua vi.hoisted de doi gia tri giua cac test.
 */
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as i18nProvider from "@/components/i18n/locale-provider";
import * as i18nMessages from "@/lib/i18n/messages";

import type { NavItem } from "./nav-config";
import { NavLink } from "./nav-link";

const { pathnameMock } = vi.hoisted(() => ({ pathnameMock: vi.fn(() => "/") }));

vi.mock("next/navigation", () => ({
  usePathname: () => pathnameMock(),
}));

const HOME: NavItem = { href: "/", label: "Trang chủ", ready: true };
const EVENTS: NavItem = { href: "/organizer/events", label: "Sự kiện", ready: true };
const REPORTS: NavItem = { href: "/organizer/reports", label: "Báo cáo", ready: true };
const SOON: NavItem = { href: "/organizer/payouts", label: "Giải ngân", ready: false };

function linkNamed(name: string): HTMLElement {
  const found = screen.getAllByRole("link", { name });
  expect(found).toHaveLength(1);
  return found[0];
}

beforeEach(() => {
  pathnameMock.mockReturnValue("/");
});

afterEach(() => {
  vi.clearAllMocks();
});

describe("NavLink — AC-5 item ready: true render link that", () => {
  it("render the <a> voi dung href", () => {
    pathnameMock.mockReturnValue("/organizer");
    render(<NavLink item={EVENTS} />);

    expect(linkNamed("Sự kiện")).toHaveAttribute("href", "/organizer/events");
  });

  it("accessible name dung bang label", () => {
    pathnameMock.mockReturnValue("/organizer");
    render(<NavLink item={EVENTS} />);

    expect(linkNamed("Sự kiện")).toHaveAccessibleName("Sự kiện");
  });

  it('KHONG co nhan "Sắp có" tren item ready: true', () => {
    pathnameMock.mockReturnValue("/organizer");
    render(<NavLink item={EVENTS} />);

    expect(screen.queryByText("Sắp có")).not.toBeInTheDocument();
  });

  it('KHONG co aria-disabled tren item ready: true', () => {
    pathnameMock.mockReturnValue("/organizer");
    const { container } = render(<NavLink item={EVENTS} />);

    expect(container.querySelector('[aria-disabled="true"]')).toBeNull();
  });

  it("className truyen vao duoc merge, khong ghi de class goc", () => {
    pathnameMock.mockReturnValue("/organizer");
    render(<NavLink item={EVENTS} className="lop-rieng-cua-toi" />);

    const link = linkNamed("Sự kiện");
    expect(link).toHaveClass("lop-rieng-cua-toi");
    expect(link.className.trim().split(/\s+/).length).toBeGreaterThan(1);
  });
});

describe("NavLink — AC-5 item ready: false KHONG duoc la link", () => {
  it("KHONG render the <a> nao", () => {
    pathnameMock.mockReturnValue("/organizer");
    const { container } = render(<NavLink item={SOON} />);

    expect(screen.queryByRole("link", { name: /Giải ngân/ })).toBeNull();
    expect(container.querySelector("a")).toBeNull();
  });

  it('render phan tu co aria-disabled="true"', () => {
    pathnameMock.mockReturnValue("/organizer");
    const { container } = render(<NavLink item={SOON} />);

    const disabled = container.querySelector('[aria-disabled="true"]');
    expect(disabled).not.toBeNull();
  });

  it('phan tu aria-disabled chua ca label va nhan "Sắp có"', () => {
    pathnameMock.mockReturnValue("/organizer");
    const { container } = render(<NavLink item={SOON} />);

    const disabled = container.querySelector('[aria-disabled="true"]');
    expect(disabled).toHaveTextContent("Giải ngân");
    expect(disabled).toHaveTextContent("Sắp có");
  });

  it("KHONG render href cua item o dau ca (khong de nguoi dung bam ra 404)", () => {
    pathnameMock.mockReturnValue("/organizer");
    const { container } = render(<NavLink item={SOON} />);

    expect(container.innerHTML).not.toContain(SOON.href);
  });

  it("KHONG focus duoc bang ban phim (khong co tabindex duong)", () => {
    pathnameMock.mockReturnValue("/organizer");
    const { container } = render(<NavLink item={SOON} />);

    const disabled = container.querySelector('[aria-disabled="true"]');
    expect(disabled).not.toHaveAttribute("href");
    expect(disabled?.getAttribute("tabindex")).not.toBe("0");
  });

  it("van KHONG la link ke ca khi pathname trung voi href", () => {
    pathnameMock.mockReturnValue(SOON.href);
    const { container } = render(<NavLink item={SOON} />);

    expect(container.querySelector("a")).toBeNull();
  });

  it("className truyen vao duoc merge tren ca item chua san sang", () => {
    pathnameMock.mockReturnValue("/organizer");
    const { container } = render(<NavLink item={SOON} className="lop-rieng-cua-toi" />);

    const disabled = container.querySelector('[aria-disabled="true"]');
    expect(disabled).toHaveClass("lop-rieng-cua-toi");
  });
});

describe("NavLink — AC-4 khop chinh xac", () => {
  it('pathname = href -> aria-current="page"', () => {
    pathnameMock.mockReturnValue("/organizer/events");
    render(<NavLink item={EVENTS} />);

    expect(linkNamed("Sự kiện")).toHaveAttribute("aria-current", "page");
  });

  it("item khong khop thi KHONG co aria-current (bo han attribute, khong phai \"false\")", () => {
    pathnameMock.mockReturnValue("/organizer/events");
    render(<NavLink item={REPORTS} />);

    expect(linkNamed("Báo cáo")).not.toHaveAttribute("aria-current");
  });
});

describe("NavLink — AC-4 route con lam sang item cha", () => {
  it("pathname /organizer/events/123 -> item /organizer/events van la page", () => {
    pathnameMock.mockReturnValue("/organizer/events/123");
    render(<NavLink item={EVENTS} />);

    expect(linkNamed("Sự kiện")).toHaveAttribute("aria-current", "page");
  });

  it("pathname /organizer/events/123/tickets -> item cha van la page", () => {
    pathnameMock.mockReturnValue("/organizer/events/123/tickets");
    render(<NavLink item={EVENTS} />);

    expect(linkNamed("Sự kiện")).toHaveAttribute("aria-current", "page");
  });

  it("tien to khong theo bien segment thi KHONG active", () => {
    pathnameMock.mockReturnValue("/organizer/events-archive");
    render(<NavLink item={EVENTS} />);

    expect(linkNamed("Sự kiện")).not.toHaveAttribute("aria-current");
  });
});

describe('NavLink — AC-4 item href="/" phai khop chinh xac', () => {
  it('pathname "/" -> item "/" la page', () => {
    pathnameMock.mockReturnValue("/");
    render(<NavLink item={HOME} />);

    expect(linkNamed("Trang chủ")).toHaveAttribute("aria-current", "page");
  });

  it('pathname "/organizer" -> item "/" KHONG active (bug "/" luon sang)', () => {
    pathnameMock.mockReturnValue("/organizer");
    render(<NavLink item={HOME} />);

    expect(linkNamed("Trang chủ")).not.toHaveAttribute("aria-current");
  });

  it('pathname "/events/le-hoi-am-nhac" -> item "/" KHONG active', () => {
    pathnameMock.mockReturnValue("/events/le-hoi-am-nhac");
    render(<NavLink item={HOME} />);

    expect(linkNamed("Trang chủ")).not.toHaveAttribute("aria-current");
  });
});

describe("NavLink — AC-4 chi dung mot item active trong nhom", () => {
  it("item dang mo co aria-current, cac item BEN CANH thi khong", () => {
    pathnameMock.mockReturnValue("/organizer/events");
    render(
      <ul>
        <li>
          <NavLink item={HOME} />
        </li>
        <li>
          <NavLink item={EVENTS} />
        </li>
        <li>
          <NavLink item={REPORTS} />
        </li>
      </ul>,
    );

    expect(linkNamed("Sự kiện")).toHaveAttribute("aria-current", "page");
    expect(linkNamed("Trang chủ")).not.toHaveAttribute("aria-current");
    expect(linkNamed("Báo cáo")).not.toHaveAttribute("aria-current");
  });

  it("dung mot phan tu aria-current trong ca nhom", () => {
    pathnameMock.mockReturnValue("/organizer/events/123");
    const { container } = render(
      <ul>
        <li>
          <NavLink item={HOME} />
        </li>
        <li>
          <NavLink item={EVENTS} />
        </li>
        <li>
          <NavLink item={REPORTS} />
        </li>
        <li>
          <NavLink item={SOON} />
        </li>
      </ul>,
    );

    expect(container.querySelectorAll('[aria-current="page"]')).toHaveLength(1);
  });

  it("khong trang nao khop thi khong co item nao active (tu tinh bang isActive)", () => {
    pathnameMock.mockReturnValue("/ops/queues");
    const { container } = render(
      <ul>
        <li>
          <NavLink item={HOME} />
        </li>
        <li>
          <NavLink item={EVENTS} />
        </li>
        <li>
          <NavLink item={REPORTS} />
        </li>
      </ul>,
    );

    expect(container.querySelectorAll("[aria-current]")).toHaveLength(0);
  });
});

/**
 * Prop `active` — ban sua cua bug M4 o cap danh sach.
 *
 * `isActive` khong the biet trong ca danh sach con item nao khop sat hon, nen khi 2
 * href long tien to nhau (`/organizer/events` va `/organizer/events/new`) thi ca 2
 * item cung sang. Cach sua: shell goi `activeHref(items, pathname)` de chon DUY NHAT
 * mot item roi truyen xuong qua `active`. NavLink van tu tinh khi khong duoc truyen,
 * de dung le van hoat dong.
 *
 * Vi vay `active` phai THANG pathname o ca hai chieu — bat len va tat di.
 */
describe("NavLink — prop `active` ghi de ket qua tu tinh", () => {
  it('active={true} tuy pathname KHONG khop item -> van co aria-current="page"', () => {
    pathnameMock.mockReturnValue("/ops/queues");
    render(<NavLink item={EVENTS} active />);

    expect(linkNamed("Sự kiện")).toHaveAttribute("aria-current", "page");
  });

  it("active={false} tuy pathname KHOP item -> KHONG co aria-current", () => {
    pathnameMock.mockReturnValue("/organizer/events");
    render(<NavLink item={EVENTS} active={false} />);

    expect(linkNamed("Sự kiện")).not.toHaveAttribute("aria-current");
  });

  it("active={false} tuy pathname la route con cua item -> KHONG co aria-current", () => {
    pathnameMock.mockReturnValue("/organizer/events/123");
    render(<NavLink item={EVENTS} active={false} />);

    expect(linkNamed("Sự kiện")).not.toHaveAttribute("aria-current");
  });

  it("khong truyen `active` -> hanh vi nhu cu (tu tinh bang isActive, truong hop khop)", () => {
    pathnameMock.mockReturnValue("/organizer/events");
    render(<NavLink item={EVENTS} />);

    expect(linkNamed("Sự kiện")).toHaveAttribute("aria-current", "page");
  });

  it("khong truyen `active` -> hanh vi nhu cu (tu tinh bang isActive, truong hop khong khop)", () => {
    pathnameMock.mockReturnValue("/ops/queues");
    render(<NavLink item={EVENTS} />);

    expect(linkNamed("Sự kiện")).not.toHaveAttribute("aria-current");
  });

  it("active={true} tren item ready: false van KHONG sinh the <a> (AC-5 khong bi pha)", () => {
    pathnameMock.mockReturnValue("/ops/queues");
    const { container } = render(<NavLink item={SOON} active />);

    expect(container.querySelector("a")).toBeNull();
  });

  it("`active` chi doi mot item, item ben canh khong bi anh huong", () => {
    pathnameMock.mockReturnValue("/organizer/events");
    const { container } = render(
      <ul>
        <li>
          <NavLink item={EVENTS} active={false} />
        </li>
        <li>
          <NavLink item={REPORTS} active />
        </li>
      </ul>,
    );

    expect(linkNamed("Báo cáo")).toHaveAttribute("aria-current", "page");
    expect(linkNamed("Sự kiện")).not.toHaveAttribute("aria-current");
    expect(container.querySelectorAll('[aria-current="page"]')).toHaveLength(1);
  });
});

// ---------------------------------------------------------------------------
// EV-188 (AC-8, plan §1.3 + §6 m5): NavLink hien t(labelKey) neu co, nguoc lai `label`;
// badge "Sắp có" vao tu dien.
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

async function renderEn(m: I18n, ui: React.ReactElement) {
  window.localStorage.setItem(LOCALE_KEY, "en");
  const result = render(<m.LocaleProvider>{ui}</m.LocaleProvider>);
  await waitFor(() => expect(document.documentElement.lang).toBe("en"));
  return result;
}

function itemWithKey(m: I18n, href: string, viLabel: string, ready: boolean): NavItem {
  const key = (Object.keys(m.vi) as Array<keyof typeof m.vi>).find((k) => m.vi[k] === viLabel);
  expect(key, `khong co khoa vi = ${viLabel}`).toBeDefined();
  return { href, label: viLabel, labelKey: key, ready } as NavItem;
}

describe("NavLink — i18n (EV-188 AC-8)", () => {
  afterEach(() => {
    window.localStorage.removeItem(LOCALE_KEY);
    document.documentElement.setAttribute("lang", "vi");
  });

  it("co labelKey + locale en: hien ban dich en", async () => {
    const m = i18n;
    pathnameMock.mockReturnValue("/");
    await renderEn(m, <NavLink item={itemWithKey(m, "/events", "Sự kiện", true)} />);

    const english = enFor(m, "Sự kiện");
    expect(await screen.findByRole("link", { name: english })).toHaveAttribute("href", "/events");
    expect(screen.queryByRole("link", { name: "Sự kiện" })).toBeNull();
  });

  it("co labelKey + locale vi (mac dinh): van la label tieng Viet", async () => {
    const m = i18n;
    pathnameMock.mockReturnValue("/");
    render(
      <m.LocaleProvider>
        <NavLink item={itemWithKey(m, "/events", "Sự kiện", true)} />
      </m.LocaleProvider>,
    );
    expect(linkNamed("Sự kiện")).toHaveAttribute("href", "/events");
  });

  it("KHONG co labelKey: hien label du dang o en", async () => {
    const m = i18n;
    pathnameMock.mockReturnValue("/");
    await renderEn(m, <NavLink item={REPORTS} />);
    expect(linkNamed("Báo cáo")).toHaveAttribute("href", "/organizer/reports");
  });

  it('badge "Sắp có" dich sang en khi locale en', async () => {
    const m = i18n;
    await renderEn(m, <NavLink item={SOON} />);
    expect(await screen.findByText(enFor(m, "Sắp có"))).toBeInTheDocument();
    expect(screen.queryByText("Sắp có")).toBeNull();
    expect(screen.queryByRole("link")).toBeNull();
  });

  it('badge "Sắp có" giu nguyen khi chua doi locale', () => {
    render(<NavLink item={SOON} />);
    expect(screen.getByText("Sắp có")).toBeInTheDocument();
  });
});

// ---------------------------------------------------------------------------
// EV-188 challenge F2 (minor): badge "Sắp có" duoc dich theo locale cho MOI nav, nhung nav workspace mang
// lang="vi" -> khi en, chu "Coming soon" bi khai bao la tieng Viet. Badge phai mang lang = locale
// (hoac ngon ngu hieu luc cua badge phai khop ngon ngu cua chu ben trong).
// ---------------------------------------------------------------------------
describe("NavLink — badge 'Sắp có' khai bao dung ngon ngu trong nav lang='vi' (EV-188 challenge F2)", () => {
  afterEach(() => {
    window.localStorage.removeItem(LOCALE_KEY);
    document.documentElement.setAttribute("lang", "vi");
  });

  it("locale en: chu badge la ban dich en va ngon ngu hieu luc cua badge la 'en'", async () => {
    const m = i18n;
    await renderEn(
      m,
      <nav lang="vi" aria-label="Điều hướng workspace">
        <NavLink item={SOON} />
      </nav>,
    );
    const badge = await screen.findByText(enFor(m, "Sắp có"));
    expect(badge.closest("[lang]")?.getAttribute("lang")).toBe("en");
    // Nhan muc nav chua dich van la tieng Viet.
    expect(screen.getByText(SOON.label).closest("[lang]")?.getAttribute("lang")).toBe("vi");
  });

  it("locale vi: badge 'Sắp có' co ngon ngu hieu luc 'vi'", () => {
    render(
      <i18n.LocaleProvider>
        <nav lang="vi" aria-label="Điều hướng workspace">
          <NavLink item={SOON} />
        </nav>
      </i18n.LocaleProvider>,
    );
    expect(screen.getByText("Sắp có").closest("[lang]")?.getAttribute("lang")).toBe("vi");
  });
});
