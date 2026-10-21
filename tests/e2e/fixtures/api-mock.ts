import { expect, test as base, type BrowserContext, type Request, type Route } from "@playwright/test";

import eventsFixture from "./events.json";

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
 * CORS (frontend-real-api AC-28): y het gateway (services/gateway/internal/proxy/gateway.go, CORS):
 * Allow-Origin = origin cua web, Allow-Headers `Authorization, Content-Type, Idempotency-Key,
 * X-Request-Id, Last-Event-ID, X-Queue-Token, If-None-Match`, Expose-Headers `X-Request-Id,
 * Retry-After, ETag, X-Poll-After-Ms`, KHONG co Allow-Credentials (web khong gui cookie).
 * Playwright tu tra 204 cho preflight khi bat interception nen trinh duyet khong tu chan header la:
 * fixture tu kiem moi request co header tac gia (Authorization, Idempotency-Key, X-*, If-None-Match...)
 * nam trong Allow-Headers; ngoai danh sach la VI PHAM (gateway that se chan preflight).
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
 * | POST /v1/events/{id}/holds (200, nhu gateway that)     | { order_id, hold_id, expires_at: ISO = now + 10' tinh LUC NHAN request }  | hold-client.ts requestHold |
 * | POST /v1/events/{id}/holds (409)                       | { title:"Conflict", detail }                                              | hold-client.ts -> SOLD_OUT |
 * | POST /v1/events/{id}/payments (200)                    | { payment_id, order_id, status:"PENDING", redirect_url: return_url + "&e2e=paid-ok" } | payment-client.ts initiatePayment |
 * | GET  /v1/orders/{orderId} (200)                       | ten truong that cua contracts.md muc 4 (order_id, total_amount, total_cents, items, issued_tickets:[{ticketId,ticketCode,tierName,attendeeName,status}] — KHONG co qrPayload) | payment-client.ts getOrderDetails |
| POST /v1/orders/{orderId}/issue-tickets (200)         | { order_id, tickets:[{ id, ticket_type_id, status, qr_payload }] }        | payment-client.ts issueTickets |
| GET /api/v1/events/{uuid}, GET /v1/events/{uuid}/availability, GET /v1/payments/methods, POST /api/v1/auth/login+refresh | registerRealApiDefaults (events.json dung chung voi stub-api.mjs) | event-service.ts, payment-client.ts, api-client.ts |
| POST /v1/orders/{orderId}/issue-tickets (200)         | { order_id, tickets:[{ id, ticket_type_id, status, qr_payload }] }        | payment-client.ts issueTickets |
| GET  /api/v1/events/{uuid} ; GET /v1/events/{uuid}/availability ; GET /v1/payments/methods ; POST /api/v1/auth/login|refresh | registerRealApiDefaults (events.json dung chung voi stub-api.mjs) | event-service.ts, payment-client.ts, api-client.ts |
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

/** Danh sach header cho phep — SAO CHEP TU gateway.go (CORS). Doi o do thi doi o day. */
export const GATEWAY_ALLOW_HEADERS =
  "Authorization, Content-Type, Idempotency-Key, X-Request-Id, Last-Event-ID, X-Queue-Token, If-None-Match";
export const GATEWAY_EXPOSE_HEADERS = "X-Request-Id, Retry-After, ETag, X-Poll-After-Ms";

function corsHeaders(): Record<string, string> {
  return {
    "Access-Control-Allow-Origin": WEB_ORIGIN,
    "Access-Control-Allow-Methods": "GET, POST, PUT, PATCH, DELETE, OPTIONS",
    "Access-Control-Allow-Headers": GATEWAY_ALLOW_HEADERS,
    "Access-Control-Expose-Headers": GATEWAY_EXPOSE_HEADERS,
    "Access-Control-Max-Age": "600",
    Vary: "Origin",
  };
}

const ALLOWED_REQUEST_HEADERS = new Set(GATEWAY_ALLOW_HEADERS.toLowerCase().split(/\s*,\s*/));
/** Header tac gia ma trinh duyet se preflight (khong thuoc nhom CORS-safelisted). */
const AUTHORED_HEADER = /^(authorization|idempotency-key|if-none-match|if-match|last-event-id|x-.+)$/;

function disallowedAuthoredHeaders(headers: Record<string, string>): string[] {
  return Object.keys(headers).filter((h) => AUTHORED_HEADER.test(h) && !ALLOWED_REQUEST_HEADERS.has(h));
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

    const blocked = disallowedAuthoredHeaders(recorded.headers);
    if (blocked.length > 0) {
      const message = `${method} ${request.url()} — header ngoai Access-Control-Allow-Headers cua gateway: ${blocked.join(", ")} (gateway that se chan preflight)`;
      this.violations.push(message);
      await this.fulfillViolation(route, message);
      return;
    }

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

interface FixtureEvent {
  id: string;
  slug: string;
  status: string;
}

/** Du lieu dung chung voi fixtures/stub-api.mjs (phuc vu fetch phia server cua Next). */
export const EVENTS_FIXTURE = eventsFixture as unknown as {
  events: FixtureEvent[];
  availability: Record<string, unknown>;
};

const NOT_FOUND = {
  status: 404,
  contentType: "application/problem+json",
  json: { type: "/problems/not-found", title: "Not Found", status: 404, detail: "not found" },
};

/** Phien dang nhap gia cho luong mua ve (token khong ky, exp xa => web khong tu refresh). */
export const E2E_USER = { id: "11111111-2222-4333-8444-555555555555", role: "buyer", email: "e2e@example.com" };
export const E2E_REFRESH_TOKEN = "e2e-refresh-token";
export const E2E_TIER_ID = "b6f0c2d4-8e13-4a57-9d6c-1a2b3c4d5e6f";
/** Id ve gia lap. Man hinh ket qua hien ma ve = 8 ky tu dau cua ticketId viet hoa (order-result-view). */
export const E2E_TICKET_IDS = ["e2e00001-0000-4000-8000-000000000001", "e2e00002-0000-4000-8000-000000000002"];
export const E2E_TICKET_CODES = E2E_TICKET_IDS.map((id) => id.slice(0, 8).toUpperCase());

export function e2eQrPayload(ticketId: string): string {
  return `QR-${ticketId.slice(0, 8).toUpperCase()}`;
}

export function e2eAccessToken(): string {
  const b64 = (o: unknown) => Buffer.from(JSON.stringify(o)).toString("base64url");
  return `${b64({ alg: "none", typ: "JWT" })}.${b64({ sub: E2E_USER.id, role: E2E_USER.role, ver: true, exp: 4_102_444_800 })}.e2e`;
}

/** Cac moi goi tu trinh duyet cua web that (frontend-real-api): su kien theo id, availability,
 *  cong thanh toan, issue-tickets, login/refresh. Dang ky TRUOC nen test ghi de duoc bang api.on().
 *  Hold/queue/payments/order do buy-flow-helpers dang ky theo tung kich ban. */
export function registerRealApiDefaults(api: ApiMock): void {
  // /events duoc prerender luc build khong co du lieu (khong fetch luc build) nen trinh duyet tu tai danh sach.
  api.on("GET", "/api/v1/events/public", {
    status: 200,
    json: { events: EVENTS_FIXTURE.events.filter((e) => e.status !== "DRAFT") },
  });

  api.on("GET", /^\/api\/v1\/events\/[0-9a-f-]{36}$/, (req) => {
    const id = req.path.split("/").pop();
    const ev = EVENTS_FIXTURE.events.find((e) => e.id === id);
    return ev ? { status: 200, json: ev } : NOT_FOUND;
  });

  api.on("GET", /^\/v1\/events\/[0-9a-f-]{36}\/availability$/, (req) => {
    const av = EVENTS_FIXTURE.availability[req.path.split("/")[3]];
    return av ? { status: 200, json: av } : NOT_FOUND;
  });

  api.on("GET", "/v1/payments/methods", {
    status: 200,
    json: {
      methods: [
        { code: "sandbox", name: "Sandbox", description: "Cổng thanh toán thử nghiệm", enabled: true, icon: "sandbox" },
        { code: "vnpay", name: "VNPay", description: "VNPay", enabled: false, icon: "vnpay" },
      ],
    },
  });

  const identity = () => ({
    status: 200,
    json: {
      id: E2E_USER.id,
      email: E2E_USER.email,
      role: E2E_USER.role,
      created_at: "2026-08-01T00:00:00Z",
      access_token: e2eAccessToken(),
      token: e2eAccessToken(),
      token_type: "Bearer",
      expires_in: 900,
      refresh_token: E2E_REFRESH_TOKEN,
      refresh_expires_in: 604_800,
    },
  });
  api.on("POST", "/api/v1/auth/login", identity);
  api.on("POST", "/api/v1/auth/refresh", identity);

  // POST /v1/orders/{id}/issue-tickets (ticketing ticket_handler): QR payload that, goi lai duoc nhieu lan.
  api.on("POST", /^\/v1\/orders\/[^/]+\/issue-tickets$/, (req) => ({
    status: 200,
    json: {
      order_id: req.path.split("/")[3],
      tickets: E2E_TICKET_IDS.map((id) => ({
        id,
        ticket_type_id: E2E_TIER_ID,
        status: "ISSUED",
        qr_payload: e2eQrPayload(id),
      })),
    },
  }));
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
    registerRealApiDefaults(api);
    await api.install(context);
    await use(api);
    await finalizeApiMock(api, context);
  },
});

export { expect };
