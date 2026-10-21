"use client";

import { ModerationStatusCard } from "@/components/organizer/moderation-status-card";
import { cancelOrganizerEvent, submitOrganizerEvent } from "@/lib/organizer-service";

import { OrganizerEventView } from "../../../_lib/organizer-event-view";

export function ReviewStatusClient({ eventId }: { eventId: string }) {
  return (
    <OrganizerEventView eventId={eventId}>
      {(event, setEvent) => (
        <ModerationStatusCard
          event={event}
          onResubmit={async (id) => {
            // Giữ hạng vé đã tải: phản hồi submit không kèm tiers.
            setEvent({ ...(await submitOrganizerEvent(id)), tiers: event.tiers });
          }}
          onCancel={async (id) => {
            setEvent({ ...(await cancelOrganizerEvent(id)), tiers: event.tiers });
          }}
        />
      )}
    </OrganizerEventView>
  );
}
