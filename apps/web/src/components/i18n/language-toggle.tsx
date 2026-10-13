"use client";

import { cn } from "@/lib/cn";
import type { Locale } from "@/lib/i18n/messages";

import { useLocale, useT } from "./locale-provider";

/** Ten rieng cua moi ngon ngu: KHONG dich theo locale va KHONG co aria-label, de ten truy cap
 *  chinh la chu hien thi (WCAG 2.5.3 Label in Name); `lang` de screen reader doc dung giong. */
/** `short` la chu hien thi o man hep (<md); ten day du la sr-only, nhan ngan KHONG aria-hidden nen ten truy cap
 *  la "Tiếng Việt VI" / "English EN" — luon chua chu hien thi o moi kich thuoc (WCAG 2.5.3). */
const OPTIONS: ReadonlyArray<{ locale: Locale; label: string; short: string }> = [
  { locale: "vi", label: "Tiếng Việt", short: "VI" },
  { locale: "en", label: "English", short: "EN" },
];

/** Nhom hai nut chon ngon ngu, aria-pressed cho biet ngon ngu hien tai. */
export function LanguageToggle({ className }: { className?: string }) {
  const { locale, setLocale } = useLocale();
  const t = useT();

  return (
    <div role="group" aria-label={t("language.group")} className={cn("flex items-center gap-xs", className)}>
      {OPTIONS.map((option) => {
        const pressed = option.locale === locale;
        return (
          <button
            key={option.locale}
            type="button"
            lang={option.locale}
            aria-pressed={pressed}
            onClick={() => {
              if (!pressed) setLocale(option.locale);
            }}
            className={cn(
              "ef-focus-ring inline-flex h-11 items-center rounded-control border px-sm text-label-lg transition-colors md:px-md",
              pressed
                ? "border-transparent bg-primary text-primary-fg"
                : "border-border-strong bg-surface text-fg hover:bg-surface-subtle",
            )}
          >
            <span className="sr-only md:not-sr-only">{option.label}</span>
            <span className="md:hidden">
              {option.short}
            </span>
          </button>
        );
      })}
    </div>
  );
}
