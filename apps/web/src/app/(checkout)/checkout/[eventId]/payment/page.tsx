import type { Metadata } from "next";

import { PaymentPageClient } from "@/components/checkout/payment-page-client";

interface PageProps {
  params: Promise<{ eventId: string }>;
  searchParams: Promise<{
    orderId?: string;
    holdId?: string;
    tierId?: string;
    quantity?: string;
  }>;
}

// Tieu de tinh: su kien duoc tai phia client (AC-18), server khong goi backend.
export const metadata: Metadata = {
  title: "Thanh toán đơn hàng | EvenFlow",
  description: "Thanh toán an toàn cho đơn hàng vé EvenFlow với đồng hồ giữ ghế 10:00 của server.",
  robots: {
    index: false,
    follow: false,
  },
};

export default async function PaymentPage({ params, searchParams }: PageProps) {
  const { eventId } = await params;
  const query = await searchParams;

  return (
    <PaymentPageClient
      eventId={eventId}
      orderId={query.orderId}
      holdId={query.holdId}
      tierId={query.tierId}
      quantity={query.quantity}
    />
  );
}
