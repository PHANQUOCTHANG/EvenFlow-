import type { ReactNode } from "react";

import { OPS_NAV, WorkspaceShell } from "@/components/layout";

export default function OpsLayout({ children }: { children: ReactNode }) {
  return (
    <WorkspaceShell title="Vận hành" items={OPS_NAV}>
      {children}
    </WorkspaceShell>
  );
}
