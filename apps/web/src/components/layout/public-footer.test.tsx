/**
 * Test cho PublicFooter — AC-2 (landmark contentinfo) va AC-5 (khong link gia).
 *
 * Hop dong da chot: moi shell chi co DUNG MOT <nav>, va no nam o header.
 * Vi vay footer khong duoc tu mo thêm mot <nav> — neu co, PublicShell se co 2
 * landmark navigation va AC-2 doc sai.
 */
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import * as i18nProvider from "@/components/i18n/locale-provider";
import * as i18nMessages from "@/lib/i18n/messages";

import { PublicFooter } from "./public-footer";

describe("PublicFooter — AC-2 landmark", () => {
  it("render landmark contentinfo", () => {
    render(<PublicFooter />);
    expect(screen.getByRole("contentinfo")).toBeInTheDocument();
  });

  it("la the <footer> that", () => {
    const { container } = render(<PublicFooter />);
    const footer = container.querySelector("footer");
    expect(footer).not.toBeNull();
    expect(footer).toBe(screen.getByRole("contentinfo"));
  });

  it("co noi dung chu, khong phai footer rong", () => {
    render(<PublicFooter />);
    expect((screen.getByRole("contentinfo").textContent ?? "").trim().length).toBeGreaterThan(0);
  });

  it("KHONG tu mo thêm <nav> (chi header co nav)", () => {
    const { container } = render(<PublicFooter />);
    expect(container.querySelectorAll("nav")).toHaveLength(0);
    expect(screen.queryAllByRole("navigation")).toHaveLength(0);
  });

  it("KHONG render <main> hay <header>", () => {
    const { container } = render(<PublicFooter />);
    expect(container.querySelectorAll("main")).toHaveLength(0);
    expect(container.querySelectorAll("header")).toHaveLength(0);
  });
});

describe("PublicFooter — AC-5 khong link gia", () => {
  it('moi <a> deu co href khong rong va khac "#"', () => {
    const { container } = render(<PublicFooter />);

    for (const anchor of Array.from(container.querySelectorAll("a"))) {
      const href = anchor.getAttribute("href");
      expect(href).toBeTruthy();
      expect(href).not.toBe("#");
    }
  });

  it("moi <a> deu co accessible name (khong link rong)", () => {
    const { container } = render(<PublicFooter />);

    for (const anchor of Array.from(container.querySelectorAll("a"))) {
      expect((anchor.textContent ?? "").trim().length).toBeGreaterThan(0);
    }
  });
});

// ---------------------------------------------------------------------------
// EV-188 (AC-8): footer dich qua <T>, van la server component.
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

const FOOTER_LINES = [
  "EventFlow — nền tảng bán vé sự kiện chịu tải đột biến.",
  "Dự án học tập. Mọi số liệu trong giao diện là dữ liệu mẫu.",
];

describe("PublicFooter — i18n (EV-188 AC-8)", () => {
  afterEach(resetLocale);

  it("mac dinh: hai dong tieng Viet giu nguyen van", () => {
    render(<PublicFooter />);
    for (const line of FOOTER_LINES) {
      expect(screen.getByText(line)).toBeInTheDocument();
    }
  });

  it("locale en: hai dong hien ban dich en", async () => {
    const m = i18n;
    await renderEn(m, <PublicFooter />);
    for (const line of FOOTER_LINES) {
      expect(await screen.findByText(enFor(m, line))).toBeInTheDocument();
      expect(screen.queryByText(line)).toBeNull();
    }
  });
});
