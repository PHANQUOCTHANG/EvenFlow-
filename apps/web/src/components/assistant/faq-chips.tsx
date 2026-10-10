"use client";

import React from "react";
import { DEFAULT_FAQ_ITEMS, type FaqItem } from "@/lib/assistant-client";
import { cn } from "@/lib/cn";

export interface FaqChipsProps {
  items?: FaqItem[];
  onSelectFaq: (faq: FaqItem) => void;
  disabled?: boolean;
  selectedId?: string | null;
  className?: string;
}

export function FaqChips({
  items = DEFAULT_FAQ_ITEMS,
  onSelectFaq,
  disabled = false,
  selectedId,
  className,
}: FaqChipsProps) {
  if (!items || items.length === 0) {
    return null;
  }

  return (
    <section
      aria-label="Câu hỏi thường gặp"
      className={cn("flex flex-col gap-xs w-full", className)}
    >
      <div className="flex items-center justify-between text-label-sm text-fg-muted px-xxs">
        <span>Gợi ý câu hỏi nhanh (0 token):</span>
      </div>

      <div
        role="group"
        aria-label="Danh sách gợi ý câu hỏi"
        className="flex flex-wrap gap-xs py-xxs"
      >
        {items.map((item) => {
          const isSelected = selectedId === item.id;
          return (
            <button
              key={item.id}
              type="button"
              disabled={disabled}
              onClick={() => onSelectFaq(item)}
              aria-pressed={isSelected}
              className={cn(
                "inline-flex items-center px-sm py-xs rounded-full text-body-sm transition-colors border text-left",
                "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2",
                isSelected
                  ? "bg-primary text-primary-fg border-primary font-medium"
                  : "bg-surface-subtle text-fg-muted border-border hover:border-primary/50 hover:text-fg hover:bg-surface-raised",
                disabled && "opacity-50 cursor-not-allowed hover:border-border hover:text-fg-muted"
              )}
            >
              <span className="truncate max-w-[260px] sm:max-w-none">{item.question}</span>
            </button>
          );
        })}
      </div>
    </section>
  );
}
