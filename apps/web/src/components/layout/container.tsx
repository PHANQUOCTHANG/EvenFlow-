import type { HTMLAttributes } from "react";

import { cn } from "@/lib/cn";

/** DESIGN.md muc Layout: 1200px cho luong dat ve (thu hep tam mat), 1440px cho dashboard
 *  van hanh (can cho nhieu cot du lieu hon). */
export type ContainerVariant = "public" | "workspace";

const MAX_WIDTH: Record<ContainerVariant, string> = {
  public: "max-w-[1200px]",
  workspace: "max-w-[1440px]",
};

export interface ContainerProps extends HTMLAttributes<HTMLDivElement> {
  variant?: ContainerVariant;
  as?: "div" | "main" | "section";
}

export function Container({
  variant = "public",
  as: Tag = "div",
  className,
  children,
  ...rest
}: ContainerProps) {
  return (
    <Tag
      className={cn("mx-auto w-full px-gutter md:px-margin", MAX_WIDTH[variant], className)}
      {...rest}
    >
      {children}
    </Tag>
  );
}
