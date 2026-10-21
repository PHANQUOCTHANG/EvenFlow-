import { defineConfig, devices } from "@playwright/test";

/**
 * E2E (issue 1909, EV-198): xep hang -> duoc goi -> giu ghe -> thanh toan -> nhan ve.
 *
 * Web that (Next build production) o BASE_URL; backend gia lap bang page.route o E2E_API_BASE
 * (fixtures/api-mock.ts). Web PHAI duoc build voi NEXT_PUBLIC_API_BASE = E2E_API_BASE
 * (mac dinh http://localhost:8080, giong compose cho nightly).
 *
 * Server cua Next tu goi API khi render (SSR/ISR: /events, /events/[slug]); page.route khong thay request do.
 * Nen `webServer` bat fixtures/stub-api.mjs (node:http, khong phu thuoc) o :8080 — cung events.json voi
 * api-mock.ts. Next chi fetch luc co request, nen thu tu bat web truoc / stub sau van dung. Request cua
 * TRINH DUYET toi :8080 van bi page.route chan truoc khi toi stub.
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
  webServer: {
    command: "node fixtures/stub-api.mjs",
    cwd: __dirname,
    url: "http://127.0.0.1:8080/api/v1/events/public",
    reuseExistingServer: !process.env.CI,
    timeout: 15_000,
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
