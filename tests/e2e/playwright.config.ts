import { defineConfig, devices } from "@playwright/test";

/**
 * E2E (issue 1909, EV-198): xep hang -> duoc goi -> giu ghe -> thanh toan -> nhan ve.
 *
 * Web that (Next build production) o BASE_URL; backend gia lap bang page.route o E2E_API_BASE
 * (fixtures/api-mock.ts). Web PHAI duoc build voi NEXT_PUBLIC_API_BASE = E2E_API_BASE
 * (mac dinh http://localhost:8080, giong compose cho nightly).
 *
 * retries: 0 (AC-8) — test flaky phai lo ra, khong duoc che bang retry.
 */
export default defineConfig({
  testDir: "./specs",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [["list"], ["html", { open: "never", outputFolder: "report" }]],
  use: {
    baseURL: process.env.BASE_URL ?? "http://localhost:3000",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
