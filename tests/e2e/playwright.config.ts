import { defineConfig, devices } from "@playwright/test";

/**
 * E2E (issue 1909): xep hang -> duoc goi -> giu ghe -> thanh toan -> nhan ve.
 *
 * Chay tren stack local (make up). Trong CI dat BASE_URL toi moi truong staging.
 * Gemini o che do mock, cong thanh toan o che do sandbox mock (issue 1501).
 */
export default defineConfig({
  testDir: "./specs",
  fullyParallel: true,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 2 : 0,
  reporter: [["list"], ["html", { open: "never", outputFolder: "report" }]],
  use: {
    baseURL: process.env.BASE_URL ?? "http://localhost:3000",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
