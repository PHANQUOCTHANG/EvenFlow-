"use client";

import { useCallback, useEffect, useState } from "react";

import { getEventById } from "@/lib/event-service";
import type { EventDetailSnapshot } from "@/lib/types/event";

export type UseEventState =
  | { status: "loading"; event: null }
  | { status: "ready"; event: EventDetailSnapshot }
  | { status: "not_found"; event: null }
  | { status: "error"; event: null };

export type UseEventReturn = UseEventState & { retry: () => void };

/** Tai su kien theo UUID o trinh duyet (AC-18). Cac trang mua ve tra cuu phia client de mock mang cua E2E
 *  va phien dang nhap deu hoat dong. null (404) -> `not_found`; loi khac -> `error` kem `retry`. */
export function useEvent(eventId: string): UseEventReturn {
  const [state, setState] = useState<UseEventState>({ status: "loading", event: null });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading", event: null });
    getEventById(eventId)
      .then((event) => {
        if (cancelled) return;
        setState(event ? { status: "ready", event } : { status: "not_found", event: null });
      })
      .catch(() => {
        if (!cancelled) setState({ status: "error", event: null });
      });
    return () => {
      cancelled = true;
    };
  }, [eventId, attempt]);

  const retry = useCallback(() => setAttempt((n) => n + 1), []);
  return { ...state, retry } as UseEventReturn;
}
