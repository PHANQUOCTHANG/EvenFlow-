"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { useServerCountdown } from "@/hooks/use-server-countdown";
import {
  cancelHold,
  clearActiveHold,
  getActiveHold,
  HoldClientError,
  requestHold,
  saveActiveHold,
  type HoldResult,
} from "@/lib/hold-client";
import { readQueueToken } from "@/lib/queue-token";
import { serverNow, toEpochMs } from "@/lib/server-time";
import type { HoldState } from "@/components/checkout/order-summary";

export interface UseHoldTimerOptions {
  eventId: string;
  queueToken?: string | null;
  offsetMs?: number;
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
  /** AC-11: huy hold dang active (DELETE /v1/holds/{id}). Chi goi sau khi nguoi mua xac nhan. */
  cancelActiveHold: () => Promise<CancelHoldOutcome>;
  /** true trong luc DELETE dang chay. */
  cancelling: boolean;
}

/** released: da tra ve (204/404/2xx); ignored: khong co hold active hoac dang huy (double click) hoac da unmount;
 *  failed: hold van giu (AUTH_REQUIRED / AMBIGUOUS / HTTP khac). */
export type CancelHoldOutcome =
  | { status: "released" }
  | { status: "ignored" }
  | { status: "failed"; error: HoldClientError };

/** Hook quản lý vòng đời giữ vé và đồng hồ 10:00 theo expires_at của server (EVF-113, BR-O2, BR-O5).
 *
 * 1. BR-O2: Client không tự tính giờ — countdown đếm theo mốc expires_at và offsetMs chuẩn của server.
 * 2. BR-O5: Đặt vé dùng Idempotency-Key; chặn double click khi creating; lỗi mạng -> ambiguous.
 * 3. Hết hạn -> tự động chuyển trạng thái sang "expired" và giải phóng storage cục bộ.
 */
export function useHoldTimer({
  eventId,
  queueToken: propQueueToken,
  offsetMs = 0,
}: UseHoldTimerOptions): UseHoldTimerReturn {
  const [hold, setHold] = useState<HoldResult | null>(null);
  const [holdState, setHoldState] = useState<HoldState>("idle");
  const [error, setError] = useState<HoldClientError | null>(null);
  const isCreatingRef = useRef(false);
  const [cancelling, setCancelling] = useState(false);
  const cancellingRef = useRef(false);
  // Doc hold/trang thai qua ref (khong dung closure cu) khi DELETE tra ve.
  const holdRef = useRef(hold);
  const holdStateRef = useRef(holdState);
  holdRef.current = hold;
  holdStateRef.current = holdState;
  const mountedRef = useRef(true);
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
    };
  }, []);

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

      const effectiveToken = propQueueToken || readQueueToken(eventId);
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
    [holdState, propQueueToken, eventId],
  );

  const resetHold = useCallback(() => {
    clearActiveHold(eventId);
    setHold(null);
    setHoldState("idle");
    setError(null);
  }, [eventId]);

  const cancelActiveHold = useCallback(async (): Promise<CancelHoldOutcome> => {
    const current = holdRef.current;
    if (cancellingRef.current || !current || holdStateRef.current !== "active") return { status: "ignored" };
    cancellingRef.current = true;
    setCancelling(true);
    try {
      await cancelHold(current.holdId);
      if (!mountedRef.current) return { status: "ignored" };
      // Het han trong luc DELETE dang bay: giu trang thai expired (khong ve idle).
      // Doc lai qua ref sau await (TS van thu hep kieu tu lan kiem tra truoc await).
      const stateNow = holdStateRef.current as HoldState;
      if (stateNow !== "expired") {
        clearActiveHold(eventId);
        setHold(null);
      }
      setHoldState((s) => (s === "expired" ? s : "idle"));
      setError(null);
      return { status: "released" };
    } catch (err) {
      if (!mountedRef.current) return { status: "ignored" };
      const clientErr =
        err instanceof HoldClientError ? err : new HoldClientError("Không thể huỷ giữ vé", "UNKNOWN");
      return { status: "failed", error: clientErr };
    } finally {
      cancellingRef.current = false;
      if (mountedRef.current) setCancelling(false);
    }
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
    cancelActiveHold,
    cancelling,
  };
}
