import { describe, expect, it } from "vitest";

import { GET } from "./route";

describe("GET /api/time (EVF-111)", () => {
  it("tra ve status 200 kem header cache edge 1s", async () => {
    const before = Date.now();
    const res = GET();
    const after = Date.now();

    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toContain("application/json");
    expect(res.headers.get("cache-control")).toBe(
      "public, s-maxage=1, stale-while-revalidate=5",
    );

    const data = await res.json();
    expect(typeof data.server_time).toBe("number");
    expect(data.server_time).toBeGreaterThanOrEqual(before);
    expect(data.server_time).toBeLessThanOrEqual(after);

    expect(typeof data.rfc3339).toBe("string");
    expect(new Date(data.rfc3339).getTime()).toBe(data.server_time);
  });
});
