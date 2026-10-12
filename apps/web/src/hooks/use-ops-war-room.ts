"use client";

/** Hook polling so lieu war room cho Ops (EV-187, EVF-117, AC-2, AC-9).
 *
 * - Co initialSnapshot: lan fetch dau sau intervalMs; khong co: fetch ngay khi mount.
 * - MOT timer setTimeout tu len lich lai sau khi request XONG (khong chong request).
 *   Them mot timer 1s cap nhat `ageMs`. Don ca hai khi unmount.
 * - refresh() khi dang co request tra CUNG promise, khong goi fetcher lan hai.
 * - Loi 403: dung polling. Loi khac: giu snapshot cu, isStale = true, van poll.
 * - ageMs = null cho toi khi mount (server va lan render dau cua client giong nhau, tranh
 *   loi hydration vi Date.now khac nhau). */

import { useCallback, useEffect, useRef, useState } from "react";

import {
  OPS_POLL_INTERVAL_MS,
  STALE_AFTER_MS,
  getOpsWarRoom,
  opsErrorStatus,
  snapshotAgeMs,
  type OpsServiceError,
  type OpsWarRoomSnapshot,
} from "@/lib/ops-service";

export interface UseOpsWarRoomOptions {
  eventId: string;
  initialSnapshot?: OpsWarRoomSnapshot | null;
  intervalMs?: number;
  fetcher?: (eventId: string) => Promise<OpsWarRoomSnapshot>;
  now?: () => number;
  /** Request qua han coi nhu loi (mac dinh 10 000ms). */
  timeoutMs?: number;
}

export interface UseOpsWarRoomReturn {
  snapshot: OpsWarRoomSnapshot | null;
  error: OpsServiceError | Error | null;
  isRefreshing: boolean;
  lastSuccessAt: number | null;
  /** Tuoi du lieu theo generatedAt; null cho toi khi mount. */
  ageMs: number | null;
  /** ageMs > STALE_AFTER_MS hoac lan lam moi gan nhat loi. */
  isStale: boolean;
  refresh: () => Promise<void>;
}

const AGE_TICK_MS = 1000;
export const OPS_REQUEST_TIMEOUT_MS = 10_000;

/** Qua han thi reject; ket qua den muon bi bo (promise goc van tu giai quyet, khong ai doc). */
function withTimeout<T>(promise: Promise<T>, ms: number): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error("Quá thời gian chờ số liệu war room.")), ms);
    promise.then(
      (value) => {
        clearTimeout(timer);
        resolve(value);
      },
      (err: unknown) => {
        clearTimeout(timer);
        reject(err);
      },
    );
  });
}

export function useOpsWarRoom({
  eventId,
  initialSnapshot = null,
  intervalMs = OPS_POLL_INTERVAL_MS,
  fetcher = getOpsWarRoom,
  now = Date.now,
  timeoutMs = OPS_REQUEST_TIMEOUT_MS,
}: UseOpsWarRoomOptions): UseOpsWarRoomReturn {
  const [snapshot, setSnapshot] = useState<OpsWarRoomSnapshot | null>(initialSnapshot);
  const [error, setError] = useState<OpsServiceError | Error | null>(null);
  const [isRefreshing, setIsRefreshing] = useState(false);
  const [lastSuccessAt, setLastSuccessAt] = useState<number | null>(null);
  const [tick, setTick] = useState<number | null>(null);

  // Luon dung gia tri moi nhat ma khong lam effect chay lai.
  const fetcherRef = useRef(fetcher);
  const nowRef = useRef(now);
  const intervalRef = useRef(intervalMs);
  fetcherRef.current = fetcher;
  nowRef.current = now;
  intervalRef.current = intervalMs;
  const timeoutRef = useRef(timeoutMs);
  timeoutRef.current = timeoutMs;

  const mountedRef = useRef(false);
  const stoppedRef = useRef(false);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Request dang bay, gan voi eventId cua no (doi su kien thi khong dung lai request cu).
  const inflightRef = useRef<{ eventId: string; promise: Promise<void> | null } | null>(null);
  const eventIdRef = useRef(eventId);
  eventIdRef.current = eventId;
  const [pollingStopped, setPollingStopped] = useState(false);
  const runRef = useRef<(manual: boolean) => Promise<void>>(() => Promise.resolve());

  const schedule = useCallback(() => {
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    if (!mountedRef.current || stoppedRef.current) return;
    timerRef.current = setTimeout(() => {
      timerRef.current = null;
      void runRef.current(false);
    }, intervalRef.current);
  }, []);

  const run = useCallback(
    (manual: boolean): Promise<void> => {
      const current = inflightRef.current;
      if (current && current.eventId === eventId) {
        // Dang co request cho su kien nay: dung lai, nhung refresh() thu cong van phai
        // cho nguoi dung thay trang thai "dang lam moi" cho toi khi no xong.
        if (manual && mountedRef.current) setIsRefreshing(true);
        return current.promise ?? Promise.resolve();
      }

      const isCurrent = () => mountedRef.current && eventIdRef.current === eventId;

      const entry: { eventId: string; promise: Promise<void> | null } = { eventId, promise: null };
      entry.promise = (async () => {
        // Nhuong mot vi tro de inflightRef duoc gan truoc khi fetcher co the chay xong dong bo.
        await null;
        if (manual && isCurrent()) setIsRefreshing(true);
        try {
          const next = await withTimeout(fetcherRef.current(eventId), timeoutRef.current);
          if (!isCurrent()) return;
          setSnapshot(next);
          setError(null);
          setLastSuccessAt(nowRef.current());
        } catch (err) {
          if (!isCurrent()) return;
          setError(err instanceof Error ? err : new Error(String(err)));
          if (opsErrorStatus(err) === 403) {
            stoppedRef.current = true;
            setPollingStopped(true);
          }
        } finally {
          if (inflightRef.current === entry) inflightRef.current = null;
          if (isCurrent()) {
            setIsRefreshing(false);
            setTick(nowRef.current());
            schedule();
          }
        }
      })();

      inflightRef.current = entry;
      return entry.promise;
    },
    [eventId, schedule],
  );

  runRef.current = run;

  useEffect(() => {
    mountedRef.current = true;
    stoppedRef.current = false;
    setPollingStopped(false);
    setTick(nowRef.current());

    if (initialSnapshot) {
      schedule();
    } else {
      void run(false);
    }

    return () => {
      mountedRef.current = false;
      if (timerRef.current !== null) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
    };
    // Chi chay lai khi doi su kien; initialSnapshot chi la gia tri khoi tao.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [eventId]);

  // Timer tuoi du lieu: dung khi polling dung vi 403 (khong con gi de cap nhat).
  useEffect(() => {
    if (pollingStopped) return;
    const ageTimer = setInterval(() => setTick(nowRef.current()), AGE_TICK_MS);
    return () => clearInterval(ageTimer);
  }, [eventId, pollingStopped]);

  const refresh = useCallback(() => run(true), [run]);

  const ageMs =
    snapshot && tick !== null ? Math.max(0, snapshotAgeMs(snapshot, tick)) : null;
  const isStale = (ageMs !== null && ageMs > STALE_AFTER_MS) || error !== null;

  return { snapshot, error, isRefreshing, lastSuccessAt, ageMs, isStale, refresh };
}
