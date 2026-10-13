"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";

import {
  DEFAULT_LOCALE,
  LOCALE_STORAGE_KEY,
  isLocale,
  translate,
  type Locale,
  type MessageKey,
} from "@/lib/i18n/messages";

type TFunction = (key: MessageKey, params?: Record<string, string | number>) => string;

interface LocaleContextValue {
  locale: Locale;
  setLocale: (next: Locale) => void;
}

/** Ngoai provider: dich theo vi, setLocale la no-op (test component cu khong can boc provider). */
const LocaleContext = createContext<LocaleContextValue>({
  locale: DEFAULT_LOCALE,
  setLocale: () => {},
});

/** Render dau (server VA client) luon la vi de HTML khop, khong loi hydration va khong ep trang ISR
 *  thanh dynamic. Locale da luu chi duoc ap dung SAU mount. Khong dung router: doi ngon ngu khong
 *  dieu huong, khong reload. */
export function LocaleProvider({ children }: { children: React.ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(DEFAULT_LOCALE);

  useEffect(() => {
    let stored: string | null = null;
    try {
      stored = window.localStorage.getItem(LOCALE_STORAGE_KEY);
    } catch {
      // Storage bi chan (private mode): giu mac dinh.
    }
    if (isLocale(stored)) {
      setLocaleState(stored);
      // Khoi phuc thi KHONG ghi lai storage.
      document.documentElement.lang = stored;
    }
  }, []);

  const setLocale = useCallback((next: Locale) => {
    setLocaleState(next);
    document.documentElement.lang = next;
    try {
      window.localStorage.setItem(LOCALE_STORAGE_KEY, next);
    } catch {
      // Khong ghi duoc thi chi mat tinh nho lua chon, van doi chu binh thuong.
    }
  }, []);

  const value = useMemo(() => ({ locale, setLocale }), [locale, setLocale]);
  return <LocaleContext.Provider value={value}>{children}</LocaleContext.Provider>;
}

export function useLocale(): LocaleContextValue {
  return useContext(LocaleContext);
}

/** Ham dich memo theo locale. */
export function useT(): TFunction {
  const { locale } = useLocale();
  return useMemo<TFunction>(() => (key, params) => translate(locale, key, params), [locale]);
}
