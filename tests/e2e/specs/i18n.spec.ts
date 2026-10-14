import type { Page } from "@playwright/test";

import { expect, test } from "../fixtures/api-mock";
import { EVENT_SLUG, installDeterministicJitter, waitForHydration } from "../fixtures/buy-flow-helpers";

/**
 * EV-188 — doi ngon ngu vi/en khong reload (spec AC-6), khong pha ISR / khong loi hydration (AC-7),
 * nut ngon ngu truy cap duoc (AC-9). Plan §1.2, §6 M1, M2, m2, m3.
 *
 * - BAT KY `console.error` / `pageerror` nao cung lam fail (m2: React production chi log loi hydration
 *   dang minified #418/#423, khong co chu "hydration"). Listener gan trong beforeEach, TRUOC goto dau
 *   tien, va giu qua reload (cung mot `page`).
 * - "Khong reload": mot bien gan tren `window` truoc khi bam van con sau khi bam, va URL khong doi.
 * - Khong chot chuoi tieng Anh cu the (implementer chon): kiem chuoi tieng Viet bien mat + `lang`.
 * - Backend gia lap EV-198 (api): request la toi API gia cung lam fail o teardown.
 */

const SKIP_VI = "Bỏ qua điều hướng, tới nội dung chính";
const NAV_VI = "Điều hướng chính";
const THEME_TOGGLE_VI = "Chuyển giao diện sáng/tối";
const FOOTER_VI = "Dự án học tập. Mọi số liệu trong giao diện là dữ liệu mẫu.";
const JOIN_VI = "Vào phòng chờ mua vé";

let consoleErrors: string[] = [];

/** Ben thu ba tren /auth/sign-in bi chan co chu dich (giong a11y.spec.ts, plan §6 M5): loi tai tai
 *  nguyen CUA CHINH cac host nay (vd. "Failed to load resource" do abort) khong tinh. Moi console.error
 *  khac van lam fail (m2). */
const BLOCKED_THIRD_PARTY = ["https://accounts.google.com/", "https://grainy-gradients.vercel.app/"];

test.beforeEach(async ({ page }) => {
  consoleErrors = [];
  page.on("console", (msg) => {
    if (msg.type() !== "error") return;
    const source = msg.location().url ?? "";
    if (BLOCKED_THIRD_PARTY.some((host) => source.startsWith(host))) return;
    consoleErrors.push(`${page.url()} :: ${msg.text()}`);
  });
  page.on("pageerror", (err) => consoleErrors.push(`pageerror ${page.url()} :: ${err.message}`));
  await installDeterministicJitter(page);
});

test.afterEach(() => {
  expect(consoleErrors, `console.error / pageerror (m2):\n${consoleErrors.join("\n")}`).toEqual([]);
});

function languageGroup(page: Page, name: "Ngôn ngữ" | "Language") {
  return page.getByRole("banner").getByRole("group", { name });
}

function viButton(page: Page) {
  return page.getByRole("banner").getByRole("button", { name: /Tiếng Việt/ });
}

function enButton(page: Page) {
  return page.getByRole("banner").getByRole("button", { name: /English/ });
}

/** Cac chuoi tieng Viet cua khung chung (AC-8) KHONG con tren trang. */
async function expectShellInEnglish(page: Page): Promise<void> {
  await expect(page.getByRole("link", { name: SKIP_VI })).toHaveCount(0);
  await expect(page.getByRole("navigation", { name: NAV_VI })).toHaveCount(0);
  await expect(page.getByRole("button", { name: THEME_TOGGLE_VI })).toHaveCount(0);
  await expect(page.getByText(FOOTER_VI, { exact: true })).toHaveCount(0);
  await expect(languageGroup(page, "Language")).toBeVisible();
  await expect(enButton(page)).toHaveAttribute("aria-pressed", "true");
  await expect(viButton(page)).toHaveAttribute("aria-pressed", "false");
}

async function expectShellInVietnamese(page: Page): Promise<void> {
  await expect(page.getByRole("link", { name: SKIP_VI })).toHaveCount(1);
  await expect(page.getByRole("button", { name: THEME_TOGGLE_VI })).toBeVisible();
  await expect(page.getByText(FOOTER_VI, { exact: true })).toBeVisible();
  await expect(languageGroup(page, "Ngôn ngữ")).toBeVisible();
  await expect(viButton(page)).toHaveAttribute("aria-pressed", "true");
  await expect(enButton(page)).toHaveAttribute("aria-pressed", "false");
}

async function storedLocale(page: Page): Promise<string | null> {
  return page.evaluate(() => window.localStorage.getItem("eventflow-locale"));
}

test.describe("AC-6 doi ngon ngu khong reload", () => {
  test("trang chu: bam English -> chu doi ngay, html lang = en, khong dieu huong, focus giu tren nut", async ({
    page,
    api: _api,
  }) => {
    await page.goto("/");
    await expect(languageGroup(page, "Ngôn ngữ")).toBeVisible();
    await waitForHydration(enButton(page));
    await expectShellInVietnamese(page);
    await expect(page.locator("html")).toHaveAttribute("lang", "vi");

    const marker = `ev188-${Date.now()}`;
    await page.evaluate((m) => {
      (window as unknown as { __ev188Marker?: string }).__ev188Marker = m;
    }, marker);
    const urlBefore = page.url();
    let mainFrameNavigations = 0;
    page.on("framenavigated", (frame) => {
      if (frame === page.mainFrame()) mainFrameNavigations += 1;
    });

    await enButton(page).click();

    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await expectShellInEnglish(page);
    await expect(enButton(page)).toBeFocused();

    // M2: noi dung chua dich van khai bao la tieng Viet.
    await expect(page.locator("main#main")).toHaveAttribute("lang", "vi");

    // Khong reload / khong dieu huong.
    expect(await page.evaluate(() => (window as unknown as { __ev188Marker?: string }).__ev188Marker)).toBe(marker);
    expect(page.url()).toBe(urlBefore);
    expect(mainFrameNavigations).toBe(0);

    expect(await storedLocale(page)).toBe("en");
  });

  test("bam Tiếng Việt -> tro ve vi, luu vi, van khong reload", async ({ page, api: _api }) => {
    await page.goto("/");
    await waitForHydration(enButton(page));
    await enButton(page).click();
    await expect(page.locator("html")).toHaveAttribute("lang", "en");

    await page.evaluate(() => {
      (window as unknown as { __ev188Marker?: string }).__ev188Marker = "van-con";
    });
    await viButton(page).click();

    await expect(page.locator("html")).toHaveAttribute("lang", "vi");
    await expectShellInVietnamese(page);
    await expect(viButton(page)).toBeFocused();
    expect(await page.evaluate(() => (window as unknown as { __ev188Marker?: string }).__ev188Marker)).toBe(
      "van-con",
    );
    expect(await storedLocale(page)).toBe("vi");
  });

  test("lua chon duoc nho qua reload", async ({ page, api: _api }) => {
    await page.goto("/");
    await waitForHydration(enButton(page));
    await enButton(page).click();
    await expect(page.locator("html")).toHaveAttribute("lang", "en");

    await page.reload();

    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await expectShellInEnglish(page);
    await expect(page.locator("main#main")).toHaveAttribute("lang", "vi");
    expect(await storedLocale(page)).toBe("en");
  });

  test(`trang su kien: bam English -> panel mua ve dich, goc panel lang = en, main van lang = vi`, async ({
    page,
    api: _api,
  }) => {
    await page.goto(`/events/${EVENT_SLUG}`);
    const joinVi = page.getByRole("button", { name: JOIN_VI });
    await expect(joinVi).toBeVisible();
    await waitForHydration(enButton(page));
    const urlBefore = page.url();

    await enButton(page).click();

    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await expect(page.getByRole("button", { name: JOIN_VI })).toHaveCount(0);
    const panel = page.locator("main#main").locator('[lang="en"]').filter({ has: page.getByRole("button") });
    await expect(panel.first()).toBeVisible();
    await expect(page.locator("main#main")).toHaveAttribute("lang", "vi");
    expect(page.url()).toBe(urlBefore);
  });
});

test.describe("AC-7 render dau la vi, locale da luu ap dung sau mount, khong loi hydration", () => {
  test.beforeEach(async ({ page }) => {
    // Locale "en" da luu TRUOC lan tai dau tien (m2).
    await page.addInitScript(() => {
      try {
        window.localStorage.setItem("eventflow-locale", "en");
      } catch {
        /* bo qua */
      }
    });
  });

  for (const path of ["/", `/events/${EVENT_SLUG}`]) {
    test(`${path}: HTML server la vi; sau mount thanh en; khong console.error`, async ({ page, request, api: _api }) => {
      // HTML server (khong chay JS) luon la vi: khong doc cookie / localStorage o server.
      const html = await (await request.get(path)).text();
      expect(html).toMatch(/<html[^>]*\blang="vi"/);
      expect(html).toContain(SKIP_VI);

      await page.goto(path);
      await expect(page.locator("html")).toHaveAttribute("lang", "en");
      await expectShellInEnglish(page);
      await expect(page.locator("main#main")).toHaveAttribute("lang", "vi");
    });
  }

  test("reload voi en da luu khong sinh loi hydration", async ({ page, api: _api }) => {
    await page.goto("/");
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await page.reload();
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
    await expectShellInEnglish(page);
  });
});

test.describe("AC-7 /events va /events/[slug] van la ISR (khong bi cookies()/headers() bien thanh dynamic)", () => {
  for (const path of ["/events", `/events/${EVENT_SLUG}`]) {
    test(`${path}: Cache-Control cua ISR (s-maxage), khong no-store`, async ({ request }) => {
      const res = await request.get(path);
      expect(res.status()).toBe(200);
      const cacheControl = res.headers()["cache-control"] ?? "";
      expect(cacheControl, `cache-control = ${cacheControl}`).toContain("s-maxage");
      expect(cacheControl).not.toContain("no-store");
    });
  }
});

// ---------------------------------------------------------------------------
// EV-188 code review (plan §7): M1, m1, m6.
// ---------------------------------------------------------------------------

async function storeEnBeforeLoad(page: Page): Promise<void> {
  await page.addInitScript(() => {
    try {
      window.localStorage.setItem("eventflow-locale", "en");
    } catch {
      /* bo qua */
    }
  });
}

/** `lang` cua to tien gan nhat co `lang` — KHONG tinh the <html> (de chung minh vung chua dich tu khai bao vi). */
async function ownLang(locator: import("@playwright/test").Locator): Promise<string | null> {
  return locator.evaluate((el) => {
    const owner = el.closest("[lang]");
    return owner && owner !== document.documentElement ? owner.getAttribute("lang") : null;
  });
}

test.describe("M1 vung chua dich nam trong [lang=vi] khi da chon en", () => {
  test.beforeEach(async ({ page }) => {
    await storeEnBeforeLoad(page);
  });

  test("/auth/sign-in: noi dung trang co lang hieu luc vi", async ({ page }) => {
    for (const pattern of BLOCKED_THIRD_PARTY) {
      await page.route(`${pattern}**`, (route) => route.abort());
    }
    await page.goto("/auth/sign-in");
    const heading = page.getByRole("heading", { level: 1, name: "Đăng nhập" });
    await expect(heading).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("lang", "en");

    expect(await ownLang(heading)).toBe("vi");
    expect(await ownLang(page.locator('button[type="submit"]'))).toBe("vi");
  });

  test("/ops/events: tieu de workspace, nav va noi dung co lang hieu luc vi", async ({ page }) => {
    await page.goto("/ops/events");
    const eventLink = page.getByRole("link", { name: /Vietnam Tech Summit 2026/ });
    await expect(eventLink).toBeVisible();
    await expect(page.locator("html")).toHaveAttribute("lang", "en");

    expect(await ownLang(eventLink)).toBe("vi");
    expect(await ownLang(page.locator("main#main"))).toBe("vi");
    expect(await ownLang(page.getByRole("navigation", { name: "Điều hướng workspace" }))).toBe("vi");
    expect(await ownLang(page.getByRole("banner").locator("p").first())).toBe("vi");
  });
});

/** Chu THUC SU hien tren man hinh trong phan tu: bo text an (display/visibility) va text sr-only
 *  (hop cat <= 1px). innerText khong dung duoc vi van tinh chu sr-only. */
async function visibleText(locator: import("@playwright/test").Locator): Promise<string> {
  return locator.evaluate((root) => {
    const parts: string[] = [];
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    for (let node = walker.nextNode(); node; node = walker.nextNode()) {
      const parent = node.parentElement;
      if (!parent || !(node.textContent ?? "").trim()) continue;
      const style = getComputedStyle(parent);
      if (style.display === "none" || style.visibility === "hidden") continue;
      const rect = parent.getBoundingClientRect();
      if (rect.width <= 1 || rect.height <= 1) continue;
      parts.push((node.textContent ?? "").trim());
    }
    return parts.join(" ").trim();
  });
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

for (const viewport of [
  { width: 320, height: 640, label: "320px (man hep)" },
  { width: 1280, height: 800, label: "1280px (desktop)" },
]) {
  test.describe(`m1 nut ngon ngu: ten truy cap chua chu hien thi — ${viewport.label}`, () => {
    test.use({ viewport: { width: viewport.width, height: viewport.height } });

    test(`Tiếng Việt va English | ${viewport.label}`, async ({ page }) => {
      await page.goto("/");
      await waitForHydration(enButton(page));

      for (const button of [viButton(page), enButton(page)]) {
        await expect(button).toBeVisible();
        const shown = await visibleText(button);
        expect(shown.length, "nut phai co chu hien thi").toBeGreaterThan(0);
        await expect(button).toHaveAccessibleName(new RegExp(escapeRegExp(shown), "i"));
      }
    });
  });
}

test.describe("m6 header cong khai o 700px van mot hang", () => {
  test("logo, nhom ngon ngu, ThemeToggle va nut menu cung mot hang; hang tren khong cao hon o desktop", async ({
    page,
  }) => {
    const banner = page.getByRole("banner");
    const topRow = banner.locator(":scope > *").first();

    await page.setViewportSize({ width: 1280, height: 800 });
    await page.goto("/");
    await waitForHydration(enButton(page));
    const desktopBox = await topRow.boundingBox();
    expect(desktopBox).not.toBeNull();

    await page.setViewportSize({ width: 700, height: 900 });
    const menuButton = banner.locator("button[aria-expanded]");
    await expect(menuButton).toBeVisible();

    const parts = [
      banner.getByRole("link", { name: "EventFlow" }),
      languageGroup(page, "Ngôn ngữ"),
      banner.getByRole("button", { name: THEME_TOGGLE_VI }),
      menuButton,
    ];
    const centers: number[] = [];
    for (const part of parts) {
      const box = await part.boundingBox();
      expect(box).not.toBeNull();
      centers.push((box as { y: number; height: number }).y + (box as { height: number }).height / 2);
    }
    const spread = Math.max(...centers) - Math.min(...centers);
    expect(spread, `tam doc cac phan tu header lech ${spread}px (xuong hang?): ${centers.join(", ")}`).toBeLessThanOrEqual(4);

    const narrowBox = await topRow.boundingBox();
    expect(narrowBox).not.toBeNull();
    expect((narrowBox as { height: number }).height).toBeLessThanOrEqual((desktopBox as { height: number }).height + 1);
  });
});

// ---------------------------------------------------------------------------
// EV-188 challenge F1, F3 (challenge.md).
// ---------------------------------------------------------------------------

/** Su kien SCHEDULED trong du lieu mau (lib/event-service.ts), mo ban 2026-10-15T10:00:00+07:00. */
const SCHEDULED_SLUG = "evenflow-grand-concert-2026";
const SCHEDULED_SALE_START_MS = Date.parse("2026-10-15T10:00:00+07:00");

/** Gio server gia = 2 ngay truoc moc mo ban: trang thai "chua toi gio" khong phu thuoc ngay chay that
 *  (khong co no sau 2026-10-15). Chi chan `/api/time` cua web, khong dong vao gio trinh duyet. */
async function pinServerTimeBeforeSale(page: Page): Promise<void> {
  await page.route("**/api/time", (route) => {
    const serverTime = SCHEDULED_SALE_START_MS - 2 * 24 * 60 * 60 * 1000;
    return route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify({ server_time: serverTime, rfc3339: new Date(serverTime).toISOString() }),
    });
  });
}

test.describe("challenge F1: panel SCHEDULED voi en — chu tieng Viet cua dong ho nam trong lang=vi", () => {
  test.beforeEach(async ({ page }) => {
    await storeEnBeforeLoad(page);
    await pinServerTimeBeforeSale(page);
  });

  test(`/events/${SCHEDULED_SLUG}: moi text node tieng Viet trong panel co [lang] gan nhat = vi`, async ({
    page,
    api: _api,
  }) => {
    await page.goto(`/events/${SCHEDULED_SLUG}`);
    await expect(page.locator("html")).toHaveAttribute("lang", "en");

    // Goc EventActionPanel (lang = locale) chua dong ho dem nguoc.
    const panel = page
      .locator("main#main")
      .locator('[lang="en"]')
      .filter({ hasText: /\d+:\d{2}:\d{2}/ })
      .first();
    await expect(panel).toBeVisible();

    const offenders = await panel.evaluate((root) => {
      const vietnamese = /[ăâđêôơưàáảãạằắẳẵặầấẩẫậèéẻẽẹềếểễệìíỉĩịòóỏõọồốổỗộờớởỡợùúủũụừứửữựỳýỷỹỵ]/i;
      const bad: string[] = [];
      const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
      for (let node = walker.nextNode(); node; node = walker.nextNode()) {
        const text = (node.textContent ?? "").trim();
        const parent = node.parentElement;
        if (!text || !parent || !vietnamese.test(text)) continue;
        if (getComputedStyle(parent).display === "none") continue;
        const lang = parent.closest("[lang]")?.getAttribute("lang");
        if (lang !== "vi") bad.push(`${lang ?? "(none)"} :: ${text}`);
      }
      return bad;
    });
    expect(offenders, "chu tieng Viet bi khai bao la ngon ngu khac (WCAG 3.1.2)").toEqual([]);
  });
});

for (const locale of ["vi", "en"] as const) {
  test.describe(`challenge F3: /auth/sign-up va /auth/forgot-password khong loi console — locale ${locale}`, () => {
    test.beforeEach(async ({ page }) => {
      if (locale === "en") await storeEnBeforeLoad(page);
      for (const pattern of BLOCKED_THIRD_PARTY) {
        await page.route(`${pattern}**`, (route) => route.abort());
      }
    });

    for (const path of ["/auth/sign-up", "/auth/forgot-password"]) {
      test(`${path} | ${locale}`, async ({ page }) => {
        await page.goto(path);
        const submit = page.locator('button[type="submit"]').first();
        await expect(submit).toBeVisible();
        await waitForHydration(submit);
        await expect(page.locator("html")).toHaveAttribute("lang", locale);
        // console.error / pageerror (vd. "<path> attribute d: Expected arc flag") lam fail o afterEach.
      });
    }
  });
}
