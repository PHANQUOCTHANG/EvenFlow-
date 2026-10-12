import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { PaymentHandoffView } from "@/components/checkout/payment-handoff-view";
import { getEventById } from "@/lib/event-service";

interface PageProps {
  params: Promise<{ eventId: string }>;
  searchParams: Promise<{
    orderId?: string;
    holdId?: string;
    tierId?: string;
    quantity?: string;
  }>;
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { eventId } = await params;
  const event = await getEventById(eventId);

  const title = event?.title ? `Thanh toán: ${event.title}` : "Thanh toán đơn hàng";

  return {
    title: `${title} | EvenFlow`,
    description: "Thanh toán an toàn cho đơn hàng vé EvenFlow với đồng hồ giữ ghế 10:00 của server.",
    robots: {
      index: false,
      follow: false,
    },
  };
}

export default async function PaymentPage({ params, searchParams }: PageProps) {
  const { eventId } = await params;
  const query = await searchParams;

  const event = await getEventById(eventId);
  if (!event) {
    notFound();
  }

  const orderId = query.orderId || `ord_${eventId}`;
  const holdId = query.holdId || `hld_${eventId}`;
  const quantity = query.quantity ? parseInt(query.quantity, 10) : 1;
  const selectedTier = event.tiers.find((t) => t.id === query.tierId) || event.tiers[0];

  return (
    <PaymentHandoffView
      eventId={event.id}
      eventTitle={event.title}
      venue={event.venue}
      orderId={orderId}
      holdId={holdId}
      tierName={selectedTier?.name || "Vé Tiêu Chuẩn"}
      quantity={quantity > 0 ? quantity : 1}
      unitPrice={selectedTier?.price || 0}
      expiresAt={undefined} // Sẽ được đồng bộ từ sessionStorage / client hold
    />
  );
}
