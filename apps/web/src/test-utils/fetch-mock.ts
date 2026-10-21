/**
 * Helper dung chung cho cac *.contract.test.ts(x) cua frontend-real-api.
 * Mock fetch o bien, ghi lai moi request de test assert URL / method / header / body.
 */
import { expect, vi } from "vitest";

export const BASE = "http://gw.test";

export interface Call {
  url: string;
  path: string;
  search: string;
  method: string;
  headers: Headers;
  body: unknown;
  init: RequestInit;
}

export type Handler = (c: Call) => Response | Promise<Response>;

export function installFetch(handler: Handler) {
  const calls: Call[] = [];
  const fn = vi.fn(async (input: unknown, init?: RequestInit) => {
    const url = typeof input === "string" ? input : input instanceof URL ? input.href : (input as Request).url;
    let body: unknown = init?.body;
    if (typeof body === "string") {
      try {
        body = JSON.parse(body);
      } catch {
        /* giu nguyen chuoi */
      }
    }
    const u = new URL(url, "http://x");
    const call: Call = {
      url,
      path: u.pathname,
      search: u.search,
      method: (init?.method ?? "GET").toUpperCase(),
      headers: new Headers(init?.headers),
      body,
      init: init ?? {},
    };
    calls.push(call);
    return handler(call);
  });
  vi.stubGlobal("fetch", fn);
  return { calls, fn };
}

export const json = (body: unknown, status = 200, headers: Record<string, string> = {}) =>
  new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json", ...headers } });

export const problem = (status: number, type: string, detail = "chi tiet loi") =>
  json({ type, title: "t", status, detail }, status, { "Content-Type": "application/problem+json" });

export const noContent = (status = 204) => new Response(null, { status });

/** Gieo phien dang nhap thang vao localStorage (khong phu thuoc lib/session). */
export function seedSession(access = "acc-1", refresh: string | null = "ref-1", role = "buyer") {
  localStorage.setItem("ef_token", access);
  if (refresh) localStorage.setItem("ef_refresh", refresh);
  localStorage.setItem("ef_user", JSON.stringify({ id: "u-1", role }));
}

export function resetEnv() {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  localStorage.clear();
  sessionStorage.clear();
}

export function setupEnv() {
  vi.stubEnv("NEXT_PUBLIC_API_BASE", BASE);
  localStorage.clear();
  sessionStorage.clear();
}

/** Khong header nao bat dau bang x-identity-, khong credentials include. */
export function expectNoIdentityOrCredentials(c: Call) {
  expect(c.init.credentials).not.toBe("include");
  for (const name of c.headers.keys()) {
    expect(name.toLowerCase().startsWith("x-identity-")).toBe(false);
  }
}

/** Ma loi cua mot error: `code` (ApiError moi) hoac `type` (HoldClientError / PaymentClientError cu). */
export function codeOf(e: unknown): unknown {
  const o = e as { code?: unknown; type?: unknown };
  return o?.code ?? o?.type;
}

export async function rejection(p: Promise<unknown>): Promise<unknown> {
  try {
    await p;
  } catch (e) {
    return e;
  }
  return undefined;
}
