import { expect, test } from "@playwright/test";

test("trang chu tai duoc", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("EventFlow");
});

// Luong day du (issue 1909) da duoc thay bang specs/buy-flow.spec.ts (EV-198).
