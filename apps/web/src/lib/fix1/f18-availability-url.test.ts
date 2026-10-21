/** Regression F-18 (S-7): availability path encodes the event id. */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { getEventById } from "@/lib/event-service";
import { installFetch, json, resetEnv, setupEnv } from "@/test-utils/fetch-mock";

beforeEach(setupEnv);
afterEach(resetEnv);

describe("F-18", () => {
  it("encodeURIComponent on wire.id in /availability", async () => {
    const { calls } = installFetch((c) =>
      c.path.endsWith("/availability")
        ? json({ tiers: [] })
        : json({
            id: "a/b c", slug: "s", title: "T", starts_at: "2026-12-01T00:00:00Z", sale_start_at: "2026-11-01T00:00:00Z",
            status: "ON_SALE", tiers: [],
          }),
    );
    await getEventById("a%2Fb%20c");
    const av = calls.find((c) => c.url.includes("/availability"));
    expect(av?.url).toContain("/v1/events/a%2Fb%20c/availability");
  });
});
