import { expect, test } from "@playwright/test";

/**
 * EV-187 (AC-10, plan §1.5 + §6 M-5): Ops mở danh sách -> vào war room sự kiện đang bán
 * -> thấy nhãn "Dữ liệu mẫu", phễu, bảng tồn kho, nút điều khiển bị vô hiệu
 * -> bấm "Làm mới" và thấy client cập nhật (chứng minh đã hydrate, không chỉ HTML server).
 *
 * Không giả lập API: dữ liệu mẫu chạy trong server component và trong trình duyệt.
 * Fail nếu console có lỗi (vd. lỗi hydration) hoặc có pageerror.
 */
const ON_SALE_TITLE = /Vietnam Tech Summit 2026/;

test("ops vào war room từ danh sách, thấy dữ liệu mẫu và làm mới được", async ({ page }) => {
  const consoleErrors: string[] = [];
  const pageErrors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error") consoleErrors.push(msg.text());
  });
  page.on("pageerror", (err) => pageErrors.push(err.message));

  await page.goto("/ops/events");
  await page.getByRole("link", { name: ON_SALE_TITLE }).click();

  await expect(page).toHaveURL(/\/ops\/events\/evt-tech-summit-2026$/);
  await expect(page.getByRole("heading", { level: 1, name: ON_SALE_TITLE })).toBeVisible();

  // AC-3: nhãn dữ liệu mẫu (role status).
  await expect(page.getByRole("status").filter({ hasText: "Dữ liệu mẫu" }).first()).toBeVisible();

  // AC-1: phễu và bảng tồn kho.
  await expect(page.getByRole("heading", { name: /phễu/i }).first()).toBeVisible();
  const inventory = page.getByRole("table").filter({ hasText: "Standard Pass" });
  await expect(inventory).toBeVisible();
  await expect(inventory.getByRole("columnheader", { name: /còn lại/i })).toBeVisible();

  // AC-6: nút điều khiển bị vô hiệu kèm giải thích EVF-60.
  const controls = page.getByRole("button", { name: /ghi đè|tạm dừng/i });
  await expect(controls.first()).toBeVisible();
  const count = await controls.count();
  for (let i = 0; i < count; i += 1) {
    await expect(controls.nth(i)).toBeDisabled();
  }
  await expect(page.getByText("Chưa có API ghi đè (EVF-60)").first()).toBeVisible();

  // M-5: bấm "Làm mới" và chờ dấu hiệu cập nhật từ client ("cập nhật lúc …" đổi giờ).
  const updatedAt = page.getByText(/cập nhật lúc\s*:?\s*\d{2}:\d{2}:\d{2}/i).first();
  await expect(updatedAt).toBeVisible();
  const before = (await updatedAt.textContent()) ?? "";
  await page.getByRole("button", { name: /làm mới/i }).first().click();
  await expect(updatedAt).not.toHaveText(before, { timeout: 10_000 });
  await expect(page.getByRole("button", { name: /làm mới/i }).first()).toBeEnabled();

  expect(pageErrors, "pageerror").toEqual([]);
  expect(consoleErrors, "console error").toEqual([]);
});
