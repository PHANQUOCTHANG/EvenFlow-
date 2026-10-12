"use client";

import { useId } from "react";
import { cn } from "@/lib/cn";

export interface ProgressRingProps {
  /** Rank hiện tại do server trả về. */
  rank: number | null;
  /** Rank ban đầu lúc vào hàng đợi (mẫu số cố định). */
  initialRank: number | null;
  size?: number;
  strokeWidth?: number;
  className?: string;
}

/** Vòng tiến trình trực quan cho hàng đợi (EV-182 / AC-5).
 *
 * Nguyên tắc bất biến:
 * 1. Không vẽ tiến trình giả: nếu thiếu initialRank hoặc rank không hợp lệ, không render (tránh thanh tiến trình 100% rởm).
 * 2. Tiến trình không nhảy ngược: cleared = initialRank - rank (chỉ tăng khi rank giảm).
 * 3. Accessibility: role="progressbar", aria-valuemin, aria-valuemax, aria-valuenow, aria-valuetext. */
export function ProgressRing({
  rank,
  initialRank,
  size = 120,
  strokeWidth = 10,
  className,
}: ProgressRingProps) {
  const titleId = useId();

  // Kiểm tra tính hợp lệ
  const isValidInitial = typeof initialRank === "number" && Number.isInteger(initialRank) && initialRank > 0;
  const isValidRank = typeof rank === "number" && Number.isInteger(rank) && rank >= 0;

  if (!isValidInitial || !isValidRank) {
    return null;
  }

  // Số lượng đã được giải tỏa kể từ lúc người dùng vào hàng
  const cleared = Math.min(initialRank, Math.max(0, initialRank - rank));
  const percent = Math.min(100, Math.max(0, Math.round((cleared / initialRank) * 100)));

  const radius = (size - strokeWidth) / 2;
  const circumference = 2 * Math.PI * radius;
  const strokeDashoffset = circumference - (percent / 100) * circumference;

  return (
    <div
      role="progressbar"
      aria-labelledby={titleId}
      aria-valuemin={0}
      aria-valuemax={initialRank}
      aria-valuenow={cleared}
      aria-valuetext={`Đã tiến ${cleared} trong ${initialRank} người xếp trước bạn (${percent}%)`}
      className={cn("relative inline-flex items-center justify-center", className)}
      style={{ width: size, height: size }}
    >
      <span id={titleId} className="sr-only">
        Tiến trình hàng đợi
      </span>
      <svg
        width={size}
        height={size}
        className="rotate-[-90deg] transition-all duration-500 ease-out"
        aria-hidden="true"
      >
        {/* Track nền */}
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          strokeWidth={strokeWidth}
          className="stroke-neutral-soft fill-none"
        />
        {/* Indicator */}
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          strokeWidth={strokeWidth}
          strokeDasharray={circumference}
          strokeDashoffset={strokeDashoffset}
          strokeLinecap="round"
          className="stroke-primary fill-none transition-[stroke-dashoffset] duration-500"
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center text-center">
        <span className="text-headline-sm font-bold text-fg tabular-nums">{percent}%</span>
        <span className="text-body-xs text-fg-muted">tiến trình</span>
      </div>
    </div>
  );
}
