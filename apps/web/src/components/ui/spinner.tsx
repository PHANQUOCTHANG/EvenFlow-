import type { SVGAttributes } from "react";

import { cn } from "@/lib/cn";

export type SpinnerSize = "sm" | "md" | "lg";

const SIZE: Record<SpinnerSize, string> = {
  sm: "size-4",
  md: "size-5",
  lg: "size-6",
};

export interface SpinnerProps extends SVGAttributes<SVGSVGElement> {
  size?: SpinnerSize;
  label?: string;
  /** Spinner nam BEN TRONG mot control da tu thong bao trang thai (vi du Button co
   *  aria-busy). Khi do spinner phai la trang tri: neu giu role="status" + aria-label thi
   *  aria-label do se nhap vao accessible name cua control cha, bien "Mua ve" thanh
   *  "Dang xu ly Mua ve" va lam vo moi truy van theo ten. */
  decorative?: boolean;
}

/** Chi bao dang tai. Dung `currentColor` nen tu an theo mau chu cua phan tu cha —
 *  khong hard-code mau. Animation tat o che do prefers-reduced-motion (xem .ef-spin). */
export function Spinner({
  size = "md",
  label = "Đang tải",
  decorative = false,
  className,
  ...rest
}: SpinnerProps) {
  return (
    <svg
      role={decorative ? undefined : "status"}
      aria-label={decorative ? undefined : label}
      aria-hidden={decorative ? true : undefined}
      viewBox="0 0 24 24"
      fill="none"
      className={cn("ef-spin", SIZE[size], className)}
      {...rest}
    >
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeOpacity="0.25" strokeWidth="3" />
      <path d="M21 12a9 9 0 0 0-9-9" stroke="currentColor" strokeWidth="3" strokeLinecap="round" />
    </svg>
  );
}
