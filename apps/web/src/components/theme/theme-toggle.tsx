"use client";

import { useT } from "@/components/i18n/locale-provider";
import { useTheme } from "@/hooks/use-theme";
import { cn } from "@/lib/cn";

export function ThemeToggle({ className }: { className?: string }) {
  const { theme, toggle } = useTheme();
  const isDark = theme === "dark";
  const t = useT();

  return (
    <button
      type="button"
      onClick={toggle}
      // aria-label phai chua chu hien thi (Sang/Toi, Light/Dark) — WCAG 2.5.3 Label in Name.
      aria-label={t("theme.toggle")}
      aria-pressed={isDark}
      className={cn(
        "ef-focus-ring inline-flex h-11 items-center gap-sm rounded-control border border-border-strong",
        "bg-surface px-md text-label-lg text-fg transition-colors hover:bg-surface-subtle",
        className,
      )}
    >
      <span aria-hidden="true">{isDark ? "☾" : "☀"}</span>
      <span>{isDark ? t("theme.dark") : t("theme.light")}</span>
    </button>
  );
}
