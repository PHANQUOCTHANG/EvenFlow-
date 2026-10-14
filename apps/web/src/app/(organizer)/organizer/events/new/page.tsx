"use client";

import { useRouter } from "next/navigation";

import { EventWizard } from "@/components/organizer/event-wizard";
import { createOrganizerEvent, type CreateEventInput } from "@/lib/organizer-service";

export default function NewEventPage() {
  const router = useRouter();

  const handleSubmit = async (data: CreateEventInput) => {
    const created = await createOrganizerEvent(data);
    if (data.submitForReview) {
      router.push(`/organizer/events/${created.id}/review-status`);
    } else {
      router.push("/organizer/events");
    }
  };

  const handleCancel = () => {
    router.push("/organizer/events");
  };

  return (
    <div className="py-md">
      <EventWizard onSubmit={handleSubmit} onCancel={handleCancel} />
    </div>
  );
}
