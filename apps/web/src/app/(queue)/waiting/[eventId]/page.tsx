import type { Metadata } from "next";

import { WaitingRoom } from "@/components/queue/waiting-room";
import { getEventById } from "@/lib/event-service";

interface PageProps {
  params: Promise<{ eventId: string }>;
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { eventId } = await params;
  const event = await getEventById(eventId);

  const title = event?.title ? `Phòng chờ: ${event.title}` : "Phòng chờ mua vé";

  return {
    title: `${title} | EvenFlow`,
    description:
      "Phòng chờ trực tuyến công bằng với hệ thống xáo trộn ngẫu nhiên và bảo vệ chống quá tải EvenFlow.",
    robots: {
      index: false,
      follow: false,
    },
  };
}

export default async function WaitingRoomPage({ params }: PageProps) {
  const { eventId } = await params;
  const event = await getEventById(eventId);

  return (
    <div className="w-full flex flex-col justify-center">
      <WaitingRoom eventId={eventId} eventTitle={event?.title} />
    </div>
  );
}
