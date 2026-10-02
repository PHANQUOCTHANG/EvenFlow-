import { THEME_ATTRIBUTE, THEME_STORAGE_KEY } from "@/lib/theme";

/** Script chan paint: phai chay TRUOC hydration, neu khong trang se nhay mau (FOUC).
 *
 * Vi vay no la script inline dat trong <head>, khong phai useEffect. Moi truy cap
 * localStorage/matchMedia boc try/catch rieng: o che do private chung co the nem loi
 * va trang van phai render (AC-2). */
const SCRIPT = `(function(){try{var s=null;try{s=window.localStorage.getItem(${JSON.stringify(
  THEME_STORAGE_KEY,
)})}catch(e){}var t=(s==="light"||s==="dark")?s:((window.matchMedia&&window.matchMedia("(prefers-color-scheme: dark)").matches)?"dark":"light");document.documentElement.setAttribute(${JSON.stringify(
  THEME_ATTRIBUTE,
)},t)}catch(e){}})();`;

export function ThemeScript() {
  return <script suppressHydrationWarning dangerouslySetInnerHTML={{ __html: SCRIPT }} />;
}
