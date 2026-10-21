"use client";

import Link from "next/link";
import { useEffect, useState } from "react";

import { useT } from "@/components/i18n/locale-provider";
import { PaymentHandoffView } from "@/components/checkout/payment-handoff-view";
import { EventLoader } from "@/components/event/event-loader";
import { Alert } from "@/components/ui";
import { getActiveHold } from "@/lib/hold-client";

export interface PaymentPageClientProps {
  eventId: string;
  orderId?: string;
  holdId?: string;
  tierId?: string;
  quantity?: string;
}

/** Trang thanh toan: event tai phia client; thieu thong tin don/hold/hang ve thi bao loi, khong bia id (AC-18). */
export function PaymentPageClient({ eventId, orderId, holdId, tierId, quantity }: PaymentPageClientProps) {
  const t = useT();
  // Han giu ve do server cap, doc tu hold luu cuc bo sau khi mount (khong lech hydration).
  const [expiresAt, setExpiresAt] = useState<string | undefined>(undefined);
  useEffect(() => {
    const hold = getActiveHold(eventId);
    if (hold && hold.holdId === holdId) setExpiresAt(hold.expiresAt);
  }, [eventId, holdId]);

  const parsedQuantity = quantity ? parseInt(quantity, 10) : 1;

  return (
    <EventLoader eventId={eventId}>
      {(event) => {
        const tier = event.tiers.find((x) => x.id === tierId);
        if (!orderId || !holdId || !tier) {
          return (
            <div className="mx-auto max-w-content py-md">
              <Alert variant="error" title={t("payment.missingOrderTitle")}>
                <p className="mb-sm">{t("payment.missingOrderBody")}</p>
                <Link
                  href={`/checkout/${event.id}`}
                  className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-border-strong bg-surface px-md py-sm text-label-md text-fg hover:bg-surface-subtle"
                >
                  {t("payment.backToCheckout")}
                </Link>
              </Alert>
            </div>
          );
        }
        return (
          <PaymentHandoffView
            eventId={event.id}
            eventTitle={event.title}
            venue={event.venue}
            orderId={orderId}
            holdId={holdId}
            tierName={tier.name}
            quantity={parsedQuantity > 0 ? parsedQuantity : 1}
            unitPrice={tier.price}
            expiresAt={expiresAt}
          />
        );
      }}
    </EventLoader>
  );
}
