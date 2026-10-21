"use client";

import { SalesDashboard } from "@/components/organizer/sales-dashboard";

import { OrganizerEventView } from "../../../_lib/organizer-event-view";

export function SalesClient({ eventId }: { eventId: string }) {
  return (
    <OrganizerEventView eventId={eventId}>
      {(event) => <SalesDashboard event={event} />}
    </OrganizerEventView>
  );
}
