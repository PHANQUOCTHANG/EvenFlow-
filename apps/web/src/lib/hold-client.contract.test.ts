/**
 * Contract test: lib/hold-client.ts (AC-3, AC-10, AC-11).
 * Gia dinh ten truong loi: ma loi doc tu `error.code ?? error.type` (HoldClientError hien tai dung `type`;
 * ApiError moi dung `code`). Test chap nhan ca hai.
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { cancelHold, getActiveHold, requestHold, saveActiveHold } from "@/lib/hold-client";
import {
  BASE,
  codeOf,
  expectNoIdentityOrCredentials,
  installFetch,
  json,
  noContent,
  problem,
  rejection,
  resetEnv,
  seedSession,
  setupEnv,
} from "@/test-utils/fetch-mock";

beforeEach(() => {
  setupEnv();
  seedSession("acc-1", "ref-1");
});
afterEach(resetEnv);

const PARAMS = { eventId: "evt-1", ticketTypeId: "tier-1", quantity: 2, queueToken: "qt-abc" };
const OK = { order_id: "ord-1", hold_id: "hld-1", expires_at: "2026-11-01T10:10:00Z" };

describe("requestHold: request (AC-3, AC-10)", () => {
  it("POST {base}/v1/events/{id}/holds voi Idempotency-Key, X-Queue-Token, Bearer; body {ticket_type_id, quantity}", async () => {
    const { calls } = installFetch(() => json(OK));
    await requestHold({ ...PARAMS, idempotencyKey: "idem-1" });
    expect(calls).toHaveLength(1);
    const c = calls[0];
    expect(c.url).toBe(`${BASE}/v1/events/evt-1/holds`);
    expect(c.method).toBe("POST");
    expect(c.headers.get("Idempotency-Key")).toBe("idem-1");
    expect(c.headers.get("X-Queue-Token")).toBe("qt-abc");
    expect(c.headers.get("Authorization")).toBe("Bearer acc-1");
    expect(c.headers.get("Content-Type")).toContain("application/json");
    expect(c.body).toEqual({ ticket_type_id: "tier-1", quantity: 2 });
  });

  it("khong bao gio gui X-Identity-Id (ke ca khi caller van truyen identityId cu) va khong credentials include", async () => {
    const { calls } = installFetch(() => json(OK));
    await requestHold({ ...PARAMS, identityId: "guest-user" } as never);
    expect(calls[0].headers.has("X-Identity-Id")).toBe(false);
    expectNoIdentityOrCredentials(calls[0]);
  });

  it("tu sinh Idempotency-Key UUID neu caller khong truyen", async () => {
    const { calls } = installFetch(() => json(OK));
    await requestHold(PARAMS);
    expect(calls[0].headers.get("Idempotency-Key")).toMatch(/^[0-9a-f-]{36}$/i);
  });
});

describe("requestHold: thanh cong (AC-10)", () => {
  it.each([200, 201])("HTTP %i la thanh cong, tra {orderId, holdId, expiresAt}", async (status) => {
    installFetch(() => json(OK, status));
    const r = await requestHold(PARAMS);
    expect(r).toMatchObject({
      orderId: "ord-1",
      holdId: "hld-1",
      expiresAt: "2026-11-01T10:10:00Z",
      ticketTypeId: "tier-1",
      quantity: 2,
    });
  });
});

describe("requestHold: anh xa loi (AC-10)", () => {
  it.each([
    [403, "/problems/queue-token-expired", "QUEUE_TOKEN_INVALID"],
    [403, "/problems/queue-token-invalid", "QUEUE_TOKEN_INVALID"],
    [403, "/problems/queue-token-mismatch", "QUEUE_TOKEN_INVALID"],
    [422, "/problems/per-identity-limit", "PER_IDENTITY_LIMIT"],
    [422, "/problems/quantity-not-allowed", "QUANTITY_NOT_ALLOWED"],
    [403, "about:blank", "NOT_ADMITTED"],
    [403, "/problems/forbidden", "FORBIDDEN"],
    [409, "about:blank", "SOLD_OUT"],
  ])("%i %s -> %s", async (status, type, code) => {
    installFetch(() => problem(status, type));
    const err = await rejection(requestHold(PARAMS));
    expect(err).toBeInstanceOf(Error);
    expect(codeOf(err)).toBe(code);
    expect((err as { status?: number }).status).toBe(status);
  });

  it("MUTATION: per-identity-limit KHONG duoc map sang QUANTITY_NOT_ALLOWED va nguoc lai", async () => {
    installFetch(() => problem(422, "/problems/per-identity-limit"));
    const a = codeOf(await rejection(requestHold(PARAMS)));
    expect(a).toBe("PER_IDENTITY_LIMIT");
    expect(a).not.toBe("QUANTITY_NOT_ALLOWED");
    installFetch(() => problem(422, "/problems/quantity-not-allowed"));
    const b = codeOf(await rejection(requestHold(PARAMS)));
    expect(b).toBe("QUANTITY_NOT_ALLOWED");
    expect(b).not.toBe("PER_IDENTITY_LIMIT");
    expect(a).not.toBe(b);
  });

  it("409 idempotency-in-progress KHONG phai het ve -> AMBIGUOUS", async () => {
    installFetch(() => problem(409, "urn:eventflow:problem:idempotency-in-progress"));
    expect(codeOf(await rejection(requestHold(PARAMS)))).toBe("AMBIGUOUS");
  });

  it("503 -> AMBIGUOUS; loi mang -> AMBIGUOUS", async () => {
    installFetch(() => problem(503, "about:blank"));
    expect(codeOf(await rejection(requestHold(PARAMS)))).toBe("AMBIGUOUS");
    installFetch(() => {
      throw new TypeError("Failed to fetch");
    });
    expect(codeOf(await rejection(requestHold(PARAMS)))).toBe("AMBIGUOUS");
  });
});

describe("requestHold + refresh (AC-4)", () => {
  it("401 token-expired -> refresh 1 lan roi retry voi CUNG Idempotency-Key va X-Queue-Token", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        return json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2" });
      }
      return c.headers.get("Authorization") === "Bearer new"
        ? json(OK)
        : problem(401, "/problems/token-expired");
    });
    const r = await requestHold({ ...PARAMS, idempotencyKey: "idem-7" });
    expect(r.holdId).toBe("hld-1");
    const holds = calls.filter((c) => c.path === "/v1/events/evt-1/holds");
    expect(holds).toHaveLength(2);
    expect(holds[1].headers.get("Idempotency-Key")).toBe("idem-7");
    expect(holds[1].headers.get("X-Queue-Token")).toBe("qt-abc");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
  });

  it("queue-token het han (401) KHONG refresh, map QUEUE_TOKEN_INVALID", async () => {
    const { calls } = installFetch(() => problem(401, "/problems/queue-token-expired"));
    const err = await rejection(requestHold(PARAMS));
    expect(codeOf(err)).toBe("QUEUE_TOKEN_INVALID");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
    expect(localStorage.getItem("ef_token")).toBe("acc-1");
  });
});

describe("cancelHold (AC-11)", () => {
  const hold = (holdId: string) => ({
    orderId: "ord-1",
    holdId,
    expiresAt: "2026-11-01T10:10:00Z",
    ticketTypeId: "tier-1",
    quantity: 2,
  });

  it("DELETE {base}/v1/holds/{holdId} voi Idempotency-Key va Bearer", async () => {
    const { calls } = installFetch(() => noContent());
    await cancelHold("hld-1");
    const c = calls[0];
    expect(c.url).toBe(`${BASE}/v1/holds/hld-1`);
    expect(c.method).toBe("DELETE");
    expect(c.headers.get("Idempotency-Key")).toMatch(/^[0-9a-f-]{36}$/i);
    expect(c.headers.get("Authorization")).toBe("Bearer acc-1");
    expectNoIdentityOrCredentials(c);
  });

  it.each([204, 404])("HTTP %i -> resolve va xoa hold luu cuc bo", async (status) => {
    saveActiveHold("evt-1", hold("hld-1"));
    installFetch(() => (status === 204 ? noContent() : problem(404, "/problems/not-found")));
    await expect(cancelHold("hld-1")).resolves.toBeUndefined();
    expect(getActiveHold("evt-1")).toBeNull();
  });

  it("khong xoa hold cua su kien khac (holdId khac)", async () => {
    saveActiveHold("evt-2", hold("hld-other"));
    installFetch(() => noContent());
    await cancelHold("hld-1");
    expect(getActiveHold("evt-2")?.holdId).toBe("hld-other");
  });

  it("409 hold-not-cancellable -> reject, giu hold cuc bo", async () => {
    saveActiveHold("evt-1", hold("hld-1"));
    installFetch(() => problem(409, "/problems/hold-not-cancellable"));
    await expect(cancelHold("hld-1")).rejects.toBeInstanceOf(Error);
    expect(getActiveHold("evt-1")?.holdId).toBe("hld-1");
  });
});
