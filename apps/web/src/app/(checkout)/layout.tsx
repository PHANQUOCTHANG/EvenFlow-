import type { ReactNode } from "react";

import { FocusShell } from "@/components/layout";

/** (checkout): chon ve, giu cho, thanh toan. Khong co nav — nguoi dung dang giu ve co han,
 *  khong moi ho roi luong (handoff muc 5 nguyen tac 2). */
export default function CheckoutLayout({ children }: { children: ReactNode }) {
  return <FocusShell>{children}</FocusShell>;
}
