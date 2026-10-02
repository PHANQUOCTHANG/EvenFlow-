/** Doc/ghi theme. Moi truy cap localStorage va matchMedia deu boc try/catch:
 *  o che do private hoac khi site data bi chan, ca hai co the NEM LOI chu khong
 *  chi tra ve null — trang van phai render duoc (AC-2). */

export type Theme = "light" | "dark";

export const THEME_STORAGE_KEY = "eventflow-theme";
export const THEME_ATTRIBUTE = "data-theme";

export function isTheme(value: unknown): value is Theme {
  return value === "light" || value === "dark";
}

/** Theme nguoi dung da chon thu cong, hoac null neu chua chon / khong doc duoc. */
export function readStoredTheme(): Theme | null {
  try {
    const raw = window.localStorage.getItem(THEME_STORAGE_KEY);
    return isTheme(raw) ? raw : null;
  } catch {
    return null;
  }
}

export function storeTheme(theme: Theme): void {
  try {
    window.localStorage.setItem(THEME_STORAGE_KEY, theme);
  } catch {
    /* khong ghi duoc thi bo qua: theme van ap dung cho phien hien tai */
  }
}

/** Theme theo cai dat he dieu hanh. Khong xac dinh duoc thi coi la "light". */
export function systemTheme(): Theme {
  try {
    return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
  } catch {
    return "light";
  }
}

/** Theme dang hieu luc: uu tien lua chon thu cong, sau do moi den he thong. */
export function resolveTheme(): Theme {
  return readStoredTheme() ?? systemTheme();
}

export function applyTheme(theme: Theme): void {
  document.documentElement.setAttribute(THEME_ATTRIBUTE, theme);
}
