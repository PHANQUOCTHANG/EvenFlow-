import type { ReactNode } from "react";

import { PublicShell } from "@/components/layout";

/** (marketing): trang tinh / ISR, phuc vu tu CDN (docs/03-cau-truc-src.md muc 6). */
export default function MarketingLayout({ children }: { children: ReactNode }) {
  return <PublicShell>{children}</PublicShell>;
}
