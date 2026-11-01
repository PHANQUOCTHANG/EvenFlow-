import { E2E_OPS_USER, expect, registerOpsApi, seedRoleSession, test } from "../fixtures/api-mock";
import { EVENT_ID } from "../fixtures/buy-flow-helpers";

/**
 * ops-warroom AC-31 (thay EV-187 AC-10 qua test change request): Ops đăng nhập, mở danh sách, vào war room của
 * sự kiện đang bán và thấy số liệu THẬT (API giả lập theo contracts §3-§5, không còn nhãn "Dữ liệu mẫu"), rồi
 * ghi đè admit rate -> tạm dừng khẩn cấp -> tiếp tục tự động. Mỗi lệnh phải tới API với đúng body, trạng thái mới
 * chỉ hiện sau khi server xác nhận.
 *
 * Fail nếu console có lỗi hoặc pageerror; fixture api-mock làm fail khi có request lạ tới API giả.
 */
const ON_SALE_TITLE = /Vietnam Tech Summit 2026/;
const OVERRIDE_PATH = `/v1/ops/events/${EVENT_ID}/admit-override`;

test("ops: war room số liệu thật, ghi đè -> tạm dừng -> tiếp tục tự động", async ({ page, api }) => {
  const consoleErrors: string[] = [];
  page.on("console", (msg) => {
    if (msg.type() === "error") consoleErrors.push(msg.text());
  });

  const state = registerOpsApi(api, EVENT_ID);
  await seedRoleSession(page, E2E_OPS_USER);

  await page.goto("/ops/events");
  await page.getByRole("link", { name: ON_SALE_TITLE }).click();

  await expect(page).toHaveURL(new RegExp(`/ops/events/${EVENT_ID}$`));
  await expect(page.getByRole("heading", { level: 1, name: ON_SALE_TITLE })).toBeVisible();

  // Số liệu thật: không có nhãn mẫu; phễu, tồn kho, sức khỏe checkout đến từ API giả lập.
  await expect(page.getByText("Dữ liệu mẫu")).toHaveCount(0);
  await expect(page.getByText("4.210").first()).toBeVisible();
  const inventory = page.getByRole("table").filter({ hasText: "Standard Pass" });
  await expect(inventory).toBeVisible();
  await expect(inventory.getByRole("columnheader", { name: /còn lại/i })).toBeVisible();
  await expect(page.getByText("420 ms")).toBeVisible();

  const controls = page.getByRole("region", { name: "Điều khiển" });
  const reason = controls.getByRole("textbox", { name: /lý do/i });

  // 1. Ghi đè admit rate 120.
  await controls.getByRole("spinbutton", { name: /tốc độ/i }).fill("120");
  await reason.fill("checkout p99 cao, giảm tải");
  await controls.getByRole("button", { name: "Ghi đè admit rate" }).click();
  await expect(controls.getByRole("status").filter({ hasText: "120" })).toBeVisible();
  expect(api.calls("PUT", OVERRIDE_PATH).at(-1)?.body).toEqual({ rate_per_sec: 120, reason: "checkout p99 cao, giảm tải" });
  await expect(page.getByText(/Ops ghi đè: 120 khách\/giây/)).toBeVisible();

  // 2. Tạm dừng khẩn cấp: cần xác nhận, gửi rate 0, rồi hiện "ĐANG TẠM DỪNG" sau khi làm mới.
  await controls.getByRole("button", { name: "Tạm dừng khẩn cấp" }).click();
  await reason.fill("sự cố cổng thanh toán");
  await page.getByRole("button", { name: /xác nhận/i }).click();
  await expect(page.getByText(/ĐANG TẠM DỪNG/)).toBeVisible();
  expect(api.calls("PUT", OVERRIDE_PATH).at(-1)?.body).toEqual({ rate_per_sec: 0, reason: "sự cố cổng thanh toán" });
  expect(state.override).toEqual({ active: true, ratePerSec: 0 });

  // 3. Tiếp tục tự động: DELETE, banner tạm dừng biến mất, chế độ về AUTO.
  await reason.fill("sự cố đã xử lý");
  await controls.getByRole("button", { name: /Bỏ ghi đè|Tiếp tục tự động/ }).click();
  await expect(page.getByText(/ĐANG TẠM DỪNG/)).toHaveCount(0);
  expect(api.calls("DELETE", OVERRIDE_PATH).at(-1)?.body).toEqual({ reason: "sự cố đã xử lý" });
  await expect(controls.getByRole("button", { name: /Bỏ ghi đè|Tiếp tục tự động/ })).toHaveCount(0);

  // Mọi lệnh ghi mang Bearer của phiên ops.
  for (const call of [...api.calls("PUT", OVERRIDE_PATH), ...api.calls("DELETE", OVERRIDE_PATH)]) {
    expect(call.headers.authorization).toMatch(/^Bearer /);
  }

  expect(consoleErrors, "console error").toEqual([]);
});
