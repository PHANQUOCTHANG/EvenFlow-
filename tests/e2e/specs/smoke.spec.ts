import { expect, test } from "@playwright/test";

test("trang chu tai duoc", async ({ page }) => {
  await page.goto("/");
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("EventFlow");
});

// Luong day du (issue 1909). Bo skip khi cac trang phong cho / checkout da co.
test.skip("xep hang -> duoc goi -> giu ghe -> thanh toan -> nhan ve", async ({ page }) => {
  await page.goto("/events/demo");
  await page.getByRole("button", { name: /vao hang cho/i }).click();
  await expect(page).toHaveURL(/\/waiting\//);
  await expect(page.getByTestId("queue-rank")).toBeVisible();
  // ... duoc admit -> /select -> /hold -> /result
});
