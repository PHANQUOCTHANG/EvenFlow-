"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import {
  forgetIssueKey,
  getOrderDetails,
  issueTickets,
  PaymentClientError,
  type IssuedTicketQR,
  type OrderDetails,
  type OrderStatus,
} from "@/lib/payment-client";

export interface UseOrderStatusOptions {
  orderId: string;
  eventId: string;
  initialDetails?: OrderDetails | null;
  pollIntervalMs?: number;
  maxPollAttempts?: number;
}

export interface UseOrderStatusReturn {
  order: OrderDetails | null;
  status: OrderStatus;
  loading: boolean;
  pollAttempts: number;
  isPendingWebhook: boolean;
  /** Loi lan tai gan nhat (null khi thanh cong). Co the la NOT_FOUND (404) hoac loi mang/may chu. */
  error: PaymentClientError | null;
  refresh: () => Promise<void>;
}

/** Nhip poll: 2 lan dau theo `base`, sau do tang 1.5x moi lan, tran 5x `base` (poll co gioi han, AC-15). */
export function backoffInterval(base: number, attempts: number): number {
  return Math.min(Math.round(base * Math.pow(1.5, Math.max(0, attempts - 2))), base * 5);
}

/** Hook theo dõi trạng thái đơn hàng và polling khi webhook chậm (EVF-114, BR-O6).
 *
 * 1. BR-O6: Trạng thái PENDING là trạng thái bình thường khi cổng thanh toán chưa gửi webhook;
 *    tuyệt đối không hiển thị lỗi mà tiếp tục polling định kỳ.
 * 2. Dừng polling ngay khi chuyển sang PAID, FAILED, EXPIRED, CANCELLED.
 */
export function useOrderStatus({
  orderId,
  eventId,
  initialDetails = null,
  pollIntervalMs = 2000,
  maxPollAttempts = 15,
}: UseOrderStatusOptions): UseOrderStatusReturn {
  const [order, setOrder] = useState<OrderDetails | null>(initialDetails);
  const [loading, setLoading] = useState<boolean>(!initialDetails);
  const [pollAttempts, setPollAttempts] = useState<number>(0);
  const [error, setError] = useState<PaymentClientError | null>(null);

  const isPollingRef = useRef(false);
  /** Loi cuoi cung doc dong bo: timer da hen co the chay truoc khi React ke kip huy no (render chua flush). */
  const stopPollRef = useRef(false);

  const fetchStatus = useCallback(async () => {
    if (!orderId || !eventId) return;

    try {
      const details = await getOrderDetails(orderId, eventId);
      stopPollRef.current = false;
      setOrder(details);
      setError(null);
    } catch (err) {
      // Giữ nguyên order hiện có (không đè lỗi làm mất thông tin), chỉ ghi nhận lỗi để UI báo + thử lại
      const e = err instanceof PaymentClientError ? err : new PaymentClientError("Lỗi không xác định", "UNKNOWN");
      stopPollRef.current = e.type === "NOT_FOUND" || e.type === "AUTH_REQUIRED" || e.type === "FORBIDDEN";
      setError(e);
    } finally {
      setLoading(false);
    }
  }, [orderId, eventId]);

  // Initial fetch khi mount nếu chưa có initialDetails
  useEffect(() => {
    if (!initialDetails && orderId && eventId) {
      setLoading(true);
      fetchStatus();
    }
  }, [initialDetails, orderId, eventId, fetchStatus]);

  const currentStatus: OrderStatus = order?.status ?? "PENDING";
  const isPendingWebhook = currentStatus === "PENDING";

  // Polling khi dang PENDING
  useEffect(() => {
    if (!isPendingWebhook || !orderId || !eventId) {
      return;
    }

    // 404 / het phien: poll tiep khong giup gi.
    if (error?.type === "NOT_FOUND" || error?.type === "AUTH_REQUIRED" || error?.type === "FORBIDDEN") {
      return;
    }

    if (pollAttempts >= maxPollAttempts) {
      // Dừng polling tự động sau khi vượt quá số lần, giữ nguyên PENDING và cho phép người dùng refresh thủ công
      return;
    }

    const timer = setTimeout(async () => {
      if (isPollingRef.current || stopPollRef.current) return;
      isPollingRef.current = true;
      try {
        setPollAttempts((prev) => prev + 1);
        await fetchStatus();
      } finally {
        isPollingRef.current = false;
      }
    }, backoffInterval(pollIntervalMs, pollAttempts));

    return () => clearTimeout(timer);
  }, [isPendingWebhook, orderId, eventId, pollAttempts, maxPollAttempts, pollIntervalMs, fetchStatus, error]);

  const refresh = useCallback(async () => {
    setLoading(true);
    await fetchStatus();
  }, [fetchStatus]);

  return {
    order,
    status: currentStatus,
    loading,
    pollAttempts,
    isPendingWebhook,
    error,
    refresh,
  };
}

export interface UseIssuedTicketsReturn {
  tickets: IssuedTicketQR[];
  /** true khi chua co ve nao (dang lay hoac cho phat hanh) va con luot thu lai. */
  pending: boolean;
  /** true khi server tu choi (403 forbidden): khong thu lai. */
  forbidden: boolean;
  /** true khi da het luot thu lai ma van chua co ve: UI cho nguoi dung bam thu lai. */
  failed: boolean;
  retry: () => void;
}

/** Lay ve kem QR cho don PAID (AC-15). Idempotency-Key on dinh theo don nam trong `issueTickets`.
 *  Chua phat hanh (loi hoac danh sach rong) -> thu lai voi backoff co gioi han, kem `onRetry` de lam moi don.
 *  KHONG bao gio sinh QR demo. */
export function useIssuedTickets(
  orderId: string,
  paid: boolean,
  opts: { baseDelayMs?: number; maxAttempts?: number; onRetry?: () => void } = {},
): UseIssuedTicketsReturn {
  const { baseDelayMs = 2000, maxAttempts = 6, onRetry } = opts;
  const [tickets, setTickets] = useState<IssuedTicketQR[]>([]);
  const [attempts, setAttempts] = useState(0);
  const [forbidden, setForbidden] = useState(false);
  const [run, setRun] = useState(0);
  const attemptsRef = useRef(0);
  const onRetryRef = useRef(onRetry);
  onRetryRef.current = onRetry;

  useEffect(() => {
    if (!paid || !orderId) return;
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | null = null;

    issueTickets(orderId)
      .then((list) => {
        if (cancelled) return;
        if (list.length > 0) {
          setTickets(list);
          return;
        }
        throw new Error("not-issued-yet");
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        // Het phien / khong thay don: thu lai khong the thanh cong -> dung han, de UI hien nut thu lai thu cong (F-13).
        if (err instanceof PaymentClientError && (err.type === "AUTH_REQUIRED" || err.type === "NOT_FOUND" || err.type === "FORBIDDEN")) {
          if (err.type === "FORBIDDEN") setForbidden(true);
          attemptsRef.current = maxAttempts;
          setAttempts(maxAttempts);
          return;
        }
        // Dem lan that bai; con luot thi hen thu lai (khong dat side effect trong updater cua setState).
        const next = attemptsRef.current + 1;
        attemptsRef.current = next;
        setAttempts(next);
        if (next < maxAttempts) {
          timer = setTimeout(() => {
            onRetryRef.current?.();
            setRun((r) => r + 1);
          }, backoffInterval(baseDelayMs, next));
        }
      });

    return () => {
      cancelled = true;
      if (timer) clearTimeout(timer);
    };
  }, [orderId, paid, run, baseDelayMs, maxAttempts]);

  const retry = useCallback(() => {
    // Khoa cu co the da duoc middleware ghi nho cung phan hoi rong/loi: thu lai bang khoa moi (F-13).
    forgetIssueKey(orderId);
    attemptsRef.current = 0;
    setAttempts(0);
    setForbidden(false);
    setRun((r) => r + 1);
  }, [orderId]);

  const hasTickets = tickets.length > 0;
  const exhausted = attempts >= maxAttempts;
  return {
    tickets,
    pending: paid && !hasTickets && !exhausted,
    forbidden: paid && !hasTickets && forbidden,
    failed: paid && !hasTickets && exhausted && !forbidden,
    retry,
  };
}
