import { expect, finalizeApiMock, test } from "../fixtures/api-mock";
import {
  EVENT_ID,
  HOLD_ID,
  ORDER_ID,
  P,
  QUANTITY,
  QUEUE_TOKEN,
  TICKET_CODES,
  TIER_ID,
  TIER_NAME,
  TOTAL_TEXT,
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
} from "../fixtures/buy-flow-helpers";

/**
 * EV-198 — E2E luong mua ve day du tren web build production, backend gia lap theo hop dong HTTP.
 * Spec: docs/workflow/EV-198-e2e-playwright/spec.md (AC-1..AC-8). Hop dong endpoint: fixtures/api-mock.ts.
 *
 * Helper gia lap / dieu huong nam o fixtures/buy-flow-helpers.ts (tach o EV-188, hanh vi khong doi;
 * a11y.spec.ts dung chung). Quyet dinh jitter / polling / cho hydrate: xem dau file helper.
 */

test.beforeEach(async ({ page }) => {
  // AC-8: jitter tat dinh = 0.
  await installDeterministicJitter(page);
});

test.describe("EV-198 luong mua ve (backend gia lap)", () => {
  test("AC-1..AC-4: xep hang -> duoc goi -> giu ve -> thanh toan -> nhan ve", async ({ page, api }) => {
    const admit = mockQueue(api);
    mockHoldCreated(api);
    mockPaymentRedirect(api);
    mockPaidOrder(api);

    await test.step("AC-1: trang su kien -> phong cho (QUEUED, rank) -> ADMITTED -> checkout", async () => {
      await joinQueueFromEvent(page, api);
      await admitToCheckout(page, admit);
    });

    await test.step("AC-1/AC-2: chon All-Access x2, giu ve -> dang giu ve co dong ho", async () => {
      await selectAllAccessTimesTwo(page);
      await holdSelectedTickets(page);
    });

    await test.step("AC-2: hold dung token, dung dia chi, dung body", async () => {
      const holds = api.calls("POST", P.holds);
      expect(holds, "phai co dung 1 request POST {API_BASE}/holds").toHaveLength(1);
      const hold = holds[0];
      expect(hold.headers["x-queue-token"]).toBe(QUEUE_TOKEN);
      expect(hold.headers["idempotency-key"] ?? "").not.toBe("");
      expect(hold.body).toEqual({ ticket_type_id: TIER_ID, quantity: QUANTITY });
      api.assertNoViolations();
    });

    await test.step("AC-4: trang thanh toan hien dung hang ve, so luong, tong tien", async () => {
      await continueToPayment(page);

      const url = new URL(page.url());
      expect(url.searchParams.get("orderId")).toBe(ORDER_ID);
      expect(url.searchParams.get("holdId")).toBe(HOLD_ID);

      await expect(page.getByRole("heading", { level: 1, name: "Thanh toán đơn hàng" })).toBeVisible();
      await expect(page.getByText(TIER_NAME, { exact: true })).toBeVisible();
      await expect(page.getByText(`${QUANTITY} vé`, { exact: true })).toBeVisible();
      await expect(page.getByText(TOTAL_TEXT, { exact: true })).toBeVisible();
      await expect(page.getByRole("button", { name: `Thanh toán ${TOTAL_TEXT}` })).toBeVisible();
    });

    await test.step("AC-1/AC-3: dien thong tin, thanh toan -> trang ket qua da thanh toan + ma ve", async () => {
      await fillRecipientAndPay(page);

      const url = new URL(page.url());
      expect(url.searchParams.get("e2e"), "redirect phai den tu phan hoi payments gia (khong phai nhanh sandbox)").toBe(
        "paid-ok",
      );
      expect(url.searchParams.get("orderId")).toBe(ORDER_ID);

      await expect(page.getByText("Đã thanh toán thành công")).toBeVisible();
      const tickets = page.getByRole("region", { name: "Danh sách vé điện tử" });
      for (const code of TICKET_CODES) {
        await expect(tickets.getByText(code, { exact: true }).first()).toBeVisible();
      }
    });

    await test.step("AC-3: request thanh toan dung don", async () => {
      const payments = api.calls("POST", P.payments);
      expect(payments, "phai co dung 1 request POST {API_BASE}/payments").toHaveLength(1);
      const pay = payments[0];
      expect(pay.headers["idempotency-key"] ?? "").not.toBe("");
      const body = pay.body as Record<string, unknown>;
      expect(body.order_id).toBe(ORDER_ID);
      expect(body.hold_id).toBe(HOLD_ID);
      expect(body.amount).toBe(5_000_000);
      expect(api.calls("GET", P.order).length).toBeGreaterThanOrEqual(1);
    });
  });

  test("AC-5: join tra 401 -> phong cho bao can dang nhap/OTP, khong chuyen trang", async ({ page, api }) => {
    api.on("POST", P.join, {
      status: 401,
      json: { title: "Unauthorized", detail: "identity required" },
    });
    api.forbid("GET", P.status, "khong duoc poll status khi join that bai");
    api.forbid("GET", P.stream, "khong duoc mo stream khi join that bai");

    await clickJoinFromEvent(page);

    const alert = page.getByRole("alert").filter({ hasText: "Bạn cần đăng nhập và xác thực OTP" });
    await expect(alert).toBeVisible();
    await expect(alert).toContainText("Không thể vào phòng chờ");
    expect(new URL(page.url()).pathname).toBe(`/waiting/${EVENT_ID}`);
    expect(api.calls("POST", P.join)).toHaveLength(1);
  });

  test("AC-6: hold tra 409 -> bao hang ve da het, khong sang trang thanh toan", async ({ page, api }) => {
    const admit = mockQueue(api);
    api.on("POST", P.holds, { status: 409, json: { title: "Conflict", detail: "sold out" } });
    api.forbid("POST", P.payments, "khong duoc thanh toan khi hold that bai");

    await joinQueueFromEvent(page, api);
    await admitToCheckout(page, admit);
    await selectAllAccessTimesTwo(page);
    await page.getByRole("button", { name: "Giữ vé và thanh toán" }).click();

    const summary = page.getByRole("region", { name: "Tóm tắt đơn hàng" });
    const soldOut = summary.getByRole("alert").filter({ hasText: "Phần vé bạn chọn vừa được người khác mua hết" });
    await expect(soldOut).toBeVisible();
    await expect(soldOut).toContainText("Đã hết vé");
    await expect(page.getByRole("button", { name: "Tiếp tục thanh toán" })).toHaveCount(0);

    expect(api.calls("POST", P.holds), "hold phai duoc gui toi API_BASE dung 1 lan").toHaveLength(1);
    expect(new URL(page.url()).pathname).toBe(`/checkout/${EVENT_ID}`);
  });
});

test.describe("AC-7: gia lap khong che loi", () => {
  test("ghi vi pham kem URL cho request /v1/** toi origin web va request API khong co handler", async ({
    page,
    api,
  }) => {
    await page.goto("/");
    const results = await page.evaluate(async (apiBase) => {
      const a = await fetch("/v1/events/evt-x/holds", { method: "POST" });
      const b = await fetch(`${apiBase}/v1/events/evt-x/unknown`);
      return [a.status, b.status, await b.text()];
    }, process.env.E2E_API_BASE ?? "http://localhost:8080");

    expect(results[0]).toBe(599);
    expect(results[1]).toBe(599);
    expect(String(results[2])).toContain("/v1/events/evt-x/unknown");

    expect(api.violations).toHaveLength(2);
    expect(api.violations[0]).toContain("/v1/events/evt-x/holds");
    expect(api.violations[0]).toContain("origin cua web");
    expect(api.violations[1]).toContain("/v1/events/evt-x/unknown");
    expect(api.violations[1]).toContain("khong co handler");

    // Da kiem noi dung; xoa de teardown khong fail test nay (test ben duoi kiem teardown).
    api.violations.length = 0;
  });

  test("teardown cua fixture nem loi kem URL khi co request /v1/** toi origin web", async ({ page, context, api }) => {
    await page.goto("/");
    const status = await page.evaluate(async () => {
      const res = await fetch("/v1/events/evt-x/payments", { method: "POST" });
      return res.status;
    });

    // 1. Route web-origin `/v1/**` cua fixture da chan request (khong lot toi Next -> 404).
    expect(status).toBe(599);

    // 2. Vi pham duoc ghi dung loai, dung URL.
    const url = new URL("/v1/events/evt-x/payments", page.url()).href;
    expect(api.violations).toHaveLength(1);
    expect(api.violations[0]).toContain(`POST ${url}`);
    expect(api.violations[0]).toContain("origin cua web");

    // 3. Chinh ham teardown cua fixture nem loi, va thong bao neu ro URL.
    let thrown: unknown = null;
    try {
      await finalizeApiMock(api, context);
    } catch (err) {
      thrown = err;
    }
    expect(thrown, "teardown phai nem loi khi co vi pham").not.toBeNull();
    expect(String((thrown as Error).message)).toContain(url);
    expect(String((thrown as Error).message)).toContain("Vi pham gia lap API (AC-7)");

    // Da chung minh co che; xoa vi pham de teardown that cua test nay xanh.
    api.violations.length = 0;
  });
});
