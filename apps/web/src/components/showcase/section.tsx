import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

export function Section({
  title,
  description,
  children,
  className,
}: {
  title: string;
  description?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={cn("flex flex-col gap-md", className)}>
      <div className="flex flex-col gap-xs">
        <h2 className="text-headline-lg text-fg">{title}</h2>
        {description ? <p className="text-body-md text-fg-muted">{description}</p> : null}
      </div>
      {children}
    </section>
  );
}

export function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-sm">
      <span className="text-label-md text-fg-muted">{label}</span>
      <div className="flex flex-wrap items-end gap-sm">{children}</div>
    </div>
  );
}
