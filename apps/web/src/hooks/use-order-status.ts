"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import {
  getOrderDetails,
  type OrderDetails,
  type OrderStatus,
} from "@/lib/payment-client";

export interface UseOrderStatusOptions {
  orderId: string;
  eventId: string;
  initialDetails?: OrderDetails | null;
  pollIntervalMs?: number;
  maxPollAttempts?: number;
  apiBaseUrl?: string;
}

export interface UseOrderStatusReturn {
  order: OrderDetails | null;
  status: OrderStatus;
  loading: boolean;
  pollAttempts: number;
  isPendingWebhook: boolean;
  refresh: () => Promise<void>;
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
  apiBaseUrl,
}: UseOrderStatusOptions): UseOrderStatusReturn {
  const [order, setOrder] = useState<OrderDetails | null>(initialDetails);
  const [loading, setLoading] = useState<boolean>(!initialDetails);
  const [pollAttempts, setPollAttempts] = useState<number>(0);

  const isPollingRef = useRef(false);

  const fetchStatus = useCallback(async () => {
    if (!orderId || !eventId) return;

    try {
      const details = await getOrderDetails(orderId, eventId, apiBaseUrl);
      setOrder(details);
    } catch {
      // Giữ nguyên order hiện có, không đè lỗi làm mất thông tin
    } finally {
      setLoading(false);
    }
  }, [orderId, eventId, apiBaseUrl]);

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

    if (pollAttempts >= maxPollAttempts) {
      // Dừng polling tự động sau khi vượt quá số lần, giữ nguyên PENDING và cho phép người dùng refresh thủ công
      return;
    }

    const timer = setTimeout(async () => {
      if (isPollingRef.current) return;
      isPollingRef.current = true;
      try {
        setPollAttempts((prev) => prev + 1);
        await fetchStatus();
      } finally {
        isPollingRef.current = false;
      }
    }, pollIntervalMs);

    return () => clearTimeout(timer);
  }, [isPendingWebhook, orderId, eventId, pollAttempts, maxPollAttempts, pollIntervalMs, fetchStatus]);

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
    refresh,
  };
}
