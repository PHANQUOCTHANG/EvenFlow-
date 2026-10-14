"use client";

import type { MessageKey } from "@/lib/i18n/messages";

import { useT } from "./locale-provider";

/** Hien chuoi da dich o dang chu tran. Cho phep server component (SkipLink, footer) dung i18n
 *  ma khong phai thanh client component. */
export function T({ k, params }: { k: MessageKey; params?: Record<string, string | number> }) {
  const t = useT();
  return <>{t(k, params)}</>;
}
