/**
 * Contract test: queue-client / queue-status / sse qua phien dang nhap (AC-3, AC-5, AC-9).
 * Gia dinh: QueueError them truong `code` (chuoi, khac nhau cho otp-required va device-required);
 * test KHONG ep gia tri cu the, chi ep: ton tai, khac nhau, khac loi 403 chung.
 */
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { joinQueue, QueueError } from "@/lib/queue-client";
import { fetchQueueStatus } from "@/lib/queue-status";
import { connectSse } from "@/lib/sse";
import {
  BASE,
  expectNoIdentityOrCredentials,
  installFetch,
  json,
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

const JOIN_OK = { state: "QUEUED", queue_token: "qt-1", rank: 5, is_new: true, poll_after_ms: 3000 };
const STATUS_OK = { state: "QUEUED", rank: 3, queue_depth: 10, admit_rate: 5, eta_seconds: 12, poll_after_ms: 3000 };

describe("joinQueue (AC-3, AC-9)", () => {
  it("POST {base}/v1/events/{id}/queue/join voi Bearer, khong credentials include, khong X-Identity-*", async () => {
    const { calls } = installFetch(() => json(JOIN_OK));
    await joinQueue("evt-1", { fp: "x" }, { jitter: false });
    const c = calls[0];
    expect(c.url).toBe(`${BASE}/v1/events/evt-1/queue/join`);
    expect(c.method).toBe("POST");
    expect(c.headers.get("Authorization")).toBe("Bearer acc-1");
    expect(c.body).toEqual({ signals: { fp: "x" } });
    expectNoIdentityOrCredentials(c);
  });

  it("khong phien -> khong gui Authorization", async () => {
    localStorage.clear();
    const { calls } = installFetch(() => json(JOIN_OK));
    await joinQueue("evt-1", {}, { jitter: false });
    expect(calls[0].headers.has("Authorization")).toBe(false);
  });

  it("401 token-expired -> refresh roi join thanh cong", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        return json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2" });
      }
      return c.headers.get("Authorization") === "Bearer new" ? json(JOIN_OK) : problem(401, "/problems/token-expired");
    });
    const r = await joinQueue("evt-1", {}, { jitter: false });
    expect(r.queue_token).toBe("qt-1");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
  });

  it("403 otp-required va device-required cho ma loi RIENG BIET, khac 403 chung", async () => {
    const codes: unknown[] = [];
    for (const [status, type] of [
      [403, "/problems/otp-required"],
      [403, "/problems/device-required"],
      [403, "/problems/forbidden"],
    ] as const) {
      installFetch(() => problem(status, type));
      const err = await rejection(joinQueue("evt-1", {}, { jitter: false }));
      expect(err).toBeInstanceOf(QueueError);
      expect((err as QueueError).status).toBe(403);
      codes.push((err as { code?: unknown }).code);
    }
    const [otp, device, generic] = codes;
    expect(typeof otp).toBe("string");
    expect(typeof device).toBe("string");
    expect(otp).not.toBe(device);
    expect(otp).not.toBe(generic);
    expect(device).not.toBe(generic);
  });

  it("SOLD_OUT join tra queue_token rong (giu hanh vi EV-146)", async () => {
    installFetch(() => json({ state: "SOLD_OUT", queue_token: "", rank: -1, is_new: false, poll_after_ms: 0 }));
    const r = await joinQueue("evt-1", {}, { jitter: false });
    expect(r.state).toBe("SOLD_OUT");
    expect(r.queue_token).toBe("");
  });
});

describe("fetchQueueStatus (AC-3, AC-5, AC-9)", () => {
  it("GET {base}/v1/events/{id}/queue/status voi Bearer + X-Queue-Token + If-None-Match, khong credentials include", async () => {
    const { calls } = installFetch(() => json(STATUS_OK, 200, { ETag: '"e1"' }));
    await fetchQueueStatus("evt-1", "qt-1", { etag: '"e0"' });
    const c = calls[0];
    expect(c.url).toBe(`${BASE}/v1/events/evt-1/queue/status`);
    expect(c.method).toBe("GET");
    expect(c.headers.get("Authorization")).toBe("Bearer acc-1");
    expect(c.headers.get("X-Queue-Token")).toBe("qt-1");
    expect(c.headers.get("If-None-Match")).toBe('"e0"');
    expectNoIdentityOrCredentials(c);
  });

  it("304 van la not-modified (khong bi apiFetch nuot)", async () => {
    installFetch(() => new Response(null, { status: 304, headers: { "X-Poll-After-Ms": "4000", ETag: '"e1"' } }));
    const r = await fetchQueueStatus("evt-1", "qt-1", { etag: '"e1"' });
    expect(r.kind).toBe("not-modified");
  });

  it.each([
    [401, "/problems/queue-token-expired"],
    [401, "/problems/queue-token-invalid"],
    [403, "/problems/queue-token-mismatch"],
  ])("%i %s -> KHONG refresh, giu phien, nem QueueError", async (status, type) => {
    const { calls } = installFetch(() => problem(status, type));
    const err = await rejection(fetchQueueStatus("evt-1", "qt-1"));
    expect(err).toBeInstanceOf(QueueError);
    expect((err as QueueError).status).toBe(status);
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(0);
    expect(localStorage.getItem("ef_token")).toBe("acc-1");
    expect(localStorage.getItem("ef_refresh")).toBe("ref-1");
  });

  it("401 token-expired (access token) -> refresh 1 lan roi poll lai thanh cong", async () => {
    seedSession("old", "r1");
    const { calls } = installFetch((c) => {
      if (c.path === "/api/v1/auth/refresh") {
        return json({ id: "u-1", role: "buyer", access_token: "new", refresh_token: "r2" });
      }
      return c.headers.get("Authorization") === "Bearer new" ? json(STATUS_OK) : problem(401, "/problems/token-expired");
    });
    const r = await fetchQueueStatus("evt-1", "qt-1");
    expect(r.kind).toBe("updated");
    expect(calls.filter((c) => c.path === "/api/v1/auth/refresh")).toHaveLength(1);
    const polls = calls.filter((c) => c.path === "/v1/events/evt-1/queue/status");
    expect(polls[1].headers.get("X-Queue-Token")).toBe("qt-1");
  });
});

describe("connectSse (AC-9)", () => {
  // url tuong doi: dung ca khi connectSse goi fetch truc tiep lan khi di qua apiFetch (tu them apiBase()).
  it("gui Bearer, khong credentials include, giu header cua caller (X-Queue-Token)", async () => {
    const { calls } = installFetch(
      () => new Response(new ReadableStream({ start: (c) => c.close() }), { status: 200 }),
    );
    const conn = connectSse({
      url: "/v1/events/evt-1/queue/stream",
      headers: { "X-Queue-Token": "qt-1" },
      onMessage: () => undefined,
      maxRetries: 0,
    });
    await conn.done;
    const c = calls[0];
    expect(c.path).toBe("/v1/events/evt-1/queue/stream");
    expect(c.headers.get("Authorization")).toBe("Bearer acc-1");
    expect(c.headers.get("X-Queue-Token")).toBe("qt-1");
    expect(c.headers.get("Accept")).toBe("text/event-stream");
    expectNoIdentityOrCredentials(c);
  });
});
