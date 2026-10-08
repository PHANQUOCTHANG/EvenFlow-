"use client";

import { Alert, Badge, Button, type BadgeVariant } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatMoney } from "@/lib/money";

/** CHI 3 nhan roi rac, KHONG co so ve con lai.
 *
 * BR-O1: Redis lech so KHONG the gay oversell, chi co the gay "bao het ve som", va job doi
 * soat se sua lai. Cong handoff muc 3: so luong ve public chi nen la snapshot/nhan khai quat,
 * khong query DB theo moi luot xem. Mot con so sai som 30 giay la loi hua sai voi khach, va
 * handoff muc 6 cam thang badge "con 1 ve" neu khong co nguon chinh xac.
 *
 * Vi vay component nay khong co prop so ve con lai — khong phai de don gian hoa. */
export type TierAvailability = "available" | "limited" | "sold_out";

const AVAILABILITY: Record<TierAvailability, { badge: BadgeVariant; text: string }> = {
  available: { badge: "admitted", text: "Còn vé" },
  // Dinh tinh, KHONG kem con so.
  limited: { badge: "holding", text: "Sắp hết" },
  sold_out: { badge: "soldout", text: "Hết vé" },
};

export interface TicketTierCardProps {
  name: string;
  unitAmount: number;
  currency: string;
  /** Thieu -> KHONG khang dinh con ve. */
  availability?: TierAvailability;
  /**
   * = min(max_per_order, max_per_identity - da mua), do SERVER tinh (BR-O4).
   *
   * Khong co default: "mac dinh 4" la default cua HE THONG, khong phai thu UI duoc phep doan,
   * va so thu hai phu thuoc lich su mua ma UI khong biet. Thieu prop -> khong cho chon so luong.
   */
  maxSelectable?: number;
  quantity?: number;
  onQuantityChange?: (next: number) => void;
  selected?: boolean;
  disabled?: boolean;
  /** BR-O3: moi khach chi co 1 hold hoat dong tren moi su kien. Dang giu ve o hang khac thi
   *  dat giu o hang nay se nhan lai DUNG hold dang co (hang ve + so luong cu), khong doi
   *  duoc cho toi khi thanh toan xong hoac hold het han. Phai noi truoc, khong thi khach chon
   *  hang nay roi nhan ve hang cu ma khong hieu vi sao. */
  hasActiveHoldElsewhere?: boolean;
  className?: string;
}

export function TicketTierCard({
  name,
  unitAmount,
  currency,
  availability,
  maxSelectable,
  quantity = 0,
  onQuantityChange,
  selected = false,
  disabled = false,
  hasActiveHoldElsewhere = false,
  className,
}: TicketTierCardProps) {
  const soldOut = availability === "sold_out";
  const blocked = disabled || soldOut;

  const limit = Number.isInteger(maxSelectable) ? (maxSelectable as number) : null;
  const canSelect = !blocked && limit !== null && limit > 0;

  const current = Number.isInteger(quantity) ? Math.min(Math.max(quantity, 0), limit ?? 0) : 0;

  function change(next: number) {
    if (!canSelect) return;
    const clamped = Math.min(Math.max(next, 0), limit as number);
    if (clamped === current) return;
    onQuantityChange?.(clamped);
  }

  return (
    <div
      data-state={soldOut ? "sold_out" : blocked ? "disabled" : "available"}
      className={cn(
        "flex flex-col gap-sm rounded-card border bg-surface p-md shadow-card",
        selected ? "border-primary" : "border-border",
        blocked && "opacity-60",
        className,
      )}
    >
      <div className="flex flex-wrap items-center justify-between gap-sm">
        <h3 className="text-headline-sm text-fg">{name}</h3>
        {availability ? (
          <Badge variant={AVAILABILITY[availability].badge}>{AVAILABILITY[availability].text}</Badge>
        ) : (
          <Badge variant="neutral">Đang cập nhật</Badge>
        )}
      </div>

      <p className="text-numeric-metric tabular-nums text-fg">
        {formatMoney(unitAmount, currency)}
      </p>

      {hasActiveHoldElsewhere ? (
        <Alert variant="warning" title="Bạn đang giữ vé ở hạng khác">
          Mỗi khách chỉ giữ được một lượt vé cho sự kiện này. Hoàn tất thanh toán phần đang giữ,
          hoặc đợi nó hết hạn, rồi mới chọn được hạng vé khác.
        </Alert>
      ) : null}

      {canSelect ? (
        <div className="flex items-center gap-sm">
          <Button
            variant="secondary"
            size="sm"
            aria-label={`Giảm số lượng ${name}`}
            disabled={current <= 0}
            onClick={() => change(current - 1)}
          >
            −
          </Button>

          {/* aria-live de screen reader doc so moi sau khi bam, khong phai tu di tim. */}
          <span
            role="status"
            aria-live="polite"
            aria-label={`Số lượng ${name}`}
            className="min-w-10 text-center text-numeric-timer tabular-nums text-fg"
          >
            {current}
          </span>

          <Button
            variant="secondary"
            size="sm"
            aria-label={`Tăng số lượng ${name}`}
            disabled={current >= (limit as number)}
            onClick={() => change(current + 1)}
          >
            +
          </Button>

          <span className="text-body-sm text-fg-muted">Tối đa {limit} vé</span>
        </div>
      ) : null}

      {/* Trang thai da chon phai den duoc screen reader, khong chi bang vien 2px.
        *
        * Nut nay KHONG phu thuoc `maxSelectable`: chon hang ve va chon so luong la hai hanh dong
        * khac nhau. Mot the co the dang o trang thai "da chon" trong khi server chua tra ve gioi
        * han so luong — luc do van phai co tin hieu doc duoc, va `aria-pressed` phai la "false"
        * tuong minh chu khong phai vang mat (vang mat = screen reader khong biet day la nut hai
        * trang thai). */}
      {!blocked ? (
        <Button
          variant={selected ? "primary" : "tertiary"}
          aria-pressed={selected}
          onClick={() => change(current > 0 ? current : 1)}
        >
          {selected ? "Đã chọn hạng này" : "Chọn hạng này"}
        </Button>
      ) : null}
    </div>
  );
}
