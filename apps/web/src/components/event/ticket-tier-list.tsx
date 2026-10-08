import { Badge, type BadgeVariant } from "@/components/ui";
import { formatVnd } from "@/lib/format";
import type { TicketTierSnapshot, TierAvailability } from "@/lib/event-service";

export interface TicketTierListProps {
  tiers: TicketTierSnapshot[];
  className?: string;
}

const AVAILABILITY_COPY: Record<
  TierAvailability,
  { label: string; badge: BadgeVariant }
> = {
  AVAILABLE: {
    label: "Còn vé",
    badge: "admitted",
  },
  FEW_LEFT: {
    label: "Sắp hết vé",
    badge: "pending",
  },
  SOLD_OUT: {
    label: "Hết vé",
    badge: "soldout",
  },
};

/** Danh sach hang ve su kien (BR-A4).
 *
 * Tuyet doi khong hien thi so luong ton kho con lai chinh xac.
 * Chi hien thi dang khoang (Con ve, Sap het ve, Het ve) de bao ve he thong truoc bot. */
export function TicketTierList({ tiers, className }: TicketTierListProps) {
  if (!tiers || tiers.length === 0) {
    return (
      <p className="text-body-md text-fg-muted italic">
        Chưa có thông tin hạng vé cho sự kiện này.
      </p>
    );
  }

  return (
    <div className={className}>
      <h2 className="text-heading-md font-semibold text-fg mb-md">
        Các hạng vé
      </h2>
      <ul className="space-y-md list-none p-0 m-0" aria-label="Danh sách hạng vé">
        {tiers.map((tier) => {
          const status = AVAILABILITY_COPY[tier.availability];

          return (
            <li
              key={tier.id}
              className="flex flex-col sm:flex-row sm:items-center justify-between p-md rounded-container border border-border bg-surface gap-sm"
              data-tier-id={tier.id}
            >
              <div className="flex-1">
                <div className="flex items-center gap-sm">
                  <span className="font-semibold text-fg text-body-lg">
                    {tier.name}
                  </span>
                  <Badge variant={status.badge}>{status.label}</Badge>
                </div>
                {tier.description && (
                  <p className="text-body-sm text-fg-muted mt-xs">
                    {tier.description}
                  </p>
                )}
              </div>

              <div className="text-left sm:text-right mt-xs sm:mt-0">
                <span className="text-numeric-price font-bold text-fg block">
                  {formatVnd(tier.price)}
                </span>
              </div>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
