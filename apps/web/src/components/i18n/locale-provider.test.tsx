/**
 * Test cho LocaleProvider / useLocale / useT — EV-188 AC-5, AC-6, AC-7 (plan §1.2 + §6 m3, m4).
 *
 * Hop dong:
 *   - Render dau (server VA client) luon `vi` — khong doc localStorage luc render (AC-7, khong lech hydrate).
 *   - Sau mount: doc localStorage["eventflow-locale"] (try/catch, gia tri la bo qua), ap dung va dat
 *     `document.documentElement.lang`, KHONG ghi lai localStorage.
 *   - setLocale: doi state, ghi localStorage (nuot loi), dat `<html lang>`. Khong dieu huong / reload.
 *     Provider khong dung useRouter / useSearchParams (test nay KHONG mock next/navigation: neu provider
 *     goi useRouter ngoai app router thi render nem loi).
 *   - Ngoai provider: useT dich theo `vi`, setLocale la no-op.
 *   - Ham tra ve tu useT memo theo locale.
 */
import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect } from "react";
import { hydrateRoot } from "react-dom/client";
import { renderToString } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { LOCALE_STORAGE_KEY, en, translate, vi as viMessages, type Locale, type MessageKey } from "@/lib/i18n/messages";

import { LocaleProvider, useLocale, useT } from "./locale-provider";

const KEY = "skipLink.label" as MessageKey;
const VI_TEXT = viMessages[KEY];
const EN_TEXT = en[KEY];

/** Hien locale + mot chuoi da dich + nut doi locale. */
function Probe() {
  const { locale, setLocale } = useLocale();
  const t = useT();
  return (
    <div>
      <span data-testid="locale">{locale}</span>
      <span data-testid="text">{t(KEY)}</span>
      <button type="button" onClick={() => setLocale("en")}>
        to-en
      </button>
      <button type="button" onClick={() => setLocale("vi")}>
        to-vi
      </button>
    </div>
  );
}

function renderWithProvider() {
  return render(
    <LocaleProvider>
      <Probe />
    </LocaleProvider>,
  );
}

function currentLocale(): string {
  return screen.getByTestId("locale").textContent ?? "";
}

function currentText(): string {
  return screen.getByTestId("text").textContent ?? "";
}

/** localStorage bi chan hoan toan (private mode) — cung cach theme-script.test.tsx. */
function blockStorage(): void {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("SecurityError: storage bi chan");
  });
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("SecurityError: storage bi chan");
  });
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

describe("tien de cua test", () => {
  it("chuoi mau co ban dich en khac vi", () => {
    expect(VI_TEXT).toBe("Bỏ qua điều hướng, tới nội dung chính");
    expect(EN_TEXT).not.toBe(VI_TEXT);
  });
});

describe("LocaleProvider — render dau luon vi (AC-7)", () => {
  it("chua luu gi: vi, html lang giu vi", () => {
    renderWithProvider();
    expect(currentLocale()).toBe("vi");
    expect(currentText()).toBe(VI_TEXT);
    expect(document.documentElement.lang).toBe("vi");
  });

  it("render tren server (renderToString) ra vi du localStorage dang la en", () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en");
    const html = renderToString(
      <LocaleProvider>
        <Probe />
      </LocaleProvider>,
    );
    expect(html).toContain(VI_TEXT);
    expect(html).not.toContain(EN_TEXT);
    expect(html).toMatch(/data-testid="locale">vi</);
  });

  it("render server KHONG doc localStorage", () => {
    const getItem = vi.spyOn(Storage.prototype, "getItem");
    renderToString(
      <LocaleProvider>
        <Probe />
      </LocaleProvider>,
    );
    expect(getItem).not.toHaveBeenCalledWith(LOCALE_STORAGE_KEY);
  });

  it("hydrate HTML server voi en da luu: khong loi hydration, sau mount chuyen sang en", async () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en");
    const tree = (
      <LocaleProvider>
        <Probe />
      </LocaleProvider>
    );
    const container = document.createElement("div");
    container.innerHTML = renderToString(tree);
    document.body.appendChild(container);
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});

    let root: ReturnType<typeof hydrateRoot> | undefined;
    try {
      await act(async () => {
        root = hydrateRoot(container, tree);
      });

      expect(consoleError).not.toHaveBeenCalled();
      expect(container.querySelector('[data-testid="locale"]')?.textContent).toBe("en");
      expect(container.querySelector('[data-testid="text"]')?.textContent).toBe(EN_TEXT);
    } finally {
      act(() => root?.unmount());
      container.remove();
    }
  });

  it("lan render DAU TIEN cua client (truoc effect) la vi", () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en");
    const seen: Locale[] = [];
    function Recorder() {
      const { locale } = useLocale();
      seen.push(locale);
      return null;
    }
    render(
      <LocaleProvider>
        <Recorder />
      </LocaleProvider>,
    );
    expect(seen[0]).toBe("vi");
    expect(seen[seen.length - 1]).toBe("en");
  });
});

describe("LocaleProvider — khoi phuc locale da luu sau mount (AC-6, m3)", () => {
  it('da luu "en": chu doi sang en va html lang = "en"', async () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en");
    renderWithProvider();
    expect(await screen.findByText(EN_TEXT)).toBeInTheDocument();
    expect(currentLocale()).toBe("en");
    expect(document.documentElement.lang).toBe("en");
  });

  it("khoi phuc KHONG ghi lai localStorage", async () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en");
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    renderWithProvider();
    await screen.findByText(EN_TEXT);
    expect(setItem).not.toHaveBeenCalled();
  });

  it('da luu "vi": giu vi, html lang = "vi"', () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "vi");
    document.documentElement.setAttribute("lang", "vi");
    renderWithProvider();
    expect(currentLocale()).toBe("vi");
    expect(document.documentElement.lang).toBe("vi");
  });

  it.each([["fr"], ["EN"], [""], ["null"], ['"en"']])("gia tri luu la %j bi bo qua: van vi", (stored) => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, stored);
    renderWithProvider();
    expect(currentLocale()).toBe("vi");
    expect(currentText()).toBe(VI_TEXT);
    expect(document.documentElement.lang).toBe("vi");
  });
});

describe("LocaleProvider — setLocale (AC-6)", () => {
  it("doi chu, ghi localStorage, dat html lang", async () => {
    const user = userEvent.setup();
    renderWithProvider();

    await user.click(screen.getByRole("button", { name: "to-en" }));

    expect(currentLocale()).toBe("en");
    expect(currentText()).toBe(EN_TEXT);
    expect(window.localStorage.getItem(LOCALE_STORAGE_KEY)).toBe("en");
    expect(document.documentElement.lang).toBe("en");

    await user.click(screen.getByRole("button", { name: "to-vi" }));

    expect(currentLocale()).toBe("vi");
    expect(currentText()).toBe(VI_TEXT);
    expect(window.localStorage.getItem(LOCALE_STORAGE_KEY)).toBe("vi");
    expect(document.documentElement.lang).toBe("vi");
  });

  it("khong dieu huong, khong reload: URL va history giu nguyen", async () => {
    const user = userEvent.setup();
    const hrefBefore = window.location.href;
    const historyBefore = window.history.length;
    const pushState = vi.spyOn(window.history, "pushState");
    const replaceState = vi.spyOn(window.history, "replaceState");
    renderWithProvider();

    await user.click(screen.getByRole("button", { name: "to-en" }));

    expect(window.location.href).toBe(hrefBefore);
    expect(window.history.length).toBe(historyBefore);
    expect(pushState).not.toHaveBeenCalled();
    expect(replaceState).not.toHaveBeenCalled();
  });

  it("khong remount con: state cua component con giu nguyen sau khi doi locale", async () => {
    const user = userEvent.setup();
    const mounts = vi.fn();
    function Child() {
      useEffect(() => {
        mounts();
      }, []);
      return null;
    }
    render(
      <LocaleProvider>
        <Child />
        <Probe />
      </LocaleProvider>,
    );
    expect(mounts).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: "to-en" }));

    expect(currentLocale()).toBe("en");
    expect(mounts).toHaveBeenCalledTimes(1);
  });
});

describe("LocaleProvider — localStorage bi chan (m4)", () => {
  it("render khong nem loi, ra vi", () => {
    blockStorage();
    expect(() => renderWithProvider()).not.toThrow();
    expect(currentLocale()).toBe("vi");
    expect(currentText()).toBe(VI_TEXT);
  });

  it("setLocale van doi chu va html lang du khong ghi duoc storage", async () => {
    blockStorage();
    const user = userEvent.setup();
    renderWithProvider();

    await user.click(screen.getByRole("button", { name: "to-en" }));

    expect(currentLocale()).toBe("en");
    expect(currentText()).toBe(EN_TEXT);
    expect(document.documentElement.lang).toBe("en");
  });
});

describe("useT / useLocale ngoai provider (m3)", () => {
  it("useT dich theo vi, khong nem loi", () => {
    render(<Probe />);
    expect(currentText()).toBe(VI_TEXT);
    expect(currentLocale()).toBe("vi");
  });

  it("useT ngoai provider van thay tham so", () => {
    const keyWithParam = (Object.keys(viMessages) as MessageKey[]).find((k) => /\{\w+\}/.test(viMessages[k]));
    expect(keyWithParam).toBeDefined();
    const k = keyWithParam as MessageKey;
    const name = /\{(\w+)\}/.exec(viMessages[k])?.[1] as string;
    function ParamProbe() {
      const t = useT();
      return <span data-testid="param">{t(k, { [name]: 7 })}</span>;
    }
    render(<ParamProbe />);
    expect(screen.getByTestId("param").textContent).toBe(translate("vi", k, { [name]: 7 }));
  });

  it("setLocale ngoai provider la no-op: khong nem loi, khong doi lang, khong ghi storage", async () => {
    const user = userEvent.setup();
    const setItem = vi.spyOn(Storage.prototype, "setItem");
    render(<Probe />);

    await user.click(screen.getByRole("button", { name: "to-en" }));

    expect(currentLocale()).toBe("vi");
    expect(currentText()).toBe(VI_TEXT);
    expect(document.documentElement.lang).toBe("vi");
    expect(setItem).not.toHaveBeenCalled();
    expect(window.localStorage.getItem(LOCALE_STORAGE_KEY)).toBeNull();
  });
});

describe("useT — memo theo locale (m3)", () => {
  it("cung mot locale: ham t giu nguyen qua cac lan re-render; doi locale: ham moi dich theo locale moi", async () => {
    const user = userEvent.setup();
    const seen: Array<(key: MessageKey) => string> = [];
    function Capture({ tick }: { tick: number }) {
      const t = useT();
      seen.push(t);
      return <span data-testid="tick">{tick}</span>;
    }

    const { rerender } = render(
      <LocaleProvider>
        <Capture tick={1} />
        <Probe />
      </LocaleProvider>,
    );
    rerender(
      <LocaleProvider>
        <Capture tick={2} />
        <Probe />
      </LocaleProvider>,
    );
    const viFns = new Set(seen);
    expect(viFns.size).toBe(1);
    expect(seen[0](KEY)).toBe(VI_TEXT);

    await user.click(screen.getByRole("button", { name: "to-en" }));
    const enFn = seen[seen.length - 1];
    expect(enFn).not.toBe(seen[0]);
    expect(enFn(KEY)).toBe(EN_TEXT);

    rerender(
      <LocaleProvider>
        <Capture tick={3} />
        <Probe />
      </LocaleProvider>,
    );
    expect(seen[seen.length - 1]).toBe(enFn);
  });
});
