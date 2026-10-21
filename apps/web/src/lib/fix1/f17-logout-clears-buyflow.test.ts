/** Regression F-17 (S-3): logout also clears buy-flow sessionStorage. */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { logout } from "@/lib/api/auth";
import { installFetch, noContent, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(setupEnv);
afterEach(resetEnv);

describe("F-17", () => {
  it("removes order cache, queue tokens, active hold, issue and payment keys but keeps unrelated keys", async () => {
    seedSession("a", "r");
    installFetch(() => noContent());
    const keys = [
      "evenflow_order_o1",
      "ef:queue:token:e1",
      "evenflow_hold_e1",
      "ef:issue:o1",
      "ef:pay:o1",
      "ef:hold:e1:t1:2",
    ];
    for (const k of keys) sessionStorage.setItem(k, "x");
    sessionStorage.setItem("unrelated", "keep");
    await logout();
    for (const k of keys) expect(sessionStorage.getItem(k), k).toBeNull();
    expect(sessionStorage.getItem("unrelated")).toBe("keep");
  });
});
