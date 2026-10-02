import { forwardRef, type HTMLAttributes } from "react";

import { cn } from "@/lib/cn";

export type AlertVariant = "info" | "success" | "warning" | "error";

const SURFACE: Record<AlertVariant, string> = {
  info: "border-info bg-info-soft",
  success: "border-success bg-success-soft",
  // Vien dung --color-accent (amber). Chu KHONG dung accent — chi 2.19:1 tren trang,
  // nen title/icon canh bao dung --color-warning-fg (xem spec muc 3.2).
  warning: "border-accent bg-warning-soft",
  error: "border-danger bg-danger-soft",
};

const TITLE: Record<AlertVariant, string> = {
  info: "text-primary",
  success: "text-success",
  warning: "text-warning-fg",
  error: "text-danger",
};

const ICON: Record<AlertVariant, string> = {
  info: "i",
  success: "✓",
  warning: "!",
  error: "✕",
};

/** warning/error can duoc doc ngay (assertive) — dung role="alert".
 *  info/success chi la thong tin — dung role="status" de khong cat loi screen reader. */
function roleFor(variant: AlertVariant): "alert" | "status" {
  return variant === "warning" || variant === "error" ? "alert" : "status";
}

export interface AlertProps extends HTMLAttributes<HTMLDivElement> {
  variant?: AlertVariant;
  title?: string;
  onDismiss?: () => void;
}

export const Alert = forwardRef<HTMLDivElement, AlertProps>(function Alert(
  { variant = "info", title, onDismiss, className, children, ...rest },
  ref,
) {
  return (
    <div
      ref={ref}
      role={roleFor(variant)}
      className={cn(
        "flex items-start gap-sm rounded-control border p-md text-body-md text-fg",
        SURFACE[variant],
        className,
      )}
      {...rest}
    >
      <span aria-hidden="true" className={cn("text-label-lg", TITLE[variant])}>
        {ICON[variant]}
      </span>
      <div className="flex-1">
        {title ? <p className={cn("text-label-lg", TITLE[variant])}>{title}</p> : null}
        {children ? <div>{children}</div> : null}
      </div>
      {onDismiss ? (
        <button
          type="button"
          onClick={onDismiss}
          aria-label="Đóng thông báo"
          className="ef-focus-ring rounded-control px-xs text-fg-muted"
        >
          ✕
        </button>
      ) : null}
    </div>
  );
});
