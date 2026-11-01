import { E2E_ORGANIZER_USER, expect, registerOrganizerSalesApi, seedRoleSession, test } from "../fixtures/api-mock";
import { EVENT_ID } from "../fixtures/buy-flow-helpers";

/**
 * ops-warroom AC-30 / AC-31: organizer sở hữu sự kiện mở trang doanh số và thấy số liệu thật từ
 * GET /v1/organizer/events/{id}/sales (API giả lập theo contracts §4.2): không còn nhãn "Dữ liệu mẫu",
 * peak TPS hiện "Chưa có dữ liệu", cơ cấu thanh toán theo nhà cung cấp trong dữ liệu.
 */
const SALES_PATH = `/v1/organizer/events/${EVENT_ID}/sales`;

test("organizer: doanh số thật, không nhãn mẫu, peak TPS chưa có dữ liệu", async ({ page, api }) => {
  const consoleErrors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error") consoleErrors.push(msg.text());
  });

  registerOrganizerSalesApi(api, EVENT_ID);
  await seedRoleSession(page, E2E_ORGANIZER_USER);

  await page.goto(`/organizer/events/${EVENT_ID}/sales`);

  await expect(page.getByRole("heading", { level: 1, name: /Vietnam Tech Summit 2026/ })).toBeVisible();
  await expect(page.getByText("Tổng doanh thu")).toBeVisible();
  await expect(page.getByText("Standard Pass")).toBeVisible();
  await expect(page.getByText(/222\.500\.000/).first()).toBeVisible();
  await expect(page.getByText(/Dữ liệu mẫu/)).toHaveCount(0);
  expect(api.calls("GET", SALES_PATH).length).toBeGreaterThan(0);
  expect(api.calls("GET", SALES_PATH)[0].headers.authorization).toMatch(/^Bearer /);

  await page.getByRole("tab", { name: /Báo cáo tổng kết sau bán/ }).click();
  const tps = page.getByText("Tốc độ giao dịch cao điểm").locator("..");
  await expect(tps).toContainText("Chưa có dữ liệu");
  await expect(tps).not.toContainText("TPS");
  await expect(page.getByText("vnpay")).toBeVisible();
  await expect(page.getByText("sandbox")).toBeVisible();

  expect(consoleErrors, "console error").toEqual([]);
});
