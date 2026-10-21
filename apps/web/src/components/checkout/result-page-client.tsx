"use client";

import Link from "next/link";

import { useT } from "@/components/i18n/locale-provider";
import { OrderResultView } from "@/components/checkout/order-result-view";
import { EventLoader } from "@/components/event/event-loader";
import { Alert } from "@/components/ui";

/** Trang ket qua don hang: event tai phia client; khong co orderId thi bao loi, khong bia ma don (AC-18). */
export function ResultPageClient({ eventId, orderId }: { eventId: string; orderId?: string }) {
  const t = useT();
  return (
    <EventLoader eventId={eventId}>
      {(event) =>
        orderId ? (
          <OrderResultView eventId={event.id} orderId={orderId} />
        ) : (
          <div className="mx-auto max-w-content py-md">
            <Alert variant="error" title={t("result.missingOrderTitle")}>
              <p className="mb-sm">{t("result.missingOrderBody")}</p>
              <Link
                href={`/checkout/${event.id}`}
                className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-border-strong bg-surface px-md py-sm text-label-md text-fg hover:bg-surface-subtle"
              >
                {t("payment.backToCheckout")}
              </Link>
            </Alert>
          </div>
        )
      }
    </EventLoader>
  );
}
