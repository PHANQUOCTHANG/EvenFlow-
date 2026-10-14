import type { Locator, Page } from "@playwright/test";

import { expect, test } from "../fixtures/api-mock";
import { expectNoSeriousA11yViolations } from "../fixtures/axe";
import {
  EVENT_ID,
  EVENT_SLUG,
  P,
  TICKET_CODES,
  admitToCheckout,
  clickJoinFromEvent,
  continueToPayment,
  fillRecipientAndPay,
  holdSelectedTickets,
  installDeterministicJitter,
  joinQueueFromEvent,
  mockHoldCreated,
  mockPaidOrder,
  mockPaymentRedirect,
  mockQueue,
  selectAllAccessTimesTwo,
  waitForHydration,
} from "../fixtures/buy-flow-helpers";

/**
 * EV-188 — quet axe-core (WCAG 2.0/2.1 A + AA) moi man hinh Buyer (spec AC-1, AC-2, AC-4).
 * Plan §1.4 + §6: M3 (reflow 320px), M4 (mot luong day du moi theme), M5 (sign-in chan ben thu ba),
 * m9 (cho on dinh truoc khi quet), m10 (quet them locale en).
 *
 * Theme dat TRUOC khi tai trang: `emulateMedia({ colorScheme })` + `localStorage["eventflow-theme"]`
 * qua addInitScript (ThemeScript uu tien localStorage). Khong lat theme giua trang (transition-colors
 * lam contrast that thuong). `reducedMotion: "reduce"` tat ef-spin / ef-pulse.
 *
 * Moi lan quet dung expect.soft (fixtures/axe.ts): mot luong chay het va bao du moi trang vi pham.
 * Backend gia lap bang fixture EV-198 (api) — request la / pageerror cung lam fail o teardown.
 */

type Theme = "light" | "dark";
type Locale = "vi" | "en";

const THEMES: readonly Theme[] = ["light", "dark"];

/** Ben thu ba tren /auth/sign-in (M5): script/iframe Google Identity va anh nen grainy-gradients.
 *  Phu thuoc mang + client id rong khi build E2E -> chan de ket qua on dinh (ghi ly do trong traceability). */
const THIRD_PARTY_SIGN_IN = ["https://accounts.google.com/**", "https://grainy-gradients.vercel.app/**"];

async function applyThemeAndLocale(page: Page, theme: Theme, locale?: Locale): Promise<void> {
  await page.emulateMedia({ colorScheme: theme, reducedMotion: "reduce" });
  await page.addInitScript(
    ({ t, l }) => {
      try {
        window.localStorage.setItem("eventflow-theme", t);
        if (l) window.localStorage.setItem("eventflow-locale", l);
      } catch {
        /* about:blank / storage bi chan: bo qua */
      }
    },
    { t: theme, l: locale ?? null },
  );
}

interface PrepareOptions {
  theme: Theme;
  locale?: Locale;
  /** Phan tu de cho hydrate; mac dinh `#main`. */
  hydrateTarget?: Locator;
  /** Trang co ThemeToggle trong header (PublicShell): cho aria-pressed khop theme (m9). */
  hasThemeToggle?: boolean;
}

/** m9: cho hydrate, cho theme da ap dung (va ThemeToggle da dong bo), dua chuot ra goc, cho font. */
async function prepareForScan(page: Page, opts: PrepareOptions): Promise<void> {
  // challenge N2: sau client navigation (router.push) Next 15 gan <title> theo stream metadata; quet
  // truoc luc do thi axe bao `document-title` (serious) gia. Cho title khac rong truoc khi quet.
  await expect(page).toHaveTitle(/\S/);
  await expect(page.locator("html")).toHaveAttribute("data-theme", opts.theme);
  await waitForHydration(opts.hydrateTarget ?? page.locator("#main"));
  if (opts.hasThemeToggle) {
    const icon = opts.theme === "dark" ? "☾" : "☀";
    const themeToggle = page.getByRole("banner").locator("button[aria-pressed]").filter({ hasText: icon });
    await expect(themeToggle).toHaveAttribute("aria-pressed", String(opts.theme === "dark"));
  }
  if (opts.locale === "en") {
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
  }
  await page.mouse.move(0, 0);
  await page.evaluate(() => document.fonts.ready.then(() => undefined));
}

async function expectNoHorizontalScroll(page: Page, label: string): Promise<void> {
  const { scrollWidth, innerWidth } = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }));
  expect
    .soft(scrollWidth, `[${label}] tran ngang (WCAG 1.4.10): scrollWidth ${scrollWidth} > innerWidth ${innerWidth}`)
    .toBeLessThanOrEqual(innerWidth);
}

test.beforeEach(async ({ page }) => {
  // Jitter tat dinh = 0 (giong buy-flow).
  await installDeterministicJitter(page);
});

for (const theme of THEMES) {
  test.describe(`AC-1 quet axe — theme ${theme}`, () => {
    test.beforeEach(async ({ page }) => {
      await applyThemeAndLocale(page, theme);
    });

    test(`trang cong khai: /, /events, /events/${EVENT_SLUG} | ${theme}`, async ({ page, api: _api }) => {
      await page.goto("/");
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      await prepareForScan(page, { theme, hasThemeToggle: true });
      await expectNoSeriousA11yViolations(page, `/ | ${theme}`);

      await page.goto("/events");
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      await prepareForScan(page, { theme, hasThemeToggle: true });
      await expectNoSeriousA11yViolations(page, `/events | ${theme}`);

      await page.goto(`/events/${EVENT_SLUG}`);
      const joinButton = page.getByRole("button", { name: "Vào phòng chờ mua vé" });
      await expect(joinButton).toBeVisible();
      await prepareForScan(page, { theme, hasThemeToggle: true, hydrateTarget: joinButton });
      await expectNoSeriousA11yViolations(page, `/events/${EVENT_SLUG} | ${theme}`);
    });

    test(`luong mua ve: QUEUED -> checkout truoc/sau giu -> thanh toan -> ket qua | ${theme}`, async ({
      page,
      api,
    }) => {
      const admit = mockQueue(api);
      mockHoldCreated(api);
      mockPaymentRedirect(api);
      mockPaidOrder(api);

      await joinQueueFromEvent(page, api);
      await prepareForScan(page, { theme });
      await expectNoSeriousA11yViolations(page, `/waiting QUEUED | ${theme}`);

      await admitToCheckout(page, admit);
      await selectAllAccessTimesTwo(page);
      await prepareForScan(page, { theme });
      await expectNoSeriousA11yViolations(page, `/checkout truoc giu ve | ${theme}`);

      await holdSelectedTickets(page);
      await prepareForScan(page, { theme });
      await expectNoSeriousA11yViolations(page, `/checkout sau giu ve | ${theme}`);

      await continueToPayment(page);
      await expect(page.getByRole("heading", { level: 1, name: "Thanh toán đơn hàng" })).toBeVisible();
      await prepareForScan(page, { theme, hydrateTarget: page.locator("#payment-form") });
      await expectNoSeriousA11yViolations(page, `/checkout/payment | ${theme}`);

      await fillRecipientAndPay(page);
      await expect(page.getByText("Đã thanh toán thành công")).toBeVisible();
      const tickets = page.getByRole("region", { name: "Danh sách vé điện tử" });
      await expect(tickets.getByText(TICKET_CODES[0], { exact: true }).first()).toBeVisible();
      await prepareForScan(page, { theme });
      await expectNoSeriousA11yViolations(page, `/checkout/result da thanh toan | ${theme}`);
    });

    test(`phong cho khi join bi 401 | ${theme}`, async ({ page, api }) => {
      api.on("POST", P.join, { status: 401, json: { title: "Unauthorized", detail: "identity required" } });
      api.forbid("GET", P.status, "khong duoc poll status khi join that bai");
      api.forbid("GET", P.stream, "khong duoc mo stream khi join that bai");

      await clickJoinFromEvent(page);
      await expect(page.getByRole("alert").filter({ hasText: "Bạn cần đăng nhập và xác thực OTP" })).toBeVisible();
      expect(new URL(page.url()).pathname).toBe(`/waiting/${EVENT_ID}`);
      await prepareForScan(page, { theme });
      await expectNoSeriousA11yViolations(page, `/waiting join 401 | ${theme}`);
    });

    test(`/auth/sign-in (chan Google + grainy-gradients) | ${theme}`, async ({ page, api: _api }) => {
      for (const pattern of THIRD_PARTY_SIGN_IN) {
        await page.route(pattern, (route) => route.abort());
      }
      await page.goto("/auth/sign-in");
      await expect(page.getByRole("heading", { level: 1, name: "Đăng nhập" })).toBeVisible();
      // Trang ep data-theme="light" tren wrapper (sign-in/page.tsx): lan quet dark gan nhu trung light.
      await prepareForScan(page, { theme, hydrateTarget: page.locator('button[type="submit"]') });
      await expectNoSeriousA11yViolations(page, `/auth/sign-in | ${theme}`);
    });

    test(`locale en (m10): /, /events/${EVENT_SLUG} | ${theme}`, async ({ page, api: _api }) => {
      await page.addInitScript(() => {
        try {
          window.localStorage.setItem("eventflow-locale", "en");
        } catch {
          /* bo qua */
        }
      });

      await page.goto("/");
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      await prepareForScan(page, { theme, locale: "en", hasThemeToggle: true });
      // plan §7 m2: chu da sang en truoc khi quet (khong quet ban vi chua kip doi).
      await expect(page.getByRole("banner").getByRole("group", { name: "Language" })).toBeVisible();
      await expectNoSeriousA11yViolations(page, `/ en | ${theme}`);

      await page.goto(`/events/${EVENT_SLUG}`);
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      await prepareForScan(page, { theme, locale: "en", hasThemeToggle: true });
      // plan §7 m2: panel mua ve da dich (nut en hien, nut vi bien mat) truoc khi quet.
      await expect(page.getByRole("banner").getByRole("group", { name: "Language" })).toBeVisible();
      await expect(page.getByRole("button", { name: "Join the waiting room" })).toBeVisible();
      await expect(page.getByRole("button", { name: "Vào phòng chờ mua vé" })).toHaveCount(0);
      await expectNoSeriousA11yViolations(page, `/events/${EVENT_SLUG} en | ${theme}`);
    });
  });

  test.describe(`M3 reflow 320px — theme ${theme}`, () => {
    test.use({ viewport: { width: 320, height: 640 } });

    test.beforeEach(async ({ page }) => {
      await applyThemeAndLocale(page, theme);
    });

    test(`/ menu dong va mo, khong tran ngang | ${theme}`, async ({ page, api: _api }) => {
      await page.goto("/");
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      await prepareForScan(page, { theme, hasThemeToggle: true });
      await expectNoHorizontalScroll(page, `/ 320px menu dong | ${theme}`);
      await expectNoSeriousA11yViolations(page, `/ 320px menu dong | ${theme}`);

      const menuButton = page.getByRole("banner").locator("button[aria-expanded]");
      await expect(menuButton).toBeVisible();
      await waitForHydration(menuButton);
      await menuButton.click();
      await expect(menuButton).toHaveAttribute("aria-expanded", "true");
      await expect(page.getByRole("navigation").first()).toBeVisible();
      await page.mouse.move(0, 0);
      await expectNoHorizontalScroll(page, `/ 320px menu mo | ${theme}`);
      await expectNoSeriousA11yViolations(page, `/ 320px menu mo | ${theme}`);
    });
  });
}
