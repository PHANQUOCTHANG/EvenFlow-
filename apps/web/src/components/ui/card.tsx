import { forwardRef, type HTMLAttributes, type ReactNode } from "react";

import { cn } from "@/lib/cn";

export interface CardProps extends HTMLAttributes<HTMLDivElement> {
  header?: ReactNode;
  footer?: ReactNode;
}

/** Elevation cap 1 theo DESIGN.md: nen surface + vien 1px + bong rat nhe, radius 16px.
 *  Slot vang thi KHONG render the rong (tranh vien/padding du thua). */
export const Card = forwardRef<HTMLDivElement, CardProps>(function Card(
  { header, footer, children, className, ...rest },
  ref,
) {
  return (
    <div
      ref={ref}
      className={cn("rounded-card border border-border bg-surface shadow-card", className)}
      {...rest}
    >
      {header ? (
        <div data-slot="header" className="border-b border-border px-md py-sm text-headline-sm">
          {header}
        </div>
      ) : null}
      <div data-slot="body" className="p-md">
        {children}
      </div>
      {footer ? (
        <div data-slot="footer" className="border-t border-border px-md py-sm text-body-md text-fg-muted">
          {footer}
        </div>
      ) : null}
    </div>
  );
});
