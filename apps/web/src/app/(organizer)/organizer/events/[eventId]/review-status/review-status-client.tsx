"use client";

import { useState } from "react";

import { ModerationStatusCard } from "@/components/organizer/moderation-status-card";
import {
  resubmitForModeration,
  type OrganizerEvent,
} from "@/lib/organizer-service";

export function ReviewStatusClient({ initialEvent }: { initialEvent: OrganizerEvent }) {
  const [event, setEvent] = useState<OrganizerEvent>(initialEvent);

  const handleResubmit = async (id: string) => {
    const updated = await resubmitForModeration(id);
    setEvent(updated);
  };

  return <ModerationStatusCard event={event} onResubmit={handleResubmit} />;
}
