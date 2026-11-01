/**
 * ops-warroom AC-22: AUTH_REQUIRED from apiFetch (session gone / refresh failed) => OpsServiceError(401).
 * ASSUMPTION: getOpsWarRoom reaches the network only through `apiFetch` from "@/lib/api-client".
 */
import { describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api-client", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api-client")>();
  return {
    ...actual,
    apiFetch: vi.fn(async () => {
      throw new actual.ApiError("AUTH_REQUIRED", 401, {}, "login");
    }),
  };
});

import { OpsServiceError, getOpsWarRoom } from "@/lib/ops-service";

describe("getOpsWarRoom - AUTH_REQUIRED (AC-22)", () => {
  it("becomes OpsServiceError(401) so the route guard can send the user to login", async () => {
    const err = await getOpsWarRoom("0b6d3b52-6f6e-4f0e-9d36-2a2f1c0a9a11").catch((e: unknown) => e);
    expect(err).toBeInstanceOf(OpsServiceError);
    expect((err as OpsServiceError).status).toBe(401);
  });
});
