/** F-5 (X-3): both clearSession guards (120 s abort, POST 401) also require the same user id. */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { installFetch, json, problem, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

import { fakeLocks, hangUntilAborted, installLocks, newTab, outcome, removeLocks, seedUser } from "./harness";

beforeEach(setupEnv);
afterEach(() => {
  vi.useRealTimers();
  removeLocks();
  resetEnv();
});

describe("F-5 clearSession guards compare the user id", () => {
  it("120 s abort: u-2 stored with the SAME refresh string \"r1\" is not cleared", async () => {
    vi.useFakeTimers();
    installLocks(fakeLocks());
    installFetch((c) => (c.path === "/api/v1/auth/refresh" ? hangUntilAborted(c.init) : problem(401, "/problems/token-expired")));
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(60_000);
    seedUser("u-2", "acc-u2", "r1");
    await vi.advanceTimersByTimeAsync(61_000);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(localStorage.getItem("ef_token")).toBe("acc-u2");
    expect(JSON.parse(localStorage.getItem("ef_user")!).id).toBe("u-2");
  });

  it("POST answers 401 after the switch: u-2 with the SAME refresh string \"r1\" is not cleared", async () => {
    vi.useFakeTimers();
    installLocks(fakeLocks());
    let answer!: () => void;
    installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        return new Promise<Response>((r) => (answer = () => r(problem(401, "/problems/invalid-token"))));
      }
      return problem(401, "/problems/token-expired");
    });
    seedUser("u-1", "old", "r1");
    const A = await newTab();
    const a = outcome("A", A.api.apiFetch("/v1/orders/a"));
    await vi.advanceTimersByTimeAsync(10_000);
    seedUser("u-2", "acc-u2", "r1");
    answer();
    await vi.advanceTimersByTimeAsync(1_000);
    expect(await a).toBe("A:AUTH_REQUIRED");
    expect(localStorage.getItem("ef_token")).toBe("acc-u2");
    expect(localStorage.getItem("ef_refresh")).toBe("r1");
    expect(JSON.parse(localStorage.getItem("ef_user")!).id).toBe("u-2");
  });
});
