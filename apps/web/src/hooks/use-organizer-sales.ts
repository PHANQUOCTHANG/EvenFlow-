"use client";

/** Hook điều phối dữ liệu doanh số realtime cho Ban tổ chức (EV-186, EVF-116).
 *
 * Tiêu chí nghiệm thu AC-4:
 * - Dữ liệu doanh số cập nhật trong <= 5s. Mặc định chu kỳ polling là 4000ms.
 * - Hỗ trợ bật/tắt tự động làm mới (autoRefresh).
 * - Cung cấp hàm làm mới thủ công (refresh) khi người dùng yêu cầu.
 * - Quản lý trạng thái loading, refreshing, lỗi và mốc thời gian cập nhật gần nhất.
 */

import { useCallback, useEffect, useRef, useState } from "react";

import {
  getRealtimeSales,
  type RealtimeSalesSnapshot,
} from "@/lib/organizer-service";

export interface UseOrganizerSalesOptions {
  eventId: string;
  pollingIntervalMs?: number;
  initialAutoRefresh?: boolean;
  initialData?: RealtimeSalesSnapshot | null;
}

export interface UseOrganizerSalesReturn {
  data: RealtimeSalesSnapshot | null;
  isLoading: boolean;
  isRefreshing: boolean;
  error: Error | null;
  lastUpdatedAt: Date | null;
  autoRefresh: boolean;
  setAutoRefresh: (enabled: boolean) => void;
  refresh: () => Promise<void>;
}

export const DEFAULT_SALES_POLLING_INTERVAL_MS = 4000; // <= 5s per AC-4

export function useOrganizerSales({
  eventId,
  pollingIntervalMs = DEFAULT_SALES_POLLING_INTERVAL_MS,
  initialAutoRefresh = true,
  initialData = null,
}: UseOrganizerSalesOptions): UseOrganizerSalesReturn {
  const [data, setData] = useState<RealtimeSalesSnapshot | null>(initialData);
  const [isLoading, setIsLoading] = useState<boolean>(!initialData);
  const [isRefreshing, setIsRefreshing] = useState<boolean>(false);
  const [error, setError] = useState<Error | null>(null);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<Date | null>(
    initialData ? new Date() : null,
  );
  const [autoRefresh, setAutoRefresh] = useState<boolean>(initialAutoRefresh);

  const isMountedRef = useRef<boolean>(true);

  // Giới hạn chu kỳ tối đa 5000ms theo đúng tiêu chí AC-4
  const safeIntervalMs = Math.min(pollingIntervalMs, 5000);

  const fetchSales = useCallback(
    async (isManual = false) => {
      if (!eventId) return;

      if (isManual) {
        setIsRefreshing(true);
      }

      try {
        const result = await getRealtimeSales(eventId);
        if (!isMountedRef.current) return;

        setData(result);
        setError(null);
        setLastUpdatedAt(new Date());
      } catch (err) {
        if (!isMountedRef.current) return;
        setError(err instanceof Error ? err : new Error(String(err)));
      } finally {
        if (isMountedRef.current) {
          setIsLoading(false);
          setIsRefreshing(false);
        }
      }
    },
    [eventId],
  );

  // Initial fetch
  useEffect(() => {
    isMountedRef.current = true;
    void fetchSales(false);

    return () => {
      isMountedRef.current = false;
    };
  }, [fetchSales]);

  // Polling timer
  useEffect(() => {
    if (!autoRefresh || !eventId) {
      return;
    }

    const timer = setInterval(() => {
      void fetchSales(false);
    }, safeIntervalMs);

    return () => {
      clearInterval(timer);
    };
  }, [autoRefresh, eventId, safeIntervalMs, fetchSales]);

  const refresh = useCallback(async () => {
    await fetchSales(true);
  }, [fetchSales]);

  return {
    data,
    isLoading,
    isRefreshing,
    error,
    lastUpdatedAt,
    autoRefresh,
    setAutoRefresh,
    refresh,
  };
}
