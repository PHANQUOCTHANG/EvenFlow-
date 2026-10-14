/**
 * Test cho <T> — EV-188 plan §1.2: client component nho de server component (SkipLink, footer)
 * hien chuoi da dich ma khong phai thanh client component.
 *
 * Hop dong: render CHU (khong boc them phan tu), theo locale cua provider; ngoai provider -> vi;
 * `params` thay {ten}.
 */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderToString } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { LOCALE_STORAGE_KEY, en, translate, vi as viMessages, type MessageKey } from "@/lib/i18n/messages";

import { LocaleProvider, useLocale } from "./locale-provider";
import { T } from "./t";

const KEY = "skipLink.label" as MessageKey;

function jitterKey(): { key: MessageKey; param: string } {
  const key = (Object.keys(viMessages) as MessageKey[]).find((k) =>
    /^Còn \{\w+\}s \(giảm tải đột biến\)$/.test(viMessages[k]),
  );
  expect(key, "can khoa chuoi dem nguoc jitter co tham so").toBeDefined();
  const param = /\{(\w+)\}/.exec(viMessages[key as MessageKey])?.[1] as string;
  return { key: key as MessageKey, param };
}

function SwitchToEn() {
  const { setLocale } = useLocale();
  return (
    <button type="button" onClick={() => setLocale("en")}>
      to-en
    </button>
  );
}

beforeEach(() => {
  window.localStorage.removeItem(LOCALE_STORAGE_KEY);
  document.documentElement.setAttribute("lang", "vi");
});

afterEach(() => {
  window.localStorage.removeItem(LOCALE_STORAGE_KEY);
  document.documentElement.setAttribute("lang", "vi");
});

describe("T", () => {
  it("ngoai provider: render chuoi vi", () => {
    render(
      <p data-testid="host">
        <T k={KEY} />
      </p>,
    );
    expect(screen.getByTestId("host").textContent).toBe(viMessages[KEY]);
  });

  it("render chu tran, khong boc them phan tu", () => {
    render(
      <p data-testid="host">
        <T k={KEY} />
      </p>,
    );
    expect(screen.getByTestId("host").childElementCount).toBe(0);
  });

  it("render server ra vi", () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en");
    const html = renderToString(
      <LocaleProvider>
        <p>
          <T k={KEY} />
        </p>
      </LocaleProvider>,
    );
    expect(html).toContain(viMessages[KEY]);
  });

  it("trong provider voi en da luu: render chuoi en sau mount", async () => {
    window.localStorage.setItem(LOCALE_STORAGE_KEY, "en");
    render(
      <LocaleProvider>
        <p>
          <T k={KEY} />
        </p>
      </LocaleProvider>,
    );
    expect(await screen.findByText(en[KEY])).toBeInTheDocument();
  });

  it("doi locale thi chu doi ngay", async () => {
    const user = userEvent.setup();
    render(
      <LocaleProvider>
        <p data-testid="host">
          <T k={KEY} />
        </p>
        <SwitchToEn />
      </LocaleProvider>,
    );
    expect(screen.getByTestId("host").textContent).toBe(viMessages[KEY]);

    await user.click(screen.getByRole("button", { name: "to-en" }));

    expect(screen.getByTestId("host").textContent).toBe(en[KEY]);
  });

  it("params duoc thay vao chuoi", async () => {
    const { key, param } = jitterKey();
    const user = userEvent.setup();
    render(
      <LocaleProvider>
        <p data-testid="host">
          <T k={key} params={{ [param]: "3.2" }} />
        </p>
        <SwitchToEn />
      </LocaleProvider>,
    );
    expect(screen.getByTestId("host").textContent).toBe("Còn 3.2s (giảm tải đột biến)");

    await user.click(screen.getByRole("button", { name: "to-en" }));

    expect(screen.getByTestId("host").textContent).toBe(translate("en", key, { [param]: "3.2" }));
  });
});
