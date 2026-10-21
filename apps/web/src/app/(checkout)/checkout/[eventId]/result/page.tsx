import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { OrderResultView } from "@/components/checkout/order-result-view";
import { getEventById } from "@/lib/event-service";

interface PageProps {
  params: Promise<{ eventId: string }>;
  searchParams: Promise<{
    orderId?: string;
  }>;
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { eventId } = await params;
  // Backend loi: dung tieu de chung (WP2 chuyen cac trang nay sang tra cuu phia client).
  const event = await getEventById(eventId).catch(() => null);

  const title = event?.title ? `Kết quả đơn hàng: ${event.title}` : "Kết quả đơn hàng";

  return {
    title: `${title} | EvenFlow`,
    description: "Trạng thái và chi tiết vé điện tử của đơn hàng trên EvenFlow.",
    robots: {
      index: false,
      follow: false,
    },
  };
}

export default async function OrderResultPage({ params, searchParams }: PageProps) {
  const { eventId } = await params;
  const query = await searchParams;

  const event = await getEventById(eventId);
  if (!event) {
    notFound();
  }

  const orderId = query.orderId || `ord_${eventId}`;

  return <OrderResultView eventId={event.id} orderId={orderId} />;
}
