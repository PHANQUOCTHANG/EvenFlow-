import type { ReactNode } from "react";

import { MODERATOR_NAV, WorkspaceShell } from "@/components/layout";

export default function ModeratorLayout({ children }: { children: ReactNode }) {
  return (
    <WorkspaceShell title="Kiểm duyệt" items={MODERATOR_NAV}>
      {children}
    </WorkspaceShell>
  );
}
