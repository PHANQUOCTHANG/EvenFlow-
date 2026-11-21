/** ops-warroom fix round 1: getRealtimeSales maps a plain 401 to AUTH_REQUIRED (R-5, X-7) and a malformed 200 body to
 * an ApiError instead of a raw TypeError (X-6). */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { ApiError } from "@/lib/api-client";
import { getRealtimeSales } from "@/lib/organizer-service";
import { installFetch, json, problem, rejection, resetEnv, seedSession, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession("acc-org", "ref-org", "organizer");
});
afterEach(resetEnv);

const EV = "0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11";
const P = `/v1/organizer/events/${EV}/sales`;

describe("getRealtimeSales error mapping (fix round 1)", () => {
  it("a plain 401 rejects with ApiError AUTH_REQUIRED", async () => {
    installFetch((c) => (c.path === P ? problem(401, "/problems/unauthorized") : problem(404, "/x")));
    const e = await rejection(getRealtimeSales(EV));
    expect(e).toBeInstanceOf(ApiError);
    expect((e as ApiError).code).toBe("AUTH_REQUIRED");
  });

  it.each([[{ event_id: EV, generated_at: "2026-11-01T10:00:00Z", tiers: [] }], [null], [{ totals: 5 }]])(
    "a malformed 200 body %j rejects with ApiError, not a TypeError",
    async (body) => {
      installFetch((c) => (c.path === P ? json(body) : problem(404, "/x")));
      const e = await rejection(getRealtimeSales(EV));
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).message).toBe("Không tải được doanh số, vui lòng thử lại.");
    },
  );
});
