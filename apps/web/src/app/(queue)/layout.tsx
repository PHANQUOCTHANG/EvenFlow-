import type { ReactNode } from "react";

import { FocusShell } from "@/components/layout";

/** (queue): phong cho ao. Khong co nav — xem comment trong focus-shell.tsx. */
export default function QueueLayout({ children }: { children: ReactNode }) {
  return <FocusShell>{children}</FocusShell>;
}
