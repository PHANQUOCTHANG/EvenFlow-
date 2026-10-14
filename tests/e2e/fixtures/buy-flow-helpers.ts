import type { Locator, Page } from "@playwright/test";

import { type ApiMock, createGate, expect } from "./api-mock";

/**
 * Helper dung chung cho luong mua ve tren backend gia lap (EV-198, tach ra o EV-188 plan §1.4 + §6 M4).
 *
 * Tach tu specs/buy-flow.spec.ts, HANH VI KHONG DOI. Diem moi duy nhat: buoc "vao phong cho -> checkout"
 * cu (queueUntilCheckout) nay la hai ham noi tiep `joinQueueFromEvent` (dung o QUEUED + rank, cong admit
 * con dong) va `admitToCheckout` (mo cong -> tu chuyen toi checkout) — de bai quet axe (a11y.spec.ts)
 * co diem dung o trang thai QUEUED.
 *
 * Quyet dinh giu nguyen tu EV-198:
 * - Jitter 0..5s cua trang su kien: `Math.random = () => 0` qua addInitScript -> sleepWithJitter resolve
 *   ngay. Khong dung page.clock / waitForTimeout.
 * - Hang cho: POLLING voi poll_after_ms = 3500 (> nguong SSE 3000) nen hook khong nang len SSE; stream
 *   bi danh dau vi pham neu bi goi. Lan poll thu 2 (ADMITTED) bi "cong" giu lai cho toi khi test mo.
 * - Cho hydrate truoc khi tuong tac voi trang SSR: doi phan tu co thuoc tinh React `__reactProps$*`.
 */

export const EVENT_ID = "evt-tech-summit-2026";
export const EVENT_SLUG = "vietnam-tech-summit-2026";
export const TIER_ID = "tier-all-access";
export const TIER_NAME = "All-Access Executive Pass";
export const QUANTITY = 2;
export const TOTAL_TEXT = "5.000.000 đ"; // lib/format.ts formatVnd(5_000_000)
export const QUEUE_TOKEN = "qt-e2e-7f3c2a91";
export const ORDER_ID = "ord-e2e-0001";
export const HOLD_ID = "hold-e2e-0001";
export const TICKET_CODES = ["E2E-TKT-0001", "E2E-TKT-0002"];
export const RANK = 42;

export const P = {
  join: `/v1/events/${EVENT_ID}/queue/join`,
  status: `/v1/events/${EVENT_ID}/queue/status`,
  stream: `/v1/events/${EVENT_ID}/queue/stream`,
  holds: `/v1/events/${EVENT_ID}/holds`,
  payments: `/v1/events/${EVENT_ID}/payments`,
  order: `/v1/events/${EVENT_ID}/orders/${ORDER_ID}`,
};

export type Gate = { open: () => void };

/** AC-8 (EV-198): jitter tat dinh = 0. Goi trong beforeEach, truoc page.goto dau tien. */
export async function installDeterministicJitter(page: Page): Promise<void> {
  await page.addInitScript(() => {
    Math.random = () => 0;
  });
}

/** Doi toi khi phan tu da duoc React hydrate (co props/handler gan vao DOM node). */
export async function waitForHydration(target: Locator): Promise<void> {
  await expect
    .poll(() => target.evaluate((el) => Object.keys(el).some((k) => k.startsWith("__reactProps$"))), {
      message: "phan tu chua duoc React hydrate",
    })
    .toBe(true);
}

/** Join 200 + status QUEUED (lan 1) -> ADMITTED (lan 2, giu boi cong). Tra ve cong de mo. */
export function mockQueue(api: ApiMock) {
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
export function mockHoldCreated(api: ApiMock) {
  api.on("POST", P.holds, () => ({
    status: 201,
    json: {
      order_id: ORDER_ID,
      hold_id: HOLD_ID,
      expires_at: new Date(Date.now() + 10 * 60_000).toISOString(),
    },
  }));
}

/** Payments 200: redirect_url = return_url + "e2e=paid-ok" (dau rieng chung minh redirect den tu server gia). */
export function mockPaymentRedirect(api: ApiMock) {
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
        // plan-review F3 (EV-198): dau rieng chung minh redirect den tu server gia, khong phai nhanh sandbox gia.
        redirect_url: `${returnUrl}${sep}e2e=paid-ok`,
      },
    };
  });
}

/** Order 200 da thanh toan, kem ve dien tu. */
export function mockPaidOrder(api: ApiMock) {
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
}

/** Mo trang su kien, cho nut vao phong cho hydrate, bam, cho URL phong cho. */
export async function clickJoinFromEvent(page: Page): Promise<void> {
  await page.goto(`/events/${EVENT_SLUG}`);
  const joinButton = page.getByRole("button", { name: "Vào phòng chờ mua vé" });
  await expect(joinButton).toBeVisible();
  await waitForHydration(joinButton);
  await joinButton.click();

  await page.waitForURL(`**/waiting/${EVENT_ID}`);
}

/** Trang su kien -> bam vao phong cho -> thay QUEUED + rank. Cong admit CON DONG (goi admitToCheckout de di tiep). */
export async function joinQueueFromEvent(page: Page, api: ApiMock): Promise<void> {
  await clickJoinFromEvent(page);

  // QUEUED co rank.
  const position = page.getByRole("region", { name: "Thông tin vị trí hàng đợi" });
  await expect(position.getByText("Số thứ tự của bạn")).toBeVisible();
  await expect(position.getByText(String(RANK), { exact: true })).toBeVisible();
  expect(api.calls("POST", P.join)).toHaveLength(1);
}

/** Server tra ADMITTED (mo cong) -> tu chuyen toi checkout. */
export async function admitToCheckout(page: Page, admit: Gate): Promise<void> {
  admit.open();
  await page.waitForURL(`**/checkout/${EVENT_ID}`);
  await expect(page.getByRole("heading", { level: 1, name: /Chọn vé/ })).toBeVisible();
}

/** Chon All-Access x 2 tren trang checkout. */
export async function selectAllAccessTimesTwo(page: Page): Promise<void> {
  const increase = page.getByRole("button", { name: `Tăng số lượng ${TIER_NAME}` });
  await increase.click();
  await expect(page.getByRole("status", { name: `Số lượng ${TIER_NAME}` })).toHaveText("1");
  await increase.click();
  await expect(page.getByRole("status", { name: `Số lượng ${TIER_NAME}` })).toHaveText(String(QUANTITY));
}

/** Bam "Giữ vé và thanh toán" -> dang giu ve co dong ho + nut tiep tuc. */
export async function holdSelectedTickets(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Giữ vé và thanh toán" }).click();

  await expect(page.getByText("Thời gian giữ vé còn").first()).toBeVisible();
  await expect(page.getByRole("button", { name: "Tiếp tục thanh toán" })).toBeVisible();
}

/** Bam "Tiếp tục thanh toán" -> trang thanh toan (URL co orderId/holdId). */
export async function continueToPayment(page: Page): Promise<void> {
  await page.getByRole("button", { name: "Tiếp tục thanh toán" }).click();
  await page.waitForURL(`**/checkout/${EVENT_ID}/payment?**`);
}

/** Dien form nguoi nhan, bam thanh toan -> trang ket qua (URL), cho thong bao da thanh toan. */
export async function fillRecipientAndPay(page: Page): Promise<void> {
  await waitForHydration(page.locator("#payment-form"));
  await page.getByLabel("Họ và tên người nhận").fill("Nguyễn Văn E2E");
  await page.getByLabel("Email nhận vé điện tử").fill("e2e@example.com");
  await page.getByLabel("Số điện thoại liên hệ").fill("0901234567");
  await page.getByRole("button", { name: `Thanh toán ${TOTAL_TEXT}` }).click();

  await page.waitForURL(`**/checkout/${EVENT_ID}/result?**`);
}
