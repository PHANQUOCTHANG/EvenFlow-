import type { ReactNode } from "react";

/** Noi dung /auth/** chua dich (tieng Viet). `display: contents` nen khong doi bo cuc,
 *  nhung `lang="vi"` van ap dung de screen reader doc dung khi <html lang> la en (WCAG 3.1.2). */
export default function AuthLayout({ children }: { children: ReactNode }) {
  return (
    <div lang="vi" className="contents">
      {children}
    </div>
  );
}
