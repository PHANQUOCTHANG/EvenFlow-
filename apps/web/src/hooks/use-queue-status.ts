"use client";

import { useEffect, useState } from "react";

import type { ConnectionState } from "@/components/queue/queue-status-panel";
import { backoffDelay } from "@/lib/backoff";
import { QueueError } from "@/lib/queue-client";
import {
  DEFAULT_POLL_MS,
  fetchQueueStatus,
  isTerminalState,
  normalizePosition,
  type QueueSnapshot,
} from "@/lib/queue-status";
import { connectSse, type SseConnection } from "@/lib/sse";

/** Hook theo doi vi tri trong phong cho (EV-182 / EVF-112).
 *
 * Ba cam ket voi backend — moi cam ket co test rieng trong use-queue-status.test.tsx:
 *
 * 1. NHIP POLL LA CUA SERVER (AC-1, BR-Q4). Lan hoi ke tiep chi duoc hen sau `poll_after_ms`
 *    (200), `X-Poll-After-Ms` (304) hoac `Retry-After` (503). Khong co duong nao khac tao request.
 *
 * 2. KHONG BAO REQUEST KHI MAT MANG (AC-2). Loi mang/5xx lui theo `backoffDelay`. Hook CO Y
 *    khong nghe su kien `online`: khi mot nha mang chap chon, hang tram nghin tab nhan `online`
 *    cung mot luc — neu moi tab poll ngay thi chinh hook nay tao ra dinh tai.
 *
 * 3. TAB AN KHONG POLL DAY HON (AC-3). Hook CO Y khong nghe `visibilitychange` / `focus`: kieu
 *    "refetch khi quay lai tab" la mot request NGOAI nhip server. Lich hen van giu nguyen; trinh
 *    duyet co the lam cham timer cua tab an, nhung khong bao gio lam nhanh hon. Khong lam CHAM
 *    hon chu dong: khach dang chuyen tab van can biet ngay khi toi luot (suat 15 phut, BR-Q7).
 *
 * SSE (AC-4): khi server cap nhip <= `sseThresholdMs` (nhom gan luot, `pollNear` = 3s) thi nang
 * cap len `/queue/stream`. Stream dong/loi qua so lan thu -> quay lai polling theo nhip gan nhat,
 * KHONG poll ngay. */

export type QueueTransport = "idle" | "polling" | "sse";

export interface UseQueueStatusOptions {
  eventId: string;
  /** null = chua co token (dang join) -> hook dung yen, khong goi gi. */
  token: string | null;
  enableSse?: boolean;
  sseThresholdMs?: number;
  baseUrl?: string;
}

export interface QueueStatusView {
  snapshot: QueueSnapshot | null;
  /** Rank LUC VAO hang — mau so co dinh cho thanh tien trinh. */
  initialRank: number | null;
  connection: ConnectionState;
  transport: QueueTransport;
  lastUpdatedAt: number | null;
  /** Loi cuoi (4xx): hook da dung, khong retry. */
  error: QueueError | null;
}

/** `pollNear` phia server (services/waitingroom/internal/domain/position.go). */
export const SSE_THRESHOLD_MS = 3_000;
/** Bao nhieu lan loi lien tiep thi coi la "mat ket noi" thay vi "dang ket noi lai". */
export const OFFLINE_AFTER_FAILURES = 3;

const INITIAL_VIEW: QueueStatusView = {
  snapshot: null,
  initialRank: null,
  connection: "live",
  transport: "idle",
  lastUpdatedAt: null,
  error: null,
};

const initialRankKey = (eventId: string) => `ef:queue:initial-rank:${eventId}`;

/** Mau so cua thanh tien trinh phai song sot qua F5: neu reset ve rank hien tai thi thanh tien
 *  trinh tut ve 0 ngay khi khach tai lai trang — dung cai cam giac "mat cho" ma ta dang tranh. */
function readInitialRank(eventId: string): number | null {
  try {
    const raw = window.sessionStorage.getItem(initialRankKey(eventId));
    const n = raw === null ? Number.NaN : Number(raw);
    return Number.isInteger(n) && n > 0 ? n : null;
  } catch {
    return null;
  }
}

function storeInitialRank(eventId: string, rank: number) {
  try {
    window.sessionStorage.setItem(initialRankKey(eventId), String(rank));
  } catch {
    // Private mode / quota: mat mau so khi F5 la chap nhan duoc, khong dang chan UI.
  }
}

function supportsStreaming(): boolean {
  return typeof ReadableStream !== "undefined" && typeof TextDecoder !== "undefined";
}

export function useQueueStatus({
  eventId,
  token,
  enableSse = true,
  sseThresholdMs = SSE_THRESHOLD_MS,
  baseUrl,
}: UseQueueStatusOptions): QueueStatusView {
  const [view, setView] = useState<QueueStatusView>(INITIAL_VIEW);

  useEffect(() => {
    if (!token) {
      setView(INITIAL_VIEW);
      return;
    }
    const queueToken = token;
    const base = baseUrl ?? process.env.NEXT_PUBLIC_API_BASE ?? "";

    let disposed = false;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let sse: SseConnection | null = null;
    let terminal = false;
    const abort = new AbortController();

    let etag: string | null = null;
    let lastSnapshot: QueueSnapshot | null = null;
    let lastPollAfterMs = DEFAULT_POLL_MS;
    let failures = 0;
    let initialRank = readInitialRank(eventId);

    setView({ ...INITIAL_VIEW, initialRank });

    const update = (patch: Partial<QueueStatusView>) => {
      if (!disposed) setView((prev) => ({ ...prev, ...patch }));
    };

    const schedule = (ms: number) => {
      if (disposed) return;
      timer = setTimeout(() => {
        timer = null;
        void poll();
      }, ms);
    };

    const applySnapshot = (snapshot: QueueSnapshot) => {
      lastSnapshot = snapshot;
      lastPollAfterMs = snapshot.pollAfterMs;
      failures = 0;
      if (initialRank === null && snapshot.state === "QUEUED" && snapshot.rank !== null && snapshot.rank > 0) {
        initialRank = snapshot.rank;
        storeInitialRank(eventId, snapshot.rank);
      }
      if (isTerminalState(snapshot.state)) terminal = true;
      update({ snapshot, initialRank, connection: "live", lastUpdatedAt: Date.now(), error: null });
    };

    const wantsSse = (snapshot: QueueSnapshot | null, pollAfterMs: number) =>
      enableSse &&
      snapshot !== null &&
      snapshot.state === "QUEUED" &&
      pollAfterMs <= sseThresholdMs &&
      supportsStreaming();

    function startSse() {
      update({ transport: "sse" });
      const conn = connectSse({
        url: `${base}/v1/events/${encodeURIComponent(eventId)}/queue/stream`,
        headers: { "X-Queue-Token": queueToken },
        onOpen: () => update({ connection: "live" }),
        onRetry: () => update({ connection: "reconnecting" }),
        onMessage: (message) => {
          if (message.event !== "position") return;
          let raw: unknown;
          try {
            raw = JSON.parse(message.data);
          } catch {
            return; // mot frame hong khong dang lam dut ca stream
          }
          applySnapshot(normalizePosition(raw));
          if (terminal) conn.close();
        },
      });
      sse = conn;

      void conn.done.then((reason) => {
        if (sse === conn) sse = null;
        if (disposed) return;
        if (terminal) {
          update({ transport: "idle" });
          return;
        }
        if (reason === "closed") return;
        // ended / gave-up / fatal: ve polling theo nhip server GAN NHAT. Khong poll ngay —
        // mot pod SSE chet keo theo hang nghin stream dong cung luc.
        update({ transport: "polling" });
        schedule(lastPollAfterMs);
      });
    }

    async function poll() {
      if (disposed) return;
      try {
        const result = await fetchQueueStatus(eventId, queueToken, { etag, signal: abort.signal, baseUrl: base });
        if (disposed) return;

        if (result.kind === "throttled") {
          // Server dang xa tai co kiem soat — van "song", chi la phai cho lau hon.
          failures = 0;
          update({ connection: "live", transport: "polling" });
          schedule(result.retryAfterMs);
          return;
        }

        etag = result.etag;

        if (result.kind === "not-modified") {
          failures = 0;
          lastPollAfterMs = result.pollAfterMs;
          update({ connection: "live", lastUpdatedAt: Date.now() });
          if (wantsSse(lastSnapshot, result.pollAfterMs)) {
            startSse();
            return;
          }
          update({ transport: "polling" });
          schedule(result.pollAfterMs);
          return;
        }

        applySnapshot(result.snapshot);
        if (terminal) {
          update({ transport: "idle" });
          return;
        }
        if (wantsSse(result.snapshot, result.pollAfterMs)) {
          startSse();
          return;
        }
        update({ transport: "polling" });
        schedule(result.pollAfterMs);
      } catch (err) {
        if (disposed || abort.signal.aborted) return;

        if (err instanceof QueueError && err.status >= 400 && err.status < 500 && err.status !== 429) {
          // Token sai/het han, chua dang nhap...: retry khong lam no dung len duoc.
          update({ error: err, transport: "idle" });
          return;
        }

        failures += 1;
        const browserOffline = typeof navigator !== "undefined" && navigator.onLine === false;
        update({
          connection: browserOffline || failures >= OFFLINE_AFTER_FAILURES ? "offline" : "reconnecting",
        });
        schedule(backoffDelay(failures));
      }
    }

    void poll();

    return () => {
      disposed = true;
      if (timer !== null) clearTimeout(timer);
      abort.abort();
      sse?.close();
    };
  }, [eventId, token, enableSse, sseThresholdMs, baseUrl]);

  return view;
}
