/**
 * Test cho FocusShell — EV-188 (plan §6 M2, m6).
 *
 * FocusShell (queue / checkout) chi co ten thuong hieu ngoai SkipLink, nen phan i18n o day chi la:
 *   - SkipLink dich theo locale (qua <T>)
 *   - <main lang="vi">: noi dung luong mua ve chua dich (no AC-8) van duoc khai bao la tieng Viet
 *     ke ca khi <html lang="en"> (WCAG 3.1.1/3.1.2).
 */
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import * as i18nProvider from "@/components/i18n/locale-provider";
import * as i18nMessages from "@/lib/i18n/messages";

import { FocusShell } from "./focus-shell";

const SKIP_LABEL = "Bỏ qua điều hướng, tới nội dung chính";
const CHILD = "Noi dung luong mua ve";

const LOCALE_KEY = "eventflow-locale";

/** Tu dien + provider gop lai cho cac helper ben duoi. */
const i18n = { ...i18nMessages, ...i18nProvider };

function renderShell() {
  return render(
    <FocusShell>
      <p>{CHILD}</p>
    </FocusShell>,
  );
}

afterEach(() => {
  window.localStorage.removeItem(LOCALE_KEY);
  document.documentElement.setAttribute("lang", "vi");
});

describe("FocusShell — cau truc", () => {
  it("co SkipLink tro toi #main va dung mot <main id=main>", () => {
    const { container } = renderShell();
    expect(screen.getByRole("link", { name: SKIP_LABEL })).toHaveAttribute("href", "#main");
    expect(container.querySelectorAll("main")).toHaveLength(1);
    expect(screen.getByRole("main")).toHaveAttribute("id", "main");
    expect(screen.getByRole("main")).toHaveTextContent(CHILD);
  });

  it("KHONG co <nav> va KHONG co link ve trang chu (luong tap trung)", () => {
    const { container } = renderShell();
    expect(container.querySelectorAll("nav")).toHaveLength(0);
    expect(container.querySelector('a[href="/"]')).toBeNull();
  });
});

describe("FocusShell — lang cua noi dung (EV-188 M2)", () => {
  it('<main> co lang="vi"', () => {
    renderShell();
    expect(screen.getByRole("main")).toHaveAttribute("lang", "vi");
  });

  it('locale en: skip link dich sang en, <main> van lang="vi"', async () => {
    const m = i18n;
    window.localStorage.setItem(LOCALE_KEY, "en");
    render(
      <m.LocaleProvider>
        <FocusShell>
          <p>{CHILD}</p>
        </FocusShell>
      </m.LocaleProvider>,
    );
    await waitFor(() => expect(document.documentElement.lang).toBe("en"));

    const skipKey = (Object.keys(m.vi) as Array<keyof typeof m.vi>).find((k) => m.vi[k] === SKIP_LABEL);
    expect(skipKey).toBeDefined();
    const english = m.en[skipKey as keyof typeof m.en];
    expect(english).not.toBe(SKIP_LABEL);
    expect(await screen.findByRole("link", { name: english })).toHaveAttribute("href", "#main");
    expect(screen.getByRole("main")).toHaveAttribute("lang", "vi");
  });
});
