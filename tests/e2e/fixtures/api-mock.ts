import { expect, test as base, type BrowserContext, type Request, type Route } from "@playwright/test";

/**
 * Fixture gia lap backend cho E2E (EV-198, plan 1.4).
 *
 * - Web that (Next build production) chay o BASE_URL; backend gia o E2E_API_BASE. Web PHAI duoc
 *   build voi NEXT_PUBLIC_API_BASE = E2E_API_BASE.
 * - Chi request co origin = E2E_API_BASE moi duoc gia lap (theo method + path da dang ky).
 * - VI PHAM (AC-7) — ghi lai va lam FAIL test o teardown, kem URL cu the:
 *     1. request toi E2E_API_BASE khong co handler;
 *     2. request `/v1/**` toi origin cua web (BASE_URL) — dau hieu goi URL tuong doi (B2);
 *     3. request `/v1/**` toi bat ky origin nao khac (vd. web build voi NEXT_PUBLIC_API_BASE khac);
 *     4. loi trang (`pageerror`) va console nhac toi CORS (plan-review F3: loi CORS/mang co the
 *        bi nhanh "sandbox" cua payment-client bien thanh thanh cong gia).
 *   Request vi pham duoc tra 599 kem thong bao (co header CORS) de UI khong treo.
 *
 * CORS (plan-review F2): web :3000 va API gia :8080 khac origin; join/status dung
 * `credentials: "include"`, hold gui header tuy bien. Moi phan hoi (ke ca 599) mang
 * Access-Control-Allow-Origin = dung origin cua web, Allow-Credentials, Allow-Headers,
 * Expose-Headers. Preflight OPTIONS: Playwright (Chromium) tu tra 204 cho preflight khi bat
 * interception; handler OPTIONS ben duoi chi la luoi an toan neu preflight lot vao route.
 *
 * Hop dong endpoint -> phan hoi dung trong specs (plan-review F5; khop parser cua frontend):
 *
 * | Endpoint (origin E2E_API_BASE)                         | Phan hoi gia lap                                                         | Parser frontend |
 * |--------------------------------------------------------|---------------------------------------------------------------------------|-----------------|
 * | POST /v1/events/{id}/queue/join  (200)                 | { state:"QUEUED", queue_token, rank, is_new:true, poll_after_ms }         | queue-client.ts joinQueue |
 * | POST /v1/events/{id}/queue/join  (401)                 | { title:"Unauthorized", detail }                                          | waiting-room.tsx (status 401) |
 * | GET  /v1/events/{id}/queue/status (lan 1, 200)         | { state:"QUEUED", rank:42, eta_seconds:90, poll_after_ms:3500 } + ETag    | queue-status.ts normalizePosition |
 * | GET  /v1/events/{id}/queue/status (lan 2+, 200)        | { state:"ADMITTED", rank:0, eta_seconds:0, poll_after_ms:3500, expires_at: epoch ms +15') } | idem |
 * | GET  /v1/events/{id}/queue/stream                      | KHONG duoc goi (poll_after_ms 3500 > nguong SSE 3000) -> vi pham          | use-queue-status.ts |
 * | POST /v1/events/{id}/holds (201)                       | { order_id, hold_id, expires_at: ISO = now + 10' tinh LUC NHAN request }  | hold-client.ts requestHold |
 * | POST /v1/events/{id}/holds (409)                       | { title:"Conflict", detail }                                              | hold-client.ts -> SOLD_OUT |
 * | POST /v1/events/{id}/payments (200)                    | { payment_id, order_id, status:"PENDING", redirect_url: return_url + "&e2e=paid-ok" } | payment-client.ts initiatePayment |
 * | GET  /v1/events/{id}/orders/{orderId} (200)            | { order_id, event_id, event_title, venue, tier_name, quantity, total_amount, status:"PAID", paid_at, payment_method, issued_tickets:[{ ticketId, ticketCode, tierName, attendeeName, qrPayload }] } | payment-client.ts getOrderDetails (issued_tickets truyen nguyen, nen phan tu dung camelCase) |
 */

export const WEB_BASE = trimSlash(process.env.BASE_URL ?? "http://localhost:3000");
export const API_BASE = trimSlash(process.env.E2E_API_BASE ?? "http://localhost:8080");

const WEB_ORIGIN = new URL(WEB_BASE).origin;
const API_ORIGIN = new URL(API_BASE).origin;

const VIOLATION_STATUS = 599;

function trimSlash(value: string): string {
  return value.replace(/\/+$/, "");
}

export interface RecordedRequest {
  method: string;
  url: string;
  /** pathname (khong kem query). */
  path: string;
  /** Ten header da viet thuong (Playwright). */
  headers: Record<string, string>;
  /** Body JSON da parse; chuoi tho neu khong phai JSON; null neu khong co body. */
  body: unknown;
}

export interface MockReply {
  status?: number;
  json?: unknown;
  body?: string;
  contentType?: string;
  headers?: Record<string, string>;
}

export type MockHandler = (req: RecordedRequest) => MockReply | Promise<MockReply>;

type PathMatcher = string | RegExp;

interface Registration {
  method: string;
  path: PathMatcher;
  handler: MockHandler;
}

function corsHeaders(): Record<string, string> {
  return {
    "Access-Control-Allow-Origin": WEB_ORIGIN,
    "Access-Control-Allow-Credentials": "true",
    "Access-Control-Allow-Methods": "GET, POST, PUT, PATCH, DELETE, OPTIONS",
    "Access-Control-Allow-Headers":
      "Content-Type, Idempotency-Key, X-Queue-Token, X-Identity-Id, If-None-Match, Accept",
    "Access-Control-Expose-Headers": "ETag, X-Poll-After-Ms, Retry-After",
    Vary: "Origin",
  };
}

function matches(path: PathMatcher, actual: string): boolean {
  return typeof path === "string" ? path === actual : path.test(actual);
}

export class ApiMock {
  private readonly registrations: Registration[] = [];
  readonly requests: RecordedRequest[] = [];
  readonly violations: string[] = [];

  /** Dang ky handler cho `method` + `path` tren origin E2E_API_BASE. Dang ky sau thang dang ky truoc. */
  on(method: string, path: PathMatcher, handler: MockHandler | MockReply): this {
    const fn: MockHandler = typeof handler === "function" ? handler : () => handler;
    this.registrations.push({ method: method.toUpperCase(), path, handler: fn });
    return this;
  }

  /** Dang ky mot endpoint ma neu bi goi thi la vi pham (vd. SSE stream khi test chon polling). */
  forbid(method: string, path: PathMatcher, reason: string): this {
    return this.on(method, path, (req) => {
      this.violations.push(`${req.method} ${req.url} — ${reason}`);
      return {
        status: VIOLATION_STATUS,
        json: { title: "E2E violation", detail: reason },
      };
    });
  }

  /** Cac request da ghi nhan toi API gia, loc theo method + path. */
  calls(method: string, path: PathMatcher): RecordedRequest[] {
    const m = method.toUpperCase();
    return this.requests.filter((r) => r.method === m && matches(path, r.path));
  }

  async install(context: BrowserContext): Promise<void> {
    // 1. API gia (origin = E2E_API_BASE).
    await context.route(
      (url) => url.origin === API_ORIGIN,
      (route, request) => this.handleApi(route, request),
    );

    // 2. `/v1/**` toi origin cua web: dau hieu goi URL tuong doi (B2).
    //    3. `/v1/**` toi moi origin khac API gia (vd. build voi NEXT_PUBLIC_API_BASE khac).
    //    (Neu API_ORIGIN === WEB_ORIGIN thi route 1 da bat truoc — cau hinh do khong hop le cho E2E.)
    await context.route(
      (url) => url.origin !== API_ORIGIN && url.pathname.startsWith("/v1/"),
      async (route, request) => {
        const where = new URL(request.url()).origin === WEB_ORIGIN ? "origin cua web (BASE_URL)" : "origin khong phai E2E_API_BASE";
        const message = `${request.method()} ${request.url()} — request /v1/** toi ${where}; API phai di qua ${API_BASE}`;
        this.violations.push(message);
        await this.fulfillViolation(route, message);
      },
    );

    // 4. Loi trang va loi CORS tren console.
    context.on("page", (page) => this.watchPage(page));
    for (const page of context.pages()) this.watchPage(page);
  }

  private watchedPages = new WeakSet<object>();

  private watchPage(page: import("@playwright/test").Page): void {
    if (this.watchedPages.has(page)) return;
    this.watchedPages.add(page);
    page.on("pageerror", (err) => {
      this.violations.push(`pageerror tai ${page.url()}: ${err.message}`);
    });
    page.on("console", (msg) => {
      const text = msg.text();
      if (/cors|access-control/i.test(text)) {
        this.violations.push(`console CORS tai ${page.url()}: ${text}`);
      }
    });
  }

  private async handleApi(route: Route, request: Request): Promise<void> {
    const method = request.method().toUpperCase();
    const url = new URL(request.url());

    if (method === "OPTIONS") {
      await safeFulfill(route, { status: 204, headers: corsHeaders(), body: "" });
      return;
    }

    const recorded: RecordedRequest = {
      method,
      url: request.url(),
      path: url.pathname,
      headers: await request.allHeaders().catch(() => request.headers()),
      body: parseBody(request.postData()),
    };
    this.requests.push(recorded);

    let registration: Registration | undefined;
    for (let i = this.registrations.length - 1; i >= 0; i -= 1) {
      const r = this.registrations[i];
      if (r.method === method && matches(r.path, url.pathname)) {
        registration = r;
        break;
      }
    }

    if (!registration) {
      const message = `${method} ${request.url()} — request toi E2E_API_BASE khong co handler gia lap`;
      this.violations.push(message);
      await this.fulfillViolation(route, message);
      return;
    }

    let reply: MockReply;
    try {
      reply = await registration.handler(recorded);
    } catch (err) {
      const message = `${method} ${request.url()} — handler gia lap nem loi: ${String(err)}`;
      this.violations.push(message);
      await this.fulfillViolation(route, message);
      return;
    }

    const headers: Record<string, string> = { ...corsHeaders(), ...(reply.headers ?? {}) };
    const isJson = reply.json !== undefined;
    await safeFulfill(route, {
      status: reply.status ?? 200,
      headers,
      contentType: reply.contentType ?? (isJson ? "application/json" : "text/plain"),
      body: isJson ? JSON.stringify(reply.json) : (reply.body ?? ""),
    });
  }

  private async fulfillViolation(route: Route, message: string): Promise<void> {
    await safeFulfill(route, {
      status: VIOLATION_STATUS,
      headers: corsHeaders(),
      contentType: "application/json",
      body: JSON.stringify({ title: "E2E violation", detail: message }),
    });
  }

  assertNoViolations(): void {
    expect(
      this.violations,
      `Vi pham gia lap API (AC-7):\n${this.violations.map((v) => `  - ${v}`).join("\n")}`,
    ).toEqual([]);
  }
}

function parseBody(raw: string | null): unknown {
  if (raw === null || raw === "") return null;
  try {
    return JSON.parse(raw);
  } catch {
    return raw;
  }
}

async function safeFulfill(route: Route, options: Parameters<Route["fulfill"]>[0]): Promise<void> {
  try {
    await route.fulfill(options);
  } catch {
    // Trang da dong (test ket thuc) — khong con ai nhan phan hoi.
  }
}

/** Mot "cong" do test mo: handler gia lap await `wait()` de giu phan hoi cho toi khi test da
 *  quan sat xong trang thai trung gian. Thay cho waitForTimeout (AC-8). */
export function createGate(): { wait: () => Promise<void>; open: () => void } {
  let open!: () => void;
  const promise = new Promise<void>((resolve) => {
    open = resolve;
  });
  return { wait: () => promise, open };
}

/** Teardown cua fixture (AC-7): cho moi handler route dang chay xong (request den muon khong bi
 *  bo qua — code-review F-4), roi nem loi neu co vi pham, thong bao kem URL cu the.
 *  Export de test AC-7 goi truc tiep va chung minh dung co che nay nem loi. */
export async function finalizeApiMock(api: ApiMock, context: BrowserContext): Promise<void> {
  // Handler dang cho mot "cong" chua mo (test da fail som) se treo mai: gioi han thoi gian cho,
  // het han thi go route bo qua loi de teardown khong che loi goc bang mot timeout.
  let timer: ReturnType<typeof setTimeout> | undefined;
  const settled = await Promise.race([
    context.unrouteAll({ behavior: "wait" }).then(() => true),
    new Promise<boolean>((resolve) => {
      timer = setTimeout(() => resolve(false), UNROUTE_WAIT_MS);
    }),
  ]);
  clearTimeout(timer);
  if (!settled) {
    await context.unrouteAll({ behavior: "ignoreErrors" });
  }
  api.assertNoViolations();
}

const UNROUTE_WAIT_MS = 5_000;

export const test = base.extend<{ api: ApiMock }>({
  api: async ({ context }, use) => {
    const api = new ApiMock();
    await api.install(context);
    await use(api);
    await finalizeApiMock(api, context);
  },
});

export { expect };
