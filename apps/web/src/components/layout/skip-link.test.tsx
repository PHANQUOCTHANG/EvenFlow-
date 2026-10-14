/**
 * Test cho SkipLink — AC-3.
 *
 * Bug kinh dien: an skip link bang `display: none` / `visibility: hidden` cho "gon".
 * Lam vay thi ban phim KHONG BAO GIO toi duoc no -> component ton tai nhung vo dung,
 * va khong co test nao do lam DOM van co the <a>. Vi vay o day kiem:
 *   - no nam trong accessibility tree (getByRole tim ra duoc)
 *   - Tab lan dau thi no nhan focus (tuc la focusable that)
 *   - khong co inline style display:none / visibility:hidden, khong aria-hidden,
 *     khong hidden, khong tabindex="-1"
 * Lop ao `sr-only`/`focus:not-sr-only` cua Tailwind khong chay trong jsdom nen
 * khong kiem duoc bang getComputedStyle — xem traceability.md.
 */
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import * as i18nProvider from "@/components/i18n/locale-provider";
import * as i18nMessages from "@/lib/i18n/messages";

import { SkipLink } from "./skip-link";

const SKIP_LABEL = "Bỏ qua điều hướng, tới nội dung chính";

function skipLink(): HTMLElement {
  return screen.getByRole("link", { name: SKIP_LABEL });
}

describe("SkipLink — hop dong copy va href", () => {
  it("render link voi dung noi dung tieng Viet theo hop dong", () => {
    render(<SkipLink />);
    expect(skipLink()).toBeInTheDocument();
    expect(skipLink()).toHaveAccessibleName(SKIP_LABEL);
  });

  it('mac dinh tro tới "#main" (targetId mac dinh "main")', () => {
    render(<SkipLink />);
    expect(skipLink()).toHaveAttribute("href", "#main");
  });

  it('targetId="main" cho ra dung href "#main"', () => {
    render(<SkipLink targetId="main" />);
    expect(skipLink()).toHaveAttribute("href", "#main");
  });

  it("targetId khac duoc ton trong", () => {
    render(<SkipLink targetId="noi-dung-chinh" />);
    expect(skipLink()).toHaveAttribute("href", "#noi-dung-chinh");
  });

  it("targetId khong duoc nhan doi dau '#'", () => {
    render(<SkipLink targetId="main" />);
    expect(skipLink().getAttribute("href")).not.toContain("##");
  });

  it("la the <a> that (khong phai <button> gia link)", () => {
    const { container } = render(<SkipLink />);
    const anchor = container.querySelector("a");
    expect(anchor).not.toBeNull();
    expect(anchor).toBe(skipLink());
  });
});

describe("SkipLink — AC-3 ban phim toi duoc", () => {
  it("nam trong accessibility tree (khong bi aria-hidden / hidden)", () => {
    render(<SkipLink />);
    const link = skipLink();
    expect(link).not.toHaveAttribute("aria-hidden", "true");
    expect(link).not.toHaveAttribute("hidden");
  });

  it('KHONG bi lay ra khoi tab order bang tabindex="-1"', () => {
    render(<SkipLink />);
    expect(skipLink()).not.toHaveAttribute("tabindex", "-1");
  });

  it("KHONG bi an bang inline display:none hay visibility:hidden", () => {
    render(<SkipLink />);
    const link = skipLink();
    expect(link.style.display).not.toBe("none");
    expect(link.style.visibility).not.toBe("hidden");
    expect(window.getComputedStyle(link).display).not.toBe("none");
    expect(window.getComputedStyle(link).visibility).not.toBe("hidden");
  });

  it("Tab lan dau thi SkipLink nhan focus (truoc moi phan tu khac)", async () => {
    const user = userEvent.setup();
    render(
      <>
        <SkipLink />
        <button type="button">Mua vé</button>
        <main id="main" tabIndex={-1}>
          Noi dung
        </main>
      </>,
    );

    await user.tab();

    expect(skipLink()).toHaveFocus();
    expect(document.activeElement).toBe(skipLink());
  });

  it("focus duoc truc tiep bang .focus()", () => {
    render(<SkipLink />);
    const link = skipLink();
    link.focus();
    expect(link).toHaveFocus();
  });
});

// ---------------------------------------------------------------------------
// EV-188 (AC-8): SkipLink dich qua <T>, van la server component.
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

describe("SkipLink — i18n (EV-188 AC-8)", () => {
  afterEach(resetLocale);

  it("locale en: link hien ban dich en, href giu nguyen", async () => {
    const m = i18n;
    await renderEn(m, <SkipLink />);
    const link = await screen.findByRole("link", { name: enFor(m, SKIP_LABEL) });
    expect(link).toHaveAttribute("href", "#main");
    expect(screen.queryByRole("link", { name: SKIP_LABEL })).toBeNull();
  });

  it("trong provider, chua doi locale: van la chuoi tieng Viet", async () => {
    const m = i18n;
    render(
      <m.LocaleProvider>
        <SkipLink />
      </m.LocaleProvider>,
    );
    expect(skipLink()).toHaveAccessibleName(SKIP_LABEL);
  });
});
