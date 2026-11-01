/** Shared helpers for the frontend-debt-r5 tests (not a test file). Reuses ../debt/harness. */
import { installFetch, json, problem, type Call } from "@/test-utils/fetch-mock";

import { hangUntilAborted } from "../debt/harness";

export const isRefresh = (c: { path: string }) => c.path === "/api/v1/auth/refresh";
export const posts = (calls: Call[]) =>
  calls.filter(isRefresh).map((c) => (c.body as { refresh_token: string }).refresh_token);
export const apiCalls = (calls: Call[]) => calls.filter((c) => !isRefresh(c));
export const bearers = (calls: Call[]) => apiCalls(calls).map((c) => c.headers.get("Authorization"));

export const IDENT = (over: Record<string, unknown> = {}) => ({
  id: "u-1",
  role: "buyer",
  access_token: "new",
  refresh_token: "r2",
  expires_in: 900,
  ...over,
});

/** API calls answer 200 only for `Bearer valid()`, else 401 token-expired. The refresh handler is swappable;
 *  default = hang until aborted. `controlled()` + `answer()` resolve the next refresh POST by hand. */
export function apiServer(valid: () => string = () => "acc-never") {
  let handler: ((c: Call) => Response | Promise<Response>) | null = null;
  let pending: ((r: Response) => void) | null = null;
  const state = installFetch(async (c) => {
    if (isRefresh(c)) return handler ? handler(c) : hangUntilAborted(c.init);
    return c.headers.get("Authorization") === `Bearer ${valid()}`
      ? json({ ok: true })
      : problem(401, "/problems/token-expired");
  });
  return {
    ...state,
    setRefresh: (h: (c: Call) => Response | Promise<Response>) => (handler = h),
    controlled: () => {
      handler = () => new Promise<Response>((r) => (pending = r));
    },
    answer: (r: Response) => pending?.(r),
  };
}

export function jwtExpIn(seconds: number): string {
  const payload = btoa(JSON.stringify({ exp: Math.floor(Date.now() / 1000) + seconds }));
  return `h.${payload}.s`;
}
