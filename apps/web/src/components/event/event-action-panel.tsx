"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";

import { useLocale, useT } from "@/components/i18n/locale-provider";
import { Button, ServerExpiryCountdown, Spinner } from "@/components/ui";
import { useServerCountdown } from "@/hooks/use-server-countdown";
import { useServerTimeSync } from "@/hooks/use-server-time-sync";
import type { EventStatus } from "@/lib/types/event";
import {
  type JitterOptions,
  type JitterProgress,
  sleepWithJitter,
} from "@/lib/jitter";

export interface EventActionPanelProps {
  eventId: string;
  state: EventStatus;
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
  const t = useT();
  const { locale } = useLocale();
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
      <div className={className} lang={locale}>
        <Button variant="secondary" disabled className="w-full">
          {t("eventAction.soldOut")}
        </Button>
      </div>
    );
  }

  if (state === "CANCELLED") {
    return (
      <div className={className} lang={locale}>
        <Button variant="secondary" disabled className="w-full">
          {t("eventAction.cancelled")}
        </Button>
      </div>
    );
  }

  // CLOSED / COMPLETED va moi trang thai khong ban duoc (DRAFT, PENDING_REVIEW, REJECTED) deu hien "da dong".
  if (state !== "SCHEDULED" && state !== "ON_SALE") {
    return (
      <div className={className} lang={locale}>
        <Button variant="secondary" disabled className="w-full">
          {t("eventAction.closed")}
        </Button>
      </div>
    );
  }

  // 2. Trang thai dang chay Jitter tai T0
  if (isJittering) {
    return (
      <div
        className={className}
        lang={locale}
        aria-busy="true"
        aria-live="polite"
        data-testid="jitter-state"
      >
        <div className="flex flex-col items-center justify-center p-md rounded-container border border-border bg-surface text-center gap-sm">
          {/* Nhan mac dinh cua Spinner la tieng Viet ("Dang tai"), chua dich. */}
          <Spinner size="md" lang="vi" />
          <p className="text-body-md font-medium text-fg">
            {t("eventAction.dispatching")}
          </p>
          <p className="text-body-xs text-fg-muted">
            {jitterProgress
              ? t("eventAction.jitterRemaining", {
                  seconds: ((jitterProgress.remainingMs || 0) / 1000).toFixed(1),
                })
              : t("eventAction.keepScreen")}
          </p>
        </div>
      </div>
    );
  }

  // 3. Su kien SCHEDULED va chua toi gio mo ban
  if (!isTimeForSale) {
    return (
      <div className={className} lang={locale}>
        <div className="flex flex-col sm:flex-row items-center justify-between gap-md p-md rounded-container border border-border bg-surface">
          <div>
            <span className="text-label-md text-fg-muted block mb-xs">
              {t("eventAction.saleStartsAt")}
            </span>
            {/* Dong ho dem nguoc chua dich (chu tieng Viet cung) nen khai bao lang="vi" trong goc lang={locale}. */}
            <div lang="vi">
              <ServerExpiryCountdown
                variant="sale-start"
                expiresAt={saleStartAt}
                offsetMs={offsetMs}
              />
            </div>
          </div>

          <Button variant="secondary" disabled className="w-full sm:w-auto">
            {t("eventAction.notYetOpen")}
          </Button>
        </div>
      </div>
    );
  }

  // 4. Su kien da toi gio mo ban (ON_SALE hoac SCHEDULED da qua T0)
  return (
    <div className={className} lang={locale}>
      <Button
        variant="primary"
        size="lg"
        onClick={handleAction}
        className="w-full font-bold shadow-md"
      >
        {t("eventAction.joinQueue")}
      </Button>
    </div>
  );
}
