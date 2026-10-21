"use client";

import { useEffect, useState } from "react";
import Link from "next/link";

import { Alert } from "@/components/ui";
import { OrderSummary, type OrderSummaryItem } from "@/components/checkout/order-summary";
import {
  TicketTierCard,
  type TierAvailability,
} from "@/components/checkout/ticket-tier-card";
import { useHoldTimer } from "@/hooks/use-hold-timer";
import { useServerTimeSync } from "@/hooks/use-server-time-sync";
import { useT } from "@/components/i18n/locale-provider";
import { cn } from "@/lib/cn";
import { clearQueueToken } from "@/lib/queue-token";
import type { EventDetailSnapshot } from "@/lib/event-service";

export interface CheckoutViewProps {
  event: EventDetailSnapshot;
  initialQueueToken?: string | null;
  maxSelectablePerOrder?: number;
  onProceedToPayment?: (orderId: string, holdId: string) => void;
  className?: string;
}

/** URL sang trang thanh toan cho mot hold dang active (EV-198, B3).
 *
 * Phai mang theo hang ve va so luong: trang thanh toan (server component) lay gia tu
 * `tierId` + `quantity` tren URL, thieu thi no roi ve hang ve dau tien x 1 va hien sai so tien. */
export function buildPaymentUrl(
  eventId: string,
  hold: { orderId: string; holdId: string; ticketTypeId: string; quantity: number },
): string {
  const query = new URLSearchParams({
    orderId: hold.orderId,
    holdId: hold.holdId,
    tierId: hold.ticketTypeId,
    quantity: String(hold.quantity),
  });
  return `/checkout/${encodeURIComponent(eventId)}/payment?${query.toString()}`;
}

/** BR-A4: UNKNOWN (chua biet) KHONG duoc hien la "con ve": tra undefined de the khong khang dinh gi.
 *  Giu hang ve chon duoc vi ket qua giu ve moi la nguon dung. */
function mapTierAvailability(availability: string): TierAvailability | undefined {
  switch (availability) {
    case "FEW_LEFT":
      return "limited";
    case "SOLD_OUT":
      return "sold_out";
    case "AVAILABLE":
      return "available";
    default:
      return undefined;
  }
}

export function CheckoutView({
  event,
  initialQueueToken,
  maxSelectablePerOrder: maxSelectableProp,
  onProceedToPayment,
  className,
}: CheckoutViewProps) {
  const t = useT();
  // F-20: gioi han moi don theo su kien (max_per_order); thieu/0 -> 4.
  const maxSelectablePerOrder = maxSelectableProp ?? (event.maxPerOrder > 0 ? event.maxPerOrder : 4);
  const { offsetMs } = useServerTimeSync();
  const {
    hold,
    holdState,
    error,
    createHold,
    resetHold,
  } = useHoldTimer({
    eventId: event.id,
    queueToken: initialQueueToken,
    offsetMs,
  });

  const [selectedTierId, setSelectedTierId] = useState<string | null>(
    hold?.ticketTypeId ?? event.tiers[0]?.id ?? null,
  );
  const [quantity, setQuantity] = useState<number>(hold?.quantity ?? 1);

  // Dong bo tier dang chon voi active hold neu co
  const activeTierId = hold?.ticketTypeId ?? selectedTierId;
  const activeQuantity = hold ? hold.quantity : quantity;

  const selectedTier = event.tiers.find((t) => t.id === activeTierId);

  const orderItems: OrderSummaryItem[] =
    selectedTier && activeQuantity > 0
      ? [
          {
            tierName: selectedTier.name,
            quantity: activeQuantity,
            unitAmount: selectedTier.price,
            currency: "VND",
          },
        ]
      : [];

  const handlePrimaryAction = async () => {
    if (holdState === "idle") {
      if (!activeTierId || activeQuantity <= 0) return;
      await createHold(activeTierId, activeQuantity);
    } else if (holdState === "active" && hold) {
      if (onProceedToPayment) {
        onProceedToPayment(hold.orderId, hold.holdId);
      } else {
        // Mac dinh chuyen huong sang trang thanh toan (EVF-114)
        window.location.href = buildPaymentUrl(event.id, hold);
      }
    } else if (holdState === "expired") {
      resetHold();
      setQuantity(1);
    }
  };

  const isAdmittedError = error?.type === "NOT_ADMITTED";
  // Queue token het han/sai (AC-10): bo token cu de phong cho cap lai, roi dua nguoi dung ve phong cho.
  const isTokenInvalid = error?.type === "QUEUE_TOKEN_INVALID";
  const isAuthRequired = error?.type === "AUTH_REQUIRED";
  const limitMessage =
    error?.type === "PER_IDENTITY_LIMIT"
      ? t("checkout.perIdentityLimit")
      : error?.type === "QUANTITY_NOT_ALLOWED"
        ? t("checkout.quantityNotAllowed")
        : null;

  // F-5: moi loi khac chua co banner rieng phai hien thi (khong bao gio im lang).
  const showGenericError =
    !!error && !["NOT_ADMITTED", "QUEUE_TOKEN_INVALID", "AUTH_REQUIRED", "PER_IDENTITY_LIMIT", "QUANTITY_NOT_ALLOWED", "SOLD_OUT", "AMBIGUOUS"].includes(error.type);

  useEffect(() => {
    if (isTokenInvalid) clearQueueToken(event.id);
  }, [isTokenInvalid, event.id]);

  return (
    <div className={cn("mx-auto max-w-content space-y-lg py-md", className)}>
      {isTokenInvalid ? (
        <Alert variant="warning" title={t("checkout.tokenInvalidTitle")} className="mb-md">
          <p className="mb-sm">{t("checkout.tokenInvalidBody")}</p>
          <Link
            href={`/waiting/${event.id}`}
            className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-border-strong bg-surface px-md py-sm text-label-md text-fg hover:bg-surface-subtle"
          >
            {t("checkout.tokenInvalidAction")}
          </Link>
        </Alert>
      ) : null}

      {isAuthRequired ? (
        <Alert variant="error" title={t("checkout.authRequired")} className="mb-md">
          <Link
            href="/auth/sign-in"
            className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-border-strong bg-surface px-md py-sm text-label-md text-fg hover:bg-surface-subtle"
          >
            {t("checkout.signInAction")}
          </Link>
        </Alert>
      ) : null}

      {showGenericError ? (
        <Alert variant="error" className="mb-md">
          {error?.message}
        </Alert>
      ) : null}

      {limitMessage ? (
        <Alert variant="warning" className="mb-md">
          {limitMessage}
        </Alert>
      ) : null}

      {/* Banner canh bao chua duoc admit (AC-6) */}
      {isAdmittedError ? (
        <Alert
          variant="error"
          title="Chưa được cấp quyền mua vé (BR-Q4)"
          className="mb-md"
        >
          <p className="mb-sm">
            Bạn chưa có lượt vào mua vé hoặc phiên giữ chỗ đã hết hạn. Vui lòng quay lại phòng chờ để lấy số thứ tự.
          </p>
          <Link
            href={`/waiting/${event.id}`}
            className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-border-strong bg-surface px-md py-sm text-label-md text-fg hover:bg-surface-subtle"
          >
            Quay lại phòng chờ
          </Link>
        </Alert>
      ) : null}

      <header className="space-y-xs">
        <h1 className="text-display-sm text-fg">Chọn vé — {event.title}</h1>
        <p className="text-body-md text-fg-muted">{event.venue}</p>
      </header>

      <div className="grid grid-cols-1 items-start gap-lg lg:grid-cols-3">
        {/* Danh sach hang ve */}
        <section
          aria-label="Danh sách hạng vé"
          className="space-y-md lg:col-span-2"
        >
          <h2 className="text-headline-sm text-fg">Hạng vé sẵn có</h2>
          <p className="text-body-sm text-fg-muted">{t("checkout.availabilityIndicative")}</p>

          <div className="space-y-md">
            {event.tiers.map((tier) => {
              const isSelected = tier.id === activeTierId;
              const hasActiveHoldElsewhere =
                holdState === "active" &&
                hold !== null &&
                hold.ticketTypeId !== tier.id;

              return (
                <TicketTierCard
                  key={tier.id}
                  name={tier.name}
                  unitAmount={tier.price}
                  currency="VND"
                  availability={mapTierAvailability(tier.availability)}
                  maxSelectable={maxSelectablePerOrder}
                  quantity={isSelected ? activeQuantity : 0}
                  selected={isSelected}
                  disabled={holdState === "creating" || (holdState === "active" && !isSelected)}
                  hasActiveHoldElsewhere={hasActiveHoldElsewhere}
                  onQuantityChange={(nextQty) => {
                    if (holdState === "active") return; // Khoa sua so luong khi da active hold
                    setSelectedTierId(tier.id);
                    setQuantity(nextQty);
                  }}
                />
              );
            })}
          </div>
        </section>

        {/* Cot phai: Tom tat don hang + Dong ho giu ghe 10:00 (OrderSummary) */}
        <aside className="lg:col-span-1">
          <OrderSummary
            items={orderItems}
            holdState={holdState}
            holdExpiresAt={hold?.expiresAt}
            offsetMs={offsetMs}
            onPrimaryAction={handlePrimaryAction}
          />
        </aside>
      </div>
    </div>
  );
}
