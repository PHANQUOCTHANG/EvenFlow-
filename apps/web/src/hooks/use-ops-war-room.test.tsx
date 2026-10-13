/**
 * Test cho useOpsWarRoom — polling an toàn cho war room (EV-187, AC-2, AC-9,
 * plan §1.2 + §6 M-1/M-6).
 *
 * Dùng fake timer (advanceTimersByTimeAsync) + fetcher/now tiêm vào, không mock module.
 * Ngữ nghĩa đã chốt (plan §6 M-6):
 *   - có initialSnapshot → lần fetch đầu sau intervalMs; không có → fetch ngay khi mount;
 *   - lịch kế tiếp = intervalMs tính từ lúc request trước XONG (không chồng request);
 *   - refresh() khi đang có request → trả CÙNG promise, không gọi fetcher lần hai;
 *   - 403 → dừng polling; lỗi khác → giữ snapshot, isStale, vẫn poll;
 *   - ageMs = null tới khi mount, sau đó max(0, now - generatedAt);
 *   - an toàn với StrictMode.
 */
import { act, cleanup, renderHook } from "@testing-library/react";
import { StrictMode, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  OPS_POLL_INTERVAL_MS,
  __setOpsSampleOverride,
  getOpsWarRoom,
  type OpsWarRoomSnapshot,
} from "@/lib/ops-service";

import { useOpsWarRoom } from "./use-ops-war-room";

const EVENT_ID = "evt-tech-summit-2026";
const BASE_MS = Date.parse("2026-10-13T03:00:00.000Z");

function makeSnapshot(
  generatedAtMs: number,
  overrides: Partial<OpsWarRoomSnapshot> = {},
): OpsWarRoomSnapshot {
  return {
    eventId: EVENT_ID,
    eventTitle: "Vietnam Tech Summit 2026",
    saleState: "ON_SALE",
    generatedAt: new Date(generatedAtMs).toISOString(),
    funnel: { lobby: 911, queued: 822, admitted: 733, holding: 644, paid: 555, issued: 466 },
    admission: { ratePerSec: 37, mode: "AUTO", queueDepth: 489 },
    inventory: [
      { tierId: "t1", name: "Standard Pass", quota: 300, held: 21, paid: 102, issued: 93, available: 84 },
    ],
    ai: { mode: "NORMAL", manualOverride: false, chatQueueLagSeconds: 17, budgetUsedRatio: 0.63 },
    sources: { waitingroom: "ok", ticketing: "ok", ai: "ok" },
    isSample: true,
    ...overrides,
  };
}

/** Promise mà test tự quyết khi nào resolve/reject. */
function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

/** Lấy một OpsServiceError thật với status mong muốn qua hook test của service
 *  (hợp đồng không chốt chữ ký constructor của OpsServiceError). */
async function realOpsError(status: number): Promise<Error> {
  __setOpsSampleOverride(EVENT_ID, { error: status });
  try {
    await getOpsWarRoom(EVENT_ID);
  } catch (err) {
    return err as Error;
  } finally {
    __setOpsSampleOverride(EVENT_ID, null);
  }
  throw new Error(`Ky vong getOpsWarRoom nem loi ${status}`);
}

async function advance(ms: number) {
  await act(async () => {
    await vi.advanceTimersByTimeAsync(ms);
  });
}

async function flush() {
  await advance(0);
}

let clock: number;
const now = () => clock;

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(BASE_MS);
  clock = BASE_MS;
});

afterEach(() => {
  cleanup();
  __setOpsSampleOverride(EVENT_ID, null);
  vi.restoreAllMocks();
  vi.useRealTimers();
});

describe("useOpsWarRoom — khởi tạo", () => {
  it("dùng initialSnapshot ngay, không có lỗi, không cũ", () => {
    const initial = makeSnapshot(clock);
    const fetcher = vi.fn(() => new Promise<OpsWarRoomSnapshot>(() => {}));

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: initial, fetcher, now }),
    );

    expect(result.current.snapshot).toEqual(initial);
    expect(result.current.error).toBeNull();
    expect(result.current.isStale).toBe(false);
  });

  it("ageMs = null ở lần render đầu (giống server), là số sau khi mount (M-1)", async () => {
    const fetcher = vi.fn(() => new Promise<OpsWarRoomSnapshot>(() => {}));
    const seen: Array<number | null> = [];

    const { result } = renderHook(() => {
      const r = useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now });
      seen.push(r.ageMs);
      return r;
    });
    await flush();

    expect(seen[0]).toBeNull();
    expect(result.current.ageMs).toBe(0);
  });

  it("có initialSnapshot thì KHÔNG fetch khi mount, lần đầu đúng sau intervalMs (M-6)", async () => {
    const fetcher = vi.fn(async () => makeSnapshot(clock));

    renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );
    await flush();
    expect(fetcher).not.toHaveBeenCalled();

    await advance(1999);
    expect(fetcher).not.toHaveBeenCalled();

    await advance(1);
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher).toHaveBeenCalledWith(EVENT_ID);
  });

  it("không có initialSnapshot thì fetch ngay khi mount (M-6)", async () => {
    const fresh = makeSnapshot(clock);
    const fetcher = vi.fn(async () => fresh);

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: null, fetcher, now, intervalMs: 2000 }),
    );
    await flush();

    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(fetcher).toHaveBeenCalledWith(EVENT_ID);
    expect(result.current.snapshot).toEqual(fresh);
    expect(result.current.lastSuccessAt).not.toBeNull();
  });
});

describe("useOpsWarRoom — polling theo intervalMs (AC-2)", () => {
  it("gọi fetcher mỗi intervalMs", async () => {
    const fetcher = vi.fn(async () => makeSnapshot(clock));

    renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );

    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(1);

    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(2);

    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(3);
    expect(fetcher).toHaveBeenLastCalledWith(EVENT_ID);
  });

  it("mặc định intervalMs = OPS_POLL_INTERVAL_MS (2000ms)", async () => {
    const fetcher = vi.fn(async () => makeSnapshot(clock));

    renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now }),
    );

    await advance(OPS_POLL_INTERVAL_MS - 1);
    expect(fetcher).not.toHaveBeenCalled();

    await advance(1);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("snapshot mới từ fetcher thay snapshot cũ", async () => {
    const next = makeSnapshot(clock, {
      funnel: { lobby: 1, queued: 2, admitted: 3, holding: 4, paid: 5, issued: 6 },
    });
    const fetcher = vi.fn(async () => next);

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );

    await advance(2000);
    expect(result.current.snapshot?.funnel).toEqual(next.funnel);
    expect(result.current.error).toBeNull();
  });

  it("StrictMode: effect chạy hai lần không tạo hai timer", async () => {
    const fetcher = vi.fn(async () => makeSnapshot(clock));
    const wrapper = ({ children }: { children: ReactNode }) => <StrictMode>{children}</StrictMode>;

    renderHook(
      () =>
        useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
      { wrapper },
    );

    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(1);

    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("StrictMode không có initialSnapshot: đúng MỘT lần fetch khi mount, một lịch poll (§7 t-1)", async () => {
    const fetcher = vi.fn(async () => makeSnapshot(clock));
    const wrapper = ({ children }: { children: ReactNode }) => <StrictMode>{children}</StrictMode>;

    renderHook(
      () => useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: null, fetcher, now, intervalMs: 2000 }),
      { wrapper },
    );
    await flush();
    expect(fetcher).toHaveBeenCalledTimes(1);

    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
});

describe("useOpsWarRoom — bổ sung sau code review (plan §7)", () => {
  it("refresh() khi poll tự động đang treo: isRefreshing = true tới khi request đó xong (m-3)", async () => {
    const d = deferred<OpsWarRoomSnapshot>();
    const fetcher = vi.fn(() => d.promise);

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );
    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(result.current.isRefreshing).toBe(false);

    let p!: Promise<void>;
    await act(async () => {
      p = result.current.refresh();
      await Promise.resolve();
    });
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(result.current.isRefreshing).toBe(true);

    await act(async () => {
      d.resolve(makeSnapshot(clock));
      await p;
    });
    expect(result.current.isRefreshing).toBe(false);
  });

  it("kết quả của request cho eventId cũ bị bỏ qua khi eventId đổi (m-4)", async () => {
    const pending: Array<{ id: string; d: ReturnType<typeof deferred<OpsWarRoomSnapshot>> }> = [];
    const fetcher = vi.fn((id: string) => {
      const d = deferred<OpsWarRoomSnapshot>();
      pending.push({ id, d });
      return d.promise;
    });
    const OTHER_ID = "evt-grand-concert-2026";

    const { result, rerender } = renderHook(
      ({ eventId }: { eventId: string }) =>
        useOpsWarRoom({ eventId, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
      { initialProps: { eventId: EVENT_ID } },
    );
    await advance(2000);
    expect(fetcher).toHaveBeenLastCalledWith(EVENT_ID);

    rerender({ eventId: OTHER_ID });

    // refresh() cho eventId mới không được trả lại promise đang treo của eventId cũ.
    await act(async () => {
      void result.current.refresh();
      await Promise.resolve();
    });
    expect(fetcher).toHaveBeenLastCalledWith(OTHER_ID);

    // Request cũ xong muộn: không được ghi đè snapshot.
    const stale = makeSnapshot(clock, {
      funnel: { lobby: 1, queued: 1, admitted: 1, holding: 1, paid: 1, issued: 1 },
    });
    await act(async () => {
      pending[0].d.resolve(stale);
    });
    await flush();
    expect(result.current.snapshot?.funnel.lobby).not.toBe(1);
  });

  it("sau 403: dừng cả timer tuổi dữ liệu — không gọi now() thêm, không render lại (m-8)", async () => {
    const err403 = await realOpsError(403);
    const fetcher = vi.fn(async () => {
      throw err403;
    });
    const nowSpy = vi.fn(() => clock);
    let renders = 0;

    renderHook(() => {
      renders += 1;
      return useOpsWarRoom({
        eventId: EVENT_ID,
        initialSnapshot: makeSnapshot(clock),
        fetcher,
        now: nowSpy,
        intervalMs: 2000,
      });
    });
    await advance(2000);
    await advance(1000);

    const nowCalls = nowSpy.mock.calls.length;
    const renderCount = renders;
    clock += 10_000;
    await advance(10_000);

    expect(nowSpy).toHaveBeenCalledTimes(nowCalls);
    expect(renders).toBe(renderCount);
  });
});

describe("useOpsWarRoom — không chồng request (AC-9)", () => {
  it("fetch chậm hơn interval thì không bắt đầu fetch mới cho đến khi xong", async () => {
    const pending: Array<ReturnType<typeof deferred<OpsWarRoomSnapshot>>> = [];
    const fetcher = vi.fn(() => {
      const d = deferred<OpsWarRoomSnapshot>();
      pending.push(d);
      return d.promise;
    });

    renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );

    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(1);

    // Request đầu vẫn treo: trôi qua nhiều chu kỳ cũng không được gọi thêm.
    await advance(10_000);
    expect(fetcher).toHaveBeenCalledTimes(1);

    await act(async () => {
      pending[0].resolve(makeSnapshot(clock));
    });

    // Lịch kế tiếp tính từ lúc xong: chưa tới intervalMs thì chưa gọi.
    await advance(1999);
    expect(fetcher).toHaveBeenCalledTimes(1);
    await advance(1);
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("refresh() khi đang có request treo trả CÙNG promise, không gọi fetcher lần hai", async () => {
    const pending: Array<ReturnType<typeof deferred<OpsWarRoomSnapshot>>> = [];
    const fetcher = vi.fn(() => {
      const d = deferred<OpsWarRoomSnapshot>();
      pending.push(d);
      return d.promise;
    });

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 60_000 }),
    );
    await flush();

    let p1!: Promise<void>;
    let p2!: Promise<void>;
    await act(async () => {
      p1 = result.current.refresh();
      await Promise.resolve();
    });
    await act(async () => {
      p2 = result.current.refresh();
      await Promise.resolve();
    });

    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(p2).toBe(p1);

    await act(async () => {
      pending[0].resolve(makeSnapshot(clock, { saleState: "SOLD_OUT" }));
      await p1;
    });
    expect(result.current.snapshot?.saleState).toBe("SOLD_OUT");
  });

  it("refresh() trong lúc poll tự động đang treo không tạo request chồng", async () => {
    const pending: Array<ReturnType<typeof deferred<OpsWarRoomSnapshot>>> = [];
    const fetcher = vi.fn(() => {
      const d = deferred<OpsWarRoomSnapshot>();
      pending.push(d);
      return d.promise;
    });

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );

    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(1);

    await act(async () => {
      void result.current.refresh();
      await Promise.resolve();
    });
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("sau refresh() thủ công, lần poll kế tiếp = intervalMs tính từ lúc refresh xong", async () => {
    const fetcher = vi.fn(async () => makeSnapshot(clock));

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );

    await advance(1500);
    await act(async () => {
      await result.current.refresh();
    });
    expect(fetcher).toHaveBeenCalledTimes(1);

    // Mốc cũ (t=2000) không còn: chưa đủ 2000ms kể từ lúc refresh xong (t=1500).
    await advance(1999);
    expect(fetcher).toHaveBeenCalledTimes(1);

    await advance(1);
    expect(fetcher).toHaveBeenCalledTimes(2);
  });
});

describe("useOpsWarRoom — dọn dẹp khi unmount (AC-9)", () => {
  it("unmount thì không gọi fetcher thêm lần nào", async () => {
    const fetcher = vi.fn(async () => makeSnapshot(clock));

    const { unmount } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );
    await advance(2000);
    const calls = fetcher.mock.calls.length;

    unmount();
    await advance(20_000);

    expect(fetcher).toHaveBeenCalledTimes(calls);
  });

  it("request treo xong sau unmount không gây cảnh báo và không lên lịch poll mới", async () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => {});
    const d = deferred<OpsWarRoomSnapshot>();
    const fetcher = vi.fn(() => d.promise);

    const { unmount } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );
    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(1);

    unmount();
    await act(async () => {
      d.resolve(makeSnapshot(clock));
    });
    await advance(10_000);

    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(errorSpy).not.toHaveBeenCalled();
  });
});

describe("useOpsWarRoom — lỗi (AC-2, AC-7, AC-9)", () => {
  it("lỗi thường: giữ snapshot cũ, đặt error, isStale = true", async () => {
    const initial = makeSnapshot(clock);
    const fetcher = vi.fn(async () => {
      throw new Error("mất kết nối");
    });

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: initial, fetcher, now, intervalMs: 2000 }),
    );

    await advance(2000);

    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(result.current.snapshot).toEqual(initial);
    expect(result.current.error).toBeInstanceOf(Error);
    expect(result.current.isStale).toBe(true);
  });

  it("lỗi 500 (OpsServiceError) vẫn tiếp tục poll", async () => {
    const err500 = await realOpsError(500);
    const fetcher = vi.fn(async () => {
      throw err500;
    });

    renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );

    await advance(2000);
    await advance(2000);
    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(3);
  });

  it("sau lỗi vẫn poll, lần thành công kế tiếp xoá lỗi và hết cũ", async () => {
    let fail = true;
    const fetcher = vi.fn(async () => {
      if (fail) throw new Error("tạm lỗi");
      return makeSnapshot(clock);
    });

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );

    await advance(2000);
    expect(result.current.isStale).toBe(true);

    fail = false;
    await advance(2000);

    expect(fetcher).toHaveBeenCalledTimes(2);
    expect(result.current.error).toBeNull();
    expect(result.current.isStale).toBe(false);
  });

  it("lỗi 403 giữa chừng: error là lỗi 403 và DỪNG polling (M-6)", async () => {
    const err403 = await realOpsError(403);
    const fetcher = vi.fn(async () => {
      throw err403;
    });

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
    );

    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(result.current.error).toBe(err403);
    expect((result.current.error as Error & { status?: number }).status).toBe(403);

    await advance(20_000);
    expect(fetcher).toHaveBeenCalledTimes(1);
  });

  it("lỗi 403 ở lần tải đầu (không có initialSnapshot) cũng dừng polling", async () => {
    const err403 = await realOpsError(403);
    const fetcher = vi.fn(async () => {
      throw err403;
    });

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: null, fetcher, now, intervalMs: 2000 }),
    );
    await flush();
    await advance(20_000);

    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(result.current.snapshot).toBeNull();
    expect(result.current.error).toBe(err403);
  });
});

describe("useOpsWarRoom — tuổi dữ liệu theo now() tiêm vào (AC-2)", () => {
  it("ageMs và isStale đổi khi now() vượt mốc 5000ms", async () => {
    const fetcher = vi.fn(() => new Promise<OpsWarRoomSnapshot>(() => {}));

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(BASE_MS), fetcher, now, intervalMs: 60_000 }),
    );
    await flush();

    clock = BASE_MS + 3000;
    await advance(1000);
    expect(result.current.ageMs).toBe(3000);
    expect(result.current.isStale).toBe(false);

    clock = BASE_MS + 5000;
    await advance(1000);
    expect(result.current.ageMs).toBe(5000);
    expect(result.current.isStale).toBe(false);

    clock = BASE_MS + 5001;
    await advance(1000);
    expect(result.current.ageMs).toBe(5001);
    expect(result.current.isStale).toBe(true);
  });

  it("snapshot có generatedAt cũ thì isStale ngay sau khi mount", async () => {
    const fetcher = vi.fn(() => new Promise<OpsWarRoomSnapshot>(() => {}));

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(BASE_MS - 60_000), fetcher, now }),
    );
    await flush();

    expect(result.current.ageMs).toBe(60_000);
    expect(result.current.isStale).toBe(true);
  });

  it("generatedAt ở tương lai (lệch đồng hồ) thì ageMs kẹp về 0", async () => {
    const fetcher = vi.fn(() => new Promise<OpsWarRoomSnapshot>(() => {}));

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(BASE_MS + 30_000), fetcher, now }),
    );
    await flush();

    expect(result.current.ageMs).toBe(0);
    expect(result.current.isStale).toBe(false);
  });

  it("snapshot mới hơn từ fetcher làm hết cảnh báo cũ", async () => {
    const fetcher = vi.fn(async () => makeSnapshot(clock));

    const { result } = renderHook(() =>
      useOpsWarRoom({
        eventId: EVENT_ID,
        initialSnapshot: makeSnapshot(BASE_MS - 60_000),
        fetcher,
        now,
        intervalMs: 2000,
      }),
    );
    await flush();
    expect(result.current.isStale).toBe(true);

    clock = BASE_MS + 2000;
    await advance(2000);

    expect(result.current.isStale).toBe(false);
    expect(result.current.ageMs).toBe(0);
  });
});

describe("useOpsWarRoom — refresh() thủ công (AC-9)", () => {
  it("refresh() gọi fetcher ngay, cập nhật snapshot và lastSuccessAt", async () => {
    const next = makeSnapshot(clock, { saleState: "SOLD_OUT" });
    const fetcher = vi.fn(async () => next);

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 60_000 }),
    );
    await flush();

    await act(async () => {
      await result.current.refresh();
    });

    expect(fetcher).toHaveBeenCalledTimes(1);
    expect(result.current.snapshot?.saleState).toBe("SOLD_OUT");
    expect(result.current.lastSuccessAt).not.toBeNull();
    expect(result.current.isRefreshing).toBe(false);
  });

  it("refresh() lỗi thì giữ snapshot cũ và đặt error", async () => {
    const initial = makeSnapshot(clock);
    const fetcher = vi.fn(async () => {
      throw new Error("lỗi khi làm mới");
    });

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: initial, fetcher, now, intervalMs: 60_000 }),
    );
    await flush();

    // Hợp đồng không chốt refresh() reject hay resolve khi lỗi; lỗi phải nằm ở state.
    await act(async () => {
      await result.current.refresh().catch(() => {});
    });

    expect(result.current.snapshot).toEqual(initial);
    expect(result.current.error?.message).toBe("lỗi khi làm mới");
    expect(result.current.isStale).toBe(true);
    expect(result.current.isRefreshing).toBe(false);
  });
});

describe("useOpsWarRoom — timeoutMs và mutant sống (plan §8 C-1, X63)", () => {
  it("mặc định timeoutMs = 10000: request treo quá hạn → isStale, hết isRefreshing, poll tiếp sau intervalMs", async () => {
    const fetcher = vi.fn(() => new Promise<OpsWarRoomSnapshot>(() => {}));
    const initial = makeSnapshot(clock);

    const { result } = renderHook(() =>
      useOpsWarRoom({ eventId: EVENT_ID, initialSnapshot: initial, fetcher, now, intervalMs: 2000 }),
    );

    await advance(2000);
    expect(fetcher).toHaveBeenCalledTimes(1);

    await advance(9999);
    expect(result.current.isStale).toBe(false);
    expect(fetcher).toHaveBeenCalledTimes(1);

    await advance(1);
    expect(result.current.isStale).toBe(true);
    expect(result.current.isRefreshing).toBe(false);
    expect(result.current.error).not.toBeNull();
    expect(result.current.snapshot).toEqual(initial);

    await advance(1999);
    expect(fetcher).toHaveBeenCalledTimes(1);
    await advance(1);
    expect(fetcher).toHaveBeenCalledTimes(2);
  });

  it("timeoutMs tuỳ chỉnh được tôn trọng", async () => {
    const fetcher = vi.fn(() => new Promise<OpsWarRoomSnapshot>(() => {}));

    const { result } = renderHook(() =>
      useOpsWarRoom({
        eventId: EVENT_ID,
        initialSnapshot: makeSnapshot(clock),
        fetcher,
        now,
        intervalMs: 2000,
        timeoutMs: 3000,
      }),
    );

    await advance(2000);
    await advance(2999);
    expect(result.current.isStale).toBe(false);
    await advance(1);
    expect(result.current.isStale).toBe(true);
  });

  it("kết quả đến muộn của request đã quá hạn không ghi đè snapshot", async () => {
    const pending: Array<ReturnType<typeof deferred<OpsWarRoomSnapshot>>> = [];
    const fetcher = vi.fn(() => {
      const d = deferred<OpsWarRoomSnapshot>();
      pending.push(d);
      return d.promise;
    });

    const { result } = renderHook(() =>
      useOpsWarRoom({
        eventId: EVENT_ID,
        initialSnapshot: makeSnapshot(clock),
        fetcher,
        now,
        intervalMs: 2000,
        timeoutMs: 3000,
      }),
    );

    await advance(2000);
    await advance(3000);
    expect(result.current.isStale).toBe(true);

    await act(async () => {
      pending[0].resolve(
        makeSnapshot(clock, { funnel: { lobby: 1, queued: 1, admitted: 1, holding: 1, paid: 1, issued: 1 } }),
      );
    });
    await flush();

    expect(result.current.snapshot?.funnel.lobby).toBe(911);
    expect(result.current.isStale).toBe(true);
  });

  it("refresh() thủ công bị treo quá hạn: isRefreshing về false", async () => {
    const fetcher = vi.fn(() => new Promise<OpsWarRoomSnapshot>(() => {}));

    const { result } = renderHook(() =>
      useOpsWarRoom({
        eventId: EVENT_ID,
        initialSnapshot: makeSnapshot(clock),
        fetcher,
        now,
        intervalMs: 60_000,
        timeoutMs: 3000,
      }),
    );
    await flush();

    await act(async () => {
      void result.current.refresh().catch(() => {});
      await Promise.resolve();
    });
    expect(result.current.isRefreshing).toBe(true);

    await advance(3000);
    expect(result.current.isRefreshing).toBe(false);
    expect(result.current.isStale).toBe(true);
  });

  it("đổi eventId sau khi 403 dừng polling → polling chạy lại cho eventId mới (X63)", async () => {
    const err403 = await realOpsError(403);
    const OTHER_ID = "evt-grand-concert-2026";
    const fetcher = vi.fn(async (id: string) => {
      if (id === EVENT_ID) throw err403;
      return makeSnapshot(clock, { eventId: OTHER_ID });
    });

    const { result, rerender } = renderHook(
      ({ eventId }: { eventId: string }) =>
        useOpsWarRoom({ eventId, initialSnapshot: makeSnapshot(clock), fetcher, now, intervalMs: 2000 }),
      { initialProps: { eventId: EVENT_ID } },
    );

    await advance(2000);
    await advance(10_000);
    expect(fetcher).toHaveBeenCalledTimes(1);

    rerender({ eventId: OTHER_ID });
    await advance(2000);
    expect(fetcher).toHaveBeenCalledWith(OTHER_ID);
    const calls = fetcher.mock.calls.length;

    await advance(2000);
    expect(fetcher.mock.calls.length).toBeGreaterThan(calls);
    expect(result.current.error).toBeNull();
  });
});
