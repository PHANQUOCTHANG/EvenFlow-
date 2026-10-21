import type { Metadata } from "next";

import { WaitingRoomPageClient } from "@/components/queue/waiting-room-page-client";

interface PageProps {
  params: Promise<{ eventId: string }>;
}

// Tieu de tinh: su kien duoc tai phia client (AC-18), server khong goi backend.
export const metadata: Metadata = {
  title: "Phòng chờ mua vé | EvenFlow",
  description:
    "Phòng chờ trực tuyến công bằng với hệ thống xáo trộn ngẫu nhiên và bảo vệ chống quá tải EvenFlow.",
  robots: {
    index: false,
    follow: false,
  },
};

export default async function WaitingRoomPage({ params }: PageProps) {
  const { eventId } = await params;
  return <WaitingRoomPageClient eventId={eventId} />;
}
