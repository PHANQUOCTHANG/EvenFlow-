/**
 * Test cho PublicShell — AC-2 (banner + main + contentinfo), AC-3 (SkipLink + main),
 * AC-6 (chi mot <nav>).
 *
 * Day la cho de lap bug "hai <main>" nhat: root layout da co cau truc, shell lai
 * boc them Container as="main". AC-2 doi DUNG MOT <main> nen kiem truc tiep bang
 * querySelectorAll.
 */
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import * as i18nProvider from "@/components/i18n/locale-provider";
import * as i18nMessages from "@/lib/i18n/messages";

import { PublicShell } from "./public-shell";

const { pathnameMock } = vi.hoisted(() => ({ pathnameMock: vi.fn(() => "/") }));

vi.mock("next/navigation", () => ({
  usePathname: () => pathnameMock(),
}));

const SKIP_LABEL = "Bỏ qua điều hướng, tới nội dung chính";
const CHILD = "Noi dung trang marketing";

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

describe("PublicShell — AC-2 landmark", () => {
  it("co banner, main va contentinfo", () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByRole("banner")).toBeInTheDocument();
    expect(screen.getByRole("main")).toBeInTheDocument();
    expect(screen.getByRole("contentinfo")).toBeInTheDocument();
  });

  it("render DUNG MOT <main>", () => {
    const { container } = render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(container.querySelectorAll("main")).toHaveLength(1);
  });

  it("render DUNG MOT <nav> (khong nhan doi desktop/mobile)", () => {
    const { container } = render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(container.querySelectorAll("nav")).toHaveLength(1);
    expect(screen.getAllByRole("navigation")).toHaveLength(1);
  });

  it("<nav> nam trong banner, khong nam trong main", () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    const nav = screen.getByRole("navigation");
    expect(screen.getByRole("banner")).toContainElement(nav);
    expect(screen.getByRole("main")).not.toContainElement(nav);
  });

  it("thu tu DOM: banner truoc main, main truoc contentinfo", () => {
    const { container } = render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    const html = container.innerHTML;
    expect(html.indexOf("<header")).toBeGreaterThanOrEqual(0);
    expect(html.indexOf("<header")).toBeLessThan(html.indexOf("<main"));
    expect(html.indexOf("<main")).toBeLessThan(html.indexOf("<footer"));
  });
});

describe("PublicShell — children", () => {
  it("render children", () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByText(CHILD)).toBeInTheDocument();
  });

  it("children nam BEN TRONG <main>", () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByRole("main")).toContainElement(screen.getByText(CHILD));
  });
});

describe("PublicShell — AC-3 SkipLink va muc tieu #main", () => {
  it('co SkipLink tro tới "#main"', () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByRole("link", { name: SKIP_LABEL })).toHaveAttribute("href", "#main");
  });

  it('<main> co id="main"', () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByRole("main")).toHaveAttribute("id", "main");
  });

  it('<main> co tabindex="-1" de nhan duoc focus sau khi nhay', () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    expect(screen.getByRole("main")).toHaveAttribute("tabindex", "-1");
  });

  it("href cua SkipLink khop dung id cua <main>", () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    const href = screen.getByRole("link", { name: SKIP_LABEL }).getAttribute("href") ?? "";
    expect(href.startsWith("#")).toBe(true);
    expect(document.getElementById(href.slice(1))).toBe(screen.getByRole("main"));
  });

  it("Tab lan dau trong shell thi SkipLink nhan focus", async () => {
    const user = userEvent.setup();
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );

    await user.tab();

    expect(screen.getByRole("link", { name: SKIP_LABEL })).toHaveFocus();
  });
});

// ---------------------------------------------------------------------------
// EV-188 (plan §6 M2): <main lang="vi"> vi noi dung chua dich la tieng Viet, ke ca khi html lang = en.
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

describe("PublicShell — lang cua noi dung (EV-188 M2)", () => {
  afterEach(resetLocale);

  it('<main> co lang="vi"', () => {
    render(
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );
    expect(screen.getByRole("main")).toHaveAttribute("lang", "vi");
  });

  it('locale en: html lang = "en" nhung <main> van lang="vi"; skip link dich sang en', async () => {
    const m = i18n;
    await renderEn(
      m,
      <PublicShell>
        <p>{CHILD}</p>
      </PublicShell>,
    );
    expect(document.documentElement.lang).toBe("en");
    expect(screen.getByRole("main")).toHaveAttribute("lang", "vi");
    expect(await screen.findByRole("link", { name: enFor(m, SKIP_LABEL) })).toHaveAttribute("href", "#main");
  });
});
