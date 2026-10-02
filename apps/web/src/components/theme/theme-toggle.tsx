"use client";

import { useTheme } from "@/hooks/use-theme";
import { cn } from "@/lib/cn";

export function ThemeToggle({ className }: { className?: string }) {
  const { theme, toggle } = useTheme();
  const isDark = theme === "dark";

  return (
    <button
      type="button"
      onClick={toggle}
      aria-label="Chuyển giao diện sáng/tối"
      aria-pressed={isDark}
      className={cn(
        "ef-focus-ring inline-flex h-11 items-center gap-sm rounded-control border border-border-strong",
        "bg-surface px-md text-label-lg text-fg transition-colors hover:bg-surface-subtle",
        className,
      )}
    >
      <span aria-hidden="true">{isDark ? "☾" : "☀"}</span>
      <span>{isDark ? "Tối" : "Sáng"}</span>
    </button>
  );
}
