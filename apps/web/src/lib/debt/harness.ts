/** Shared helpers for the frontend-debt-r4 tests (not a test file): FIFO Web Lock stub, tab factory, outcome helper. */
import { vi } from "vitest";

/** FIFO lock manager (no `steal`); an aborted pending request never receives the lock. Mirrors fix2/g1-lock-refresh. */
export function fakeLocks() {
  let held = false;
  const queue: Array<() => void> = [];
  const next = () => {
    held = false;
    queue.shift()?.();
  };
  const request = (_name: string, a: unknown, b?: unknown) => {
    const cb = (typeof a === "function" ? a : b) as () => Promise<unknown>;
    const opts = (typeof a === "function" ? {} : a) as { signal?: AbortSignal };
    return new Promise<unknown>((resolve, reject) => {
      const run = () => {
        held = true;
        Promise.resolve()
          .then(cb)
          .then(
            (v) => {
              resolve(v);
              next();
            },
            (e) => {
              reject(e);
              next();
            },
          );
      };
      if (!held) {
        run();
        return;
      }
      const entry = () => run();
      queue.push(entry);
      opts.signal?.addEventListener("abort", () => {
        const i = queue.indexOf(entry);
        if (i >= 0) {
          queue.splice(i, 1);
          reject(new DOMException("aborted", "AbortError"));
        }
      });
    });
  };
  return { request, isHeld: () => held };
}

export function installLocks(locks: unknown) {
  Object.defineProperty(navigator, "locks", { value: locks, configurable: true });
}

export function removeLocks() {
  Reflect.deleteProperty(navigator, "locks");
}

/** A fresh module graph = a new "tab" (own in-flight refresh state). localStorage is shared, as in a real browser. */
export async function newTab() {
  vi.resetModules();
  const api = await import("@/lib/api-client");
  const sess = await import("@/lib/session");
  return { api, sess };
}

export const outcome = (tag: string, p: Promise<Response>) =>
  p.then(
    (r) => `${tag}:${r.status}`,
    (e: { code?: string }) => `${tag}:${e.code}`,
  );

/** AbortSignal.timeout runs on a native timer that vi fake timers do not control; route it through the faked setTimeout
 *  so a 120 s abort can be observed with advanceTimersByTimeAsync. Call AFTER vi.useFakeTimers(). */
export function routeAbortSignalTimeoutThroughFakeTimers() {
  const original = AbortSignal.timeout;
  AbortSignal.timeout = (ms: number) => {
    const c = new AbortController();
    setTimeout(() => c.abort(new DOMException("The operation timed out.", "TimeoutError")), ms);
    return c.signal;
  };
  return () => {
    AbortSignal.timeout = original;
  };
}

/** A fetch body that never answers but rejects when the request signal aborts (like a real fetch). */
export function hangUntilAborted(init: RequestInit): Promise<Response> {
  return new Promise<Response>((_resolve, reject) => {
    const s = init.signal;
    if (!s) return;
    const fail = () => reject(s.reason ?? new DOMException("aborted", "AbortError"));
    if (s.aborted) fail();
    else s.addEventListener("abort", fail);
  });
}

export function seedUser(id: string, access: string, refresh: string | null, store: Storage = localStorage) {
  store.setItem("ef_token", access);
  if (refresh) store.setItem("ef_refresh", refresh);
  else store.removeItem("ef_refresh");
  store.setItem("ef_user", JSON.stringify({ id, role: "buyer" }));
}

export const BUY_FLOW_KEYS = [
  "ef:queue:token:e1",
  "evenflow_hold_e1",
  "evenflow_order_o1",
  "ef:issue:o1",
  "ef:pay:o1:abc",
  "ef:hold:e1",
];

export function seedBuyFlow(store: Storage = sessionStorage) {
  for (const k of BUY_FLOW_KEYS) store.setItem(k, "v-" + k);
}

export function buyFlowRemaining(store: Storage = sessionStorage): string[] {
  return BUY_FLOW_KEYS.filter((k) => store.getItem(k) !== null);
}
