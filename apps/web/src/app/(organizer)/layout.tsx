import type { ReactNode } from "react";

import { ORGANIZER_NAV, WorkspaceShell } from "@/components/layout";

export default function OrganizerLayout({ children }: { children: ReactNode }) {
  return (
    <WorkspaceShell title="Nhà tổ chức" items={ORGANIZER_NAV}>
      {children}
    </WorkspaceShell>
  );
}
