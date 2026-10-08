"use client";

import { useCallback, useEffect, useState } from "react";

import { serverNow, serverOffsetMs } from "@/lib/server-time";

export interface ServerTimeSyncState {
  offsetMs: number;
  synced: boolean;
  isLoading: boolean;
  error: Error | null;
  /** Lay thoi gian server hien tai da bu tru do lech. */
  getServerNow: () => number;
}

const DEFAULT_STATE: ServerTimeSyncState = {
  offsetMs: 0,
  synced: false,
  isLoading: true,
  error: null,
  getServerNow: () => Date.now(),
};

/** Hook dong bo do lech dong ho giua client va server (EVF-111, BR-O2).
 *
 * Goi endpoint `/api/time` mot lan khi mount tren browser de do round-trip time (RTT)
 * va tinh `offsetMs = (server_time + rtt/2) - clientNowMs`.
 * Neu that bai, fallback an toan ve gio client (offsetMs = 0) va ghi nhan error. */
export function useServerTimeSync(endpoint: string = "/api/time"): ServerTimeSyncState {
  const [state, setState] = useState<ServerTimeSyncState>(DEFAULT_STATE);

  useEffect(() => {
    let cancelled = false;

    async function syncTime() {
      const clientReqStart = Date.now();

      try {
        const res = await fetch(endpoint, {
          method: "GET",
          headers: { Accept: "application/json" },
        });

        if (!res.ok) {
          throw new Error(`HTTP ${res.status}: Khong the lay gio server tu ${endpoint}`);
        }

        const data = await res.json();
        const clientReqEnd = Date.now();
        const rtt = Math.max(0, clientReqEnd - clientReqStart);

        // Uoc luong gio server tai thoi diem client nhan goi tin
        const estimatedServerTime = data.server_time + Math.floor(rtt / 2);
        const offset = serverOffsetMs(estimatedServerTime, clientReqEnd);

        if (!cancelled) {
          setState({
            offsetMs: offset,
            synced: true,
            isLoading: false,
            error: null,
            getServerNow: () => serverNow(offset),
          });
        }
      } catch (err) {
        if (!cancelled) {
          const errorObj = err instanceof Error ? err : new Error(String(err));
          setState({
            offsetMs: 0,
            synced: false,
            isLoading: false,
            error: errorObj,
            getServerNow: () => Date.now(),
          });
        }
      }
    }

    void syncTime();

    return () => {
      cancelled = true;
    };
  }, [endpoint]);

  const getServerNow = useCallback(() => {
    return serverNow(state.offsetMs);
  }, [state.offsetMs]);

  return {
    ...state,
    getServerNow,
  };
}
