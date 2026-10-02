"use client";

import { useCallback, useEffect, useState } from "react";

import { THEME_ATTRIBUTE, applyTheme, isTheme, resolveTheme, storeTheme, type Theme } from "@/lib/theme";

/** Theme dang hieu luc + cach doi.
 *
 * Khi mount, uu tien doc `data-theme` da duoc ThemeScript dat san tren <html>: do la
 * gia tri DANG hien thi. Chi khi khong co attribute moi tinh lai tu storage/he thong. */
export function useTheme(): {
  theme: Theme;
  setTheme: (next: Theme) => void;
  toggle: () => void;
} {
  const [theme, setThemeState] = useState<Theme>("light");

  useEffect(() => {
    const current = document.documentElement.getAttribute(THEME_ATTRIBUTE);
    setThemeState(isTheme(current) ? current : resolveTheme());
  }, []);

  const setTheme = useCallback((next: Theme) => {
    applyTheme(next);
    storeTheme(next);
    setThemeState(next);
  }, []);

  const toggle = useCallback(() => {
    setTheme(theme === "dark" ? "light" : "dark");
  }, [theme, setTheme]);

  return { theme, setTheme, toggle };
}
