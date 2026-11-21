import type { ReactNode } from "react";

import { OPS_NAV, WorkspaceShell } from "@/components/layout";

import { OpsOnly } from "./_lib/ops-only";

export default function OpsLayout({ children }: { children: ReactNode }) {
  return (
    <WorkspaceShell title="Vận hành" items={OPS_NAV}>
      <OpsOnly>{children}</OpsOnly>
    </WorkspaceShell>
  );
}
