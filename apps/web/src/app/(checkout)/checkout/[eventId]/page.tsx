import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { CheckoutView } from "@/components/checkout/checkout-view";
import { getEventById } from "@/lib/event-service";

interface PageProps {
  params: Promise<{ eventId: string }>;
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { eventId } = await params;
  const event = await getEventById(eventId);

  const title = event?.title ? `Chọn vé: ${event.title}` : "Chọn vé";

  return {
    title: `${title} | EvenFlow`,
    description:
      "Chọn hạng vé và số lượng với đồng hồ giữ ghế 10:00 theo mốc của server EvenFlow.",
    robots: {
      index: false,
      follow: false,
    },
  };
}

export default async function CheckoutPage({ params }: PageProps) {
  const { eventId } = await params;
  const event = await getEventById(eventId);

  if (!event) {
    notFound();
  }

  return <CheckoutView event={event} />;
}
