"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { useServerCountdown } from "@/hooks/use-server-countdown";
import {
  clearActiveHold,
  getActiveHold,
  HoldClientError,
  requestHold,
  saveActiveHold,
  type HoldResult,
} from "@/lib/hold-client";
import { serverNow, toEpochMs } from "@/lib/server-time";
import type { HoldState } from "@/components/checkout/order-summary";

export interface UseHoldTimerOptions {
  eventId: string;
  queueToken?: string | null;
  identityId?: string;
  offsetMs?: number;
  apiBaseUrl?: string;
}

export interface UseHoldTimerReturn {
  hold: HoldResult | null;
  holdState: HoldState;
  remainingMs: number;
  expired: boolean;
  ready: boolean;
  error: HoldClientError | null;
  createHold: (ticketTypeId: string, quantity: number) => Promise<boolean>;
  resetHold: () => void;
}

const QUEUE_STORAGE_PREFIX = "evenflow_queue_token_";

function getStoredQueueToken(eventId: string): string | null {
  if (typeof window === "undefined" || !window.sessionStorage) return null;
  try {
    return sessionStorage.getItem(`${QUEUE_STORAGE_PREFIX}${eventId}`);
  } catch {
    return null;
  }
}

/** Hook quản lý vòng đời giữ vé và đồng hồ 10:00 theo expires_at của server (EVF-113, BR-O2, BR-O5).
 *
 * 1. BR-O2: Client không tự tính giờ — countdown đếm theo mốc expires_at và offsetMs chuẩn của server.
 * 2. BR-O5: Đặt vé dùng Idempotency-Key; chặn double click khi creating; lỗi mạng -> ambiguous.
 * 3. Hết hạn -> tự động chuyển trạng thái sang "expired" và giải phóng storage cục bộ.
 */
export function useHoldTimer({
  eventId,
  queueToken: propQueueToken,
  identityId,
  offsetMs = 0,
  apiBaseUrl,
}: UseHoldTimerOptions): UseHoldTimerReturn {
  const [hold, setHold] = useState<HoldResult | null>(null);
  const [holdState, setHoldState] = useState<HoldState>("idle");
  const [error, setError] = useState<HoldClientError | null>(null);
  const isCreatingRef = useRef(false);

  // Khoi phuc hold tu sessionStorage khi mount
  useEffect(() => {
    if (!eventId) return;
    const existing = getActiveHold(eventId);
    if (existing) {
      const expEpoch = toEpochMs(existing.expiresAt);
      if (expEpoch !== null && expEpoch - serverNow(offsetMs) > 0) {
        setHold(existing);
        setHoldState("active");
      } else {
        clearActiveHold(eventId);
        setHold(null);
        setHoldState("expired");
      }
    }
  }, [eventId, offsetMs]);

  // Dong ho dem nguoc theo expiresAt cua server (BR-O2)
  const countdown = useServerCountdown(hold?.expiresAt, offsetMs);

  // Xu ly khi dong ho cham 0 (expired)
  useEffect(() => {
    if (hold && countdown.ready && countdown.expired && holdState === "active") {
      setHoldState("expired");
      clearActiveHold(eventId);
    }
  }, [hold, countdown.ready, countdown.expired, holdState, eventId]);

  const createHold = useCallback(
    async (ticketTypeId: string, quantity: number): Promise<boolean> => {
      // Chặn duplicate click khi đang tạo (BR-O5)
      if (holdState === "creating" || isCreatingRef.current) {
        return false;
      }

      const effectiveToken = propQueueToken || getStoredQueueToken(eventId);
      if (!effectiveToken) {
        const notAdmittedErr = new HoldClientError(
          "Bạn chưa được cấp quyền mua vé, vui lòng quay lại phòng chờ",
          "NOT_ADMITTED",
          403,
        );
        setError(notAdmittedErr);
        return false;
      }

      isCreatingRef.current = true;
      setHoldState("creating");
      setError(null);

      try {
        const result = await requestHold({
          eventId,
          ticketTypeId,
          quantity,
          queueToken: effectiveToken,
          identityId,
          apiBaseUrl,
        });

        setHold(result);
        setHoldState("active");
        saveActiveHold(eventId, result);
        return true;
      } catch (err) {
        const clientErr =
          err instanceof HoldClientError
            ? err
            : new HoldClientError("Lỗi không xác định khi tạo hold", "UNKNOWN");

        setError(clientErr);

        if (clientErr.type === "SOLD_OUT") {
          setHoldState("sold_out");
        } else if (clientErr.type === "AMBIGUOUS") {
          // BR-O5: Trạng thái không rõ kết quả, không cho bấm lại bừa bãi
          setHoldState("ambiguous");
        } else {
          setHoldState("idle");
        }
        return false;
      } finally {
        isCreatingRef.current = false;
      }
    },
    [holdState, propQueueToken, eventId, identityId, apiBaseUrl],
  );

  const resetHold = useCallback(() => {
    clearActiveHold(eventId);
    setHold(null);
    setHoldState("idle");
    setError(null);
  }, [eventId]);

  return {
    hold,
    holdState,
    remainingMs: countdown.remainingMs,
    expired: countdown.expired,
    ready: countdown.ready,
    error,
    createHold,
    resetHold,
  };
}
