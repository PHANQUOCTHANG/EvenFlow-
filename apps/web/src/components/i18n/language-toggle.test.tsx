/**
 * Test cho LanguageToggle — EV-188 AC-6, AC-9 (plan §6 M1).
 *
 * Hop dong da chot sau plan review:
 *   - `<div role="group" aria-label={t("language.group")}>` ("Ngôn ngữ" / "Language")
 *   - hai `<button type="button" aria-pressed>`: "Tiếng Việt" (`lang="vi"`) va "English" (`lang="en"`)
 *   - KHONG `aria-label` tren nut: ten truy cap lay tu chu hien thi (WCAG 2.5.3 Label in Name)
 *   - bam nut dang chon: khong lam gi; focus o lai nut vua bam (khong remount)
 * setLocale ngoai provider la no-op nen moi test boc LocaleProvider.
 */
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { LOCALE_STORAGE_KEY } from "@/lib/i18n/messages";

import { LanguageToggle } from "./language-toggle";
import { LocaleProvider } from "./locale-provider";

const VI_LABEL = "Tiếng Việt";
const EN_LABEL = "English";
/** Ten truy cap chua ten ngon ngu (plan §7 m1: o man hep co them nhan ngan "VI"/"EN", vd. "English EN"). */
const VI_NAME = /Tiếng Việt/;
const EN_NAME = /English/;

function renderToggle() {
  return render(
    <LocaleProvider>
      <LanguageToggle />
    </LocaleProvider>,
  );
}

function viButton(): HTMLElement {
  return screen.getByRole("button", { name: VI_NAME });
}

function enButton(): HTMLElement {
  return screen.getByRole("button", { name: EN_NAME });
}

/** `lang` dat tren chinh nut, hoac tren phan tu chua chu ben trong nut. */
function langOf(btn: HTMLElement): string | null {
  return btn.getAttribute("lang") ?? btn.querySelector("[lang]")?.getAttribute("lang") ?? null;
}

beforeEach(() => {
  window.localStorage.removeItem(LOCALE_STORAGE_KEY);
  document.documentElement.setAttribute("lang", "vi");
});

afterEach(() => {
  vi.restoreAllMocks();
  window.localStorage.removeItem(LOCALE_STORAGE_KEY);
  document.documentElement.setAttribute("lang", "vi");
});

describe("LanguageToggle — cau truc (AC-9)", () => {
  it('nhom co ten "Ngôn ngữ" chua dung hai nut', () => {
    renderToggle();
    const group = screen.getByRole("group", { name: "Ngôn ngữ" });
    const buttons = within(group).getAllByRole("button");
    expect(buttons).toHaveLength(2);
    expect(buttons).toContain(viButton());
    expect(buttons).toContain(enButton());
  });

  it('hai nut la <button type="button">', () => {
    renderToggle();
    for (const btn of [viButton(), enButton()]) {
      expect(btn.tagName).toBe("BUTTON");
      expect(btn).toHaveAttribute("type", "button");
    }
  });

  it('chu hien thi co lang dung: "Tiếng Việt" lang="vi", "English" lang="en"', () => {
    renderToggle();
    expect(langOf(viButton())).toBe("vi");
    expect(langOf(enButton())).toBe("en");
  });

  it("lang cua chu tren nut khong doi khi doi locale", async () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en");
    renderToggle();
    await screen.findByRole("group", { name: "Language" });
    expect(langOf(viButton())).toBe("vi");
    expect(langOf(enButton())).toBe("en");
  });

  it("KHONG co aria-label / aria-labelledby tren nut (ten lay tu chu hien thi)", () => {
    renderToggle();
    for (const btn of [viButton(), enButton()]) {
      expect(btn).not.toHaveAttribute("aria-label");
      expect(btn).not.toHaveAttribute("aria-labelledby");
    }
  });

  it("ten truy cap chua chu hien thi (WCAG 2.5.3)", () => {
    renderToggle();
    for (const [btn, label] of [
      [viButton(), VI_LABEL],
      [enButton(), EN_LABEL],
    ] as const) {
      const visible = (btn.textContent ?? "").replace(/\s+/g, " ").trim();
      expect(visible).toContain(label);
      expect(btn).toHaveAccessibleName(expect.stringContaining(label));
    }
  });

  it("mac dinh: Tiếng Việt aria-pressed=true, English aria-pressed=false", () => {
    renderToggle();
    expect(viButton()).toHaveAttribute("aria-pressed", "true");
    expect(enButton()).toHaveAttribute("aria-pressed", "false");
  });

  it('da luu en: nhom ten "Language", English duoc chon', async () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en");
    renderToggle();
    expect(await screen.findByRole("group", { name: "Language" })).toBeInTheDocument();
    expect(enButton()).toHaveAttribute("aria-pressed", "true");
    expect(viButton()).toHaveAttribute("aria-pressed", "false");
  });

  it("chu tren nut KHONG doi theo locale (ten rieng cua ngon ngu)", async () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en");
    renderToggle();
    await screen.findByRole("group", { name: "Language" });
    expect(viButton()).toBeInTheDocument();
    expect(enButton()).toBeInTheDocument();
  });
});

describe("LanguageToggle — hanh vi (AC-6)", () => {
  it("bam English: doi html lang, aria-pressed, ten nhom, luu storage", async () => {
    const user = userEvent.setup();
    renderToggle();

    await user.click(enButton());

    expect(document.documentElement.lang).toBe("en");
    expect(enButton()).toHaveAttribute("aria-pressed", "true");
    expect(viButton()).toHaveAttribute("aria-pressed", "false");
    expect(screen.getByRole("group", { name: "Language" })).toBeInTheDocument();
    expect(window.localStorage.getItem(LOCALE_STORAGE_KEY)).toBe("en");
  });

  it("focus o lai dung nut vua bam (cung phan tu DOM, khong remount)", async () => {
    const user = userEvent.setup();
    renderToggle();
    const before = enButton();

    await user.click(before);

    expect(enButton()).toBe(before);
    expect(before).toHaveFocus();
  });

  it("bam lai Tiếng Việt: tro ve vi", async () => {
    const user = userEvent.setup();
    renderToggle();

    await user.click(enButton());
    await user.click(viButton());

    expect(document.documentElement.lang).toBe("vi");
    expect(viButton()).toHaveAttribute("aria-pressed", "true");
    expect(viButton()).toHaveFocus();
    expect(window.localStorage.getItem(LOCALE_STORAGE_KEY)).toBe("vi");
  });

  it("bam nut dang chon: khong lam gi (khong ghi storage, khong doi lang)", async () => {
    const user = userEvent.setup();
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    renderToggle();

    await user.click(viButton());

    expect(setItem).not.toHaveBeenCalled();
    expect(document.documentElement.lang).toBe("vi");
    expect(viButton()).toHaveAttribute("aria-pressed", "true");
    expect(enButton()).toHaveAttribute("aria-pressed", "false");
  });

  it("dung duoc bang ban phim: Tab toi nut, Enter / Space de chon", async () => {
    const user = userEvent.setup();
    renderToggle();

    await user.tab();
    await user.tab();
    expect(enButton()).toHaveFocus();
    await user.keyboard("{Enter}");
    expect(document.documentElement.lang).toBe("en");

    await user.tab({ shift: true });
    expect(viButton()).toHaveFocus();
    await user.keyboard(" ");
    expect(document.documentElement.lang).toBe("vi");
  });

  it("khong dieu huong: URL giu nguyen", async () => {
    const user = userEvent.setup();
    const hrefBefore = window.location.href;
    renderToggle();

    await user.click(enButton());

    expect(window.location.href).toBe(hrefBefore);
  });
});

// ---------------------------------------------------------------------------
// EV-188 code review m1 (plan §7): nhan ngan "VI"/"EN" (hien o man hep) KHONG duoc aria-hidden —
// neu an thi o man hep chu hien thi ("EN") khong nam trong ten truy cap, vi pham 2.5.3. jsdom khong
// ap CSS nen o day moi nhan deu "hien": ten phai chua CA ten day du lan nhan ngan. Kiem theo
// kich thuoc that (320px / desktop) nam o tests/e2e/specs/i18n.spec.ts.
// ---------------------------------------------------------------------------
describe("LanguageToggle — nhan ngan VI/EN (EV-188 m1)", () => {
  const CASES = [
    { name: VI_NAME, label: VI_LABEL, short: "VI" },
    { name: EN_NAME, label: EN_LABEL, short: "EN" },
  ] as const;

  it.each(CASES)("nut $label co nhan ngan $short khong bi aria-hidden", ({ name, short }) => {
    renderToggle();
    const btn = screen.getByRole("button", { name });
    const shortEl = within(btn).getByText(short, { exact: true });
    expect(shortEl.closest('[aria-hidden="true"]')).toBeNull();
  });

  it.each(CASES)("ten truy cap cua nut $label chua ca ten day du va nhan ngan $short", ({ name, label, short }) => {
    renderToggle();
    const btn = screen.getByRole("button", { name });
    expect(btn).toHaveAccessibleName(expect.stringContaining(label));
    expect(btn).toHaveAccessibleName(expect.stringContaining(short));
  });
});
