import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "@/lib/cn";

/** Trang thai nghiep vu EventFlow. Giu dung ten semantic cua backend
 *  (LOBBY / QUEUED / ADMITTED / EXPIRED / SOLD_OUT) theo handoff muc 7. */
export type BadgeVariant =
  | "lobby"
  | "queued"
  | "admitted"
  | "holding"
  | "paid"
  | "pending"
  | "expired"
  | "soldout"
  | "failed"
  | "neutral";

const VARIANT: Record<BadgeVariant, string> = {
  lobby: "bg-neutral-soft text-fg-muted",
  queued: "bg-info-soft text-primary",
  admitted: "bg-success-soft text-success",
  holding: "bg-warning-soft text-warning-fg",
  paid: "bg-success-soft text-success",
  pending: "bg-warning-soft text-warning-fg",
  expired: "bg-neutral-soft text-fg-muted",
  soldout: "bg-neutral-soft text-fg-muted",
  failed: "bg-danger-soft text-danger",
  neutral: "bg-neutral-soft text-fg-muted",
};

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: BadgeVariant;
  /** Bat buoc: badge KHONG BAO GIO truyen tin chi bang mau (handoff muc 5 nguyen tac 7). */
  children: React.ReactNode;
}

/** Pill la ngoai le duy nhat duoc dung full-radius trong he thong (DESIGN.md muc Shapes). */
export const Badge = forwardRef<HTMLSpanElement, BadgeProps>(function Badge(
  { variant = "neutral", className, children, ...rest },
  ref,
) {
  return (
    <span
      ref={ref}
      className={cn(
        "inline-flex items-center gap-xs rounded-full px-sm py-xs text-label-sm uppercase",
        VARIANT[variant],
        className,
      )}
      {...rest}
    >
      {children}
    </span>
  );
});
