"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import type { EventPublicState } from "@/components/event/event-card";
import { Button, ServerExpiryCountdown, Spinner } from "@/components/ui";
import { useServerCountdown } from "@/hooks/use-server-countdown";
import { useServerTimeSync } from "@/hooks/use-server-time-sync";
import {
  type JitterOptions,
  type JitterProgress,
  sleepWithJitter,
} from "@/lib/jitter";

export interface EventActionPanelProps {
  eventId: string;
  state: EventPublicState;
  saleStartAt: string;
  onJoinQueue?: (eventId: string) => void;
  jitterOptions?: JitterOptions;
  className?: string;
}

/** Panel hanh dong cho trang su kien: Dong ho dem nguoc gio server + Nut vao phong cho kem Jitter (EVF-111).
 *
 * Tuan thu tuyet doi:
 * 1. BR-O2: Dong ho dong bo theo gio server (qua useServerTimeSync + useServerCountdown).
 * 2. BR-Q5 / EVF-61: Ap dung Jitter 0..5s tai T0 de triet tieu xung dot 300k rps thanh ~60k rps.
 * 3. Accessibility: aria-busy, aria-live thong bao trang thai cho screen reader. */
export function EventActionPanel({
  eventId,
  state,
  saleStartAt,
  onJoinQueue,
  jitterOptions,
  className,
}: EventActionPanelProps) {
  const router = useRouter();
  const { offsetMs } = useServerTimeSync();
  const { expired, ready } = useServerCountdown(saleStartAt, offsetMs);

  const [isJittering, setIsJittering] = useState(false);
  const [jitterProgress, setJitterProgress] = useState<JitterProgress | null>(null);

  // Da mo ban khi backend danh dau ON_SALE hoac su kien SCHEDULED da vuot qua moc saleStartAt tren server
  const isTimeForSale = state === "ON_SALE" || (state === "SCHEDULED" && ready && expired);

  const handleAction = async () => {
    if (!isTimeForSale || isJittering) return;

    setIsJittering(true);

    await sleepWithJitter((p) => setJitterProgress(p), jitterOptions);

    setIsJittering(false);

    if (onJoinQueue) {
      onJoinQueue(eventId);
    } else {
      router.push(`/waiting/${eventId}`);
    }
  };

  // 1. Cac trang thai dong/huy/het ve
  if (state === "SOLD_OUT") {
    return (
      <div className={className}>
        <Button variant="secondary" disabled className="w-full">
          Đã hết vé
        </Button>
      </div>
    );
  }

  if (state === "CANCELLED") {
    return (
      <div className={className}>
        <Button variant="secondary" disabled className="w-full">
          Sự kiện đã huỷ
        </Button>
      </div>
    );
  }

  if (state === "CLOSED" || state === "COMPLETED") {
    return (
      <div className={className}>
        <Button variant="secondary" disabled className="w-full">
          Đã đóng bán
        </Button>
      </div>
    );
  }

  // 2. Trang thai dang chay Jitter tai T0
  if (isJittering) {
    return (
      <div
        className={className}
        aria-busy="true"
        aria-live="polite"
        data-testid="jitter-state"
      >
        <div className="flex flex-col items-center justify-center p-md rounded-container border border-border bg-surface text-center gap-sm">
          <Spinner size="md" />
          <p className="text-body-md font-medium text-fg">
            Đang điều phối lượt vào phòng chờ...
          </p>
          <p className="text-body-xs text-fg-muted">
            {jitterProgress
              ? `Còn ${((jitterProgress.remainingMs || 0) / 1000).toFixed(1)}s (giảm tải đột biến)`
              : "Vui lòng giữ nguyên màn hình..."}
          </p>
        </div>
      </div>
    );
  }

  // 3. Su kien SCHEDULED va chua toi gio mo ban
  if (!isTimeForSale) {
    return (
      <div className={className}>
        <div className="flex flex-col sm:flex-row items-center justify-between gap-md p-md rounded-container border border-border bg-surface">
          <div>
            <span className="text-label-md text-fg-muted block mb-xs">
              Thời gian mở bán
            </span>
            <ServerExpiryCountdown
              variant="sale-start"
              expiresAt={saleStartAt}
              offsetMs={offsetMs}
            />
          </div>

          <Button variant="secondary" disabled className="w-full sm:w-auto">
            Chưa tới giờ mở bán
          </Button>
        </div>
      </div>
    );
  }

  // 4. Su kien da toi gio mo ban (ON_SALE hoac SCHEDULED da qua T0)
  return (
    <div className={className}>
      <Button
        variant="primary"
        size="lg"
        onClick={handleAction}
        className="w-full font-bold shadow-md"
      >
        Vào phòng chờ mua vé
      </Button>
    </div>
  );
}
