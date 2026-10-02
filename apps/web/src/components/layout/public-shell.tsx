import type { ReactNode } from "react";

import { Container } from "./container";
import { PublicFooter } from "./public-footer";
import { PublicHeader } from "./public-header";
import { SkipLink } from "./skip-link";

/** Shell cho route group (marketing): trang cong khai, co header + footer day du. */
export function PublicShell({ children }: { children: ReactNode }) {
  return (
    <>
      <SkipLink />
      <PublicHeader />
      <Container as="main" id="main" tabIndex={-1} className="py-xl">
        {children}
      </Container>
      <PublicFooter />
    </>
  );
}
