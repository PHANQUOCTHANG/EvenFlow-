import type { Locator, Page } from "@playwright/test";

import { ApiMock, createGate, expect, finalizeApiMock, test } from "../fixtures/api-mock";

/**
 * EV-198 — E2E luong mua ve day du tren web build production, backend gia lap theo hop dong HTTP.
 * Spec: docs/workflow/EV-198-e2e-playwright/spec.md (AC-1..AC-8). Hop dong endpoint: fixtures/api-mock.ts.
 *
 * Quyet dinh:
 * - Jitter 0..5s cua trang su kien (AC-8, plan-review F8): `Math.random = () => 0` qua addInitScript
 *   -> calculateJitterMs = 0 -> sleepWithJitter resolve ngay. Khong dung page.clock / waitForTimeout.
 * - Trang thai hang cho (plan-review F9): POLLING voi poll_after_ms = 3500 (> nguong SSE 3000 cua
 *   use-queue-status.ts) nen hook khong nang len SSE; endpoint stream bi danh dau vi pham neu bi goi.
 *   Lan poll thu 2 (ADMITTED) bi "cong" giu lai cho toi khi test da thay QUEUED + rank -> khong
 *   phu thuoc toc do may.
 * - Cho hydrate truoc khi tuong tac voi trang SSR (nut/Form chi co handler sau hydrate): doi phan tu
 *   co thuoc tinh React `__reactProps$*`.
 */

const EVENT_ID = "evt-tech-summit-2026";
const EVENT_SLUG = "vietnam-tech-summit-2026";
const TIER_ID = "tier-all-access";
const TIER_NAME = "All-Access Executive Pass";
const QUANTITY = 2;
const TOTAL_TEXT = "5.000.000 đ"; // lib/format.ts formatVnd(5_000_000)
const QUEUE_TOKEN = "qt-e2e-7f3c2a91";
const ORDER_ID = "ord-e2e-0001";
const HOLD_ID = "hold-e2e-0001";
const TICKET_CODES = ["E2E-TKT-0001", "E2E-TKT-0002"];
const RANK = 42;

const P = {
  join: `/v1/events/${EVENT_ID}/queue/join`,
  status: `/v1/events/${EVENT_ID}/queue/status`,
  stream: `/v1/events/${EVENT_ID}/queue/stream`,
  holds: `/v1/events/${EVENT_ID}/holds`,
  payments: `/v1/events/${EVENT_ID}/payments`,
  order: `/v1/events/${EVENT_ID}/orders/${ORDER_ID}`,
};

test.beforeEach(async ({ page }) => {
  // AC-8: jitter tat dinh = 0.
  await page.addInitScript(() => {
    Math.random = () => 0;
  });
});

/** Doi toi khi phan tu da duoc React hydrate (co props/handler gan vao DOM node). */
async function waitForHydration(target: Locator): Promise<void> {
  await expect
    .poll(() => target.evaluate((el) => Object.keys(el).some((k) => k.startsWith("__reactProps$"))), {
      message: "phan tu chua duoc React hydrate",
    })
    .toBe(true);
}

/** Join 200 + status QUEUED (lan 1) -> ADMITTED (lan 2, giu boi cong). Tra ve cong de mo. */
function mockQueue(api: ApiMock) {
  const admit = createGate();
  let statusCalls = 0;

  api.on("POST", P.join, {
    status: 200,
    json: { state: "QUEUED", queue_token: QUEUE_TOKEN, rank: RANK, is_new: true, poll_after_ms: 3500 },
  });

  api.on("GET", P.status, async () => {
    statusCalls += 1;
    if (statusCalls === 1) {
      return {
        status: 200,
        headers: { ETag: '"pos-1"' },
        json: { state: "QUEUED", rank: RANK, eta_seconds: 90, poll_after_ms: 3500 },
      };
    }
    await admit.wait();
    return {
      status: 200,
      headers: { ETag: '"pos-2"' },
      json: {
        state: "ADMITTED",
        rank: 0,
        eta_seconds: 0,
        poll_after_ms: 3500,
        expires_at: Date.now() + 15 * 60_000,
      },
    };
  });

  api.forbid("GET", P.stream, "SSE stream khong duoc goi: test chon polling (poll_after_ms 3500 > 3000)");

  return admit;
}

/** Hold 201: expires_at tinh luc nhan request (now + 10 phut). */
function mockHoldCreated(api: ApiMock) {
  api.on("POST", P.holds, () => ({
    status: 201,
    json: {
      order_id: ORDER_ID,
      hold_id: HOLD_ID,
      expires_at: new Date(Date.now() + 10 * 60_000).toISOString(),
    },
  }));
}

/** Trang su kien -> bam vao phong cho -> thay QUEUED + rank -> mo cong -> tu chuyen toi checkout. */
async function queueUntilCheckout(page: Page, api: ApiMock, admit: { open: () => void }) {
  await page.goto(`/events/${EVENT_SLUG}`);
  const joinButton = page.getByRole("button", { name: "Vào phòng chờ mua vé" });
  await expect(joinButton).toBeVisible();
  await waitForHydration(joinButton);
  await joinButton.click();

  await page.waitForURL(`**/waiting/${EVENT_ID}`);

  // QUEUED co rank.
  const position = page.getByRole("region", { name: "Thông tin vị trí hàng đợi" });
  await expect(position.getByText("Số thứ tự của bạn")).toBeVisible();
  await expect(position.getByText(String(RANK), { exact: true })).toBeVisible();
  expect(api.calls("POST", P.join)).toHaveLength(1);

  // Server tra ADMITTED -> tu chuyen trang.
  admit.open();
  await page.waitForURL(`**/checkout/${EVENT_ID}`);
  await expect(page.getByRole("heading", { level: 1, name: /Chọn vé/ })).toBeVisible();
}

/** Chon All-Access x 2 tren trang checkout. */
async function selectAllAccessTimesTwo(page: Page) {
  const increase = page.getByRole("button", { name: `Tăng số lượng ${TIER_NAME}` });
  await increase.click();
  await expect(page.getByRole("status", { name: `Số lượng ${TIER_NAME}` })).toHaveText("1");
  await increase.click();
  await expect(page.getByRole("status", { name: `Số lượng ${TIER_NAME}` })).toHaveText(String(QUANTITY));
}

test.describe("EV-198 luong mua ve (backend gia lap)", () => {
  test("AC-1..AC-4: xep hang -> duoc goi -> giu ve -> thanh toan -> nhan ve", async ({ page, api }) => {
    const admit = mockQueue(api);
    mockHoldCreated(api);

    api.on("POST", P.payments, (req) => {
      const body = (req.body ?? {}) as { return_url?: string };
      const returnUrl = body.return_url ?? `/checkout/${EVENT_ID}/result?orderId=${ORDER_ID}`;
      const sep = returnUrl.includes("?") ? "&" : "?";
      return {
        status: 200,
        json: {
          payment_id: "pay-e2e-0001",
          order_id: ORDER_ID,
          status: "PENDING",
          // plan-review F3: dau rieng chung minh redirect den tu server gia, khong phai nhanh sandbox gia.
          redirect_url: `${returnUrl}${sep}e2e=paid-ok`,
        },
      };
    });

    api.on("GET", P.order, {
      status: 200,
      json: {
        order_id: ORDER_ID,
        event_id: EVENT_ID,
        event_title: "Vietnam Tech Summit 2026 — Kỷ Nguyên AI & Cloud Scale",
        venue: "Trung tâm Hội nghị Quốc gia, Hà Nội",
        tier_name: TIER_NAME,
        quantity: QUANTITY,
        total_amount: 5_000_000,
        status: "PAID",
        paid_at: "2026-10-13T03:00:00Z",
        payment_method: "sandbox",
        // payment-client.ts truyen nguyen issued_tickets; order-result-view doc ticketCode (camelCase).
        issued_tickets: TICKET_CODES.map((code, i) => ({
          ticketId: `tkt-e2e-${i + 1}`,
          ticketCode: code,
          tierName: TIER_NAME,
          attendeeName: "Nguyen Van E2E",
          qrPayload: `QR-${code}`,
        })),
      },
    });

    await test.step("AC-1: trang su kien -> phong cho (QUEUED, rank) -> ADMITTED -> checkout", async () => {
      await queueUntilCheckout(page, api, admit);
    });

    await test.step("AC-1/AC-2: chon All-Access x2, giu ve -> dang giu ve co dong ho", async () => {
      await selectAllAccessTimesTwo(page);
      await page.getByRole("button", { name: "Giữ vé và thanh toán" }).click();

      await expect(page.getByText("Thời gian giữ vé còn").first()).toBeVisible();
      await expect(page.getByRole("button", { name: "Tiếp tục thanh toán" })).toBeVisible();
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
      await page.getByRole("button", { name: "Tiếp tục thanh toán" }).click();
      await page.waitForURL(`**/checkout/${EVENT_ID}/payment?**`);

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
      await waitForHydration(page.locator("#payment-form"));
      await page.getByLabel("Họ và tên người nhận").fill("Nguyễn Văn E2E");
      await page.getByLabel("Email nhận vé điện tử").fill("e2e@example.com");
      await page.getByLabel("Số điện thoại liên hệ").fill("0901234567");
      await page.getByRole("button", { name: `Thanh toán ${TOTAL_TEXT}` }).click();

      await page.waitForURL(`**/checkout/${EVENT_ID}/result?**`);
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

    await page.goto(`/events/${EVENT_SLUG}`);
    const joinButton = page.getByRole("button", { name: "Vào phòng chờ mua vé" });
    await expect(joinButton).toBeVisible();
    await waitForHydration(joinButton);
    await joinButton.click();
    await page.waitForURL(`**/waiting/${EVENT_ID}`);

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

    await queueUntilCheckout(page, api, admit);
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
