"use client";

import { EventLoader } from "@/components/event/event-loader";
import { WaitingRoom } from "@/components/queue/waiting-room";

/** Trang phong cho: tai su kien phia client (AC-18) roi vao phong cho. */
export function WaitingRoomPageClient({ eventId }: { eventId: string }) {
  return (
    <EventLoader eventId={eventId}>
      {(event) => (
        <div className="w-full flex flex-col justify-center">
          <WaitingRoom eventId={event.id} eventTitle={event.title} />
        </div>
      )}
    </EventLoader>
  );
}
