import type { ReactNode } from "react";

import { Container } from "./container";
import { SkipLink } from "./skip-link";

/** Shell cho (queue) va (checkout).
 *
 * CO Y khong co nav va khong co footer. Handoff muc 6: "Queue/checkout: binh tinh, co trat tu,
 * thong tin uu tien hon trang tri"; muc 5 nguyen tac 2: "One primary action". Them menu dieu
 * huong vao hai khu vuc nay la moi nguoi dung roi luong ngay luc ho dang giu ve co han.
 *
 * Logo la <span>, KHONG phai link ve trang chu — cung ly do tren. */
export function FocusShell({ children }: { children: ReactNode }) {
  return (
    <>
      <SkipLink />
      <header className="border-b border-border bg-surface">
        <Container className="flex h-16 items-center">
          <span className="text-headline-md text-fg">EventFlow</span>
        </Container>
      </header>
      <Container as="main" id="main" tabIndex={-1} className="py-lg">
        {children}
      </Container>
    </>
  );
}
