"use client";

import { useRouter } from "next/navigation";

import { EventWizard } from "@/components/organizer/event-wizard";
import {
  updateOrganizerEvent,
  type CreateEventInput,
  type OrganizerEvent,
} from "@/lib/organizer-service";

export function EditEventClient({
  eventId,
  initialEvent,
}: {
  eventId: string;
  initialEvent: OrganizerEvent;
}) {
  const router = useRouter();

  const handleSubmit = async (data: CreateEventInput) => {
    await updateOrganizerEvent(eventId, data);
    router.push(`/organizer/events/${eventId}/review-status`);
  };

  const handleCancel = () => {
    router.push("/organizer/events");
  };

  return (
    <EventWizard
      initialEvent={initialEvent}
      onSubmit={handleSubmit}
      onCancel={handleCancel}
    />
  );
}
