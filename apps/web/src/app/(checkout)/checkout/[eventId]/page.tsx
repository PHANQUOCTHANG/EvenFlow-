import type { Metadata } from "next";

import { CheckoutPageClient } from "@/components/checkout/checkout-page-client";

interface PageProps {
  params: Promise<{ eventId: string }>;
}

// Tieu de tinh: su kien duoc tai phia client (AC-18), server khong goi backend.
export const metadata: Metadata = {
  title: "Chọn vé | EvenFlow",
  description:
    "Chọn hạng vé và số lượng với đồng hồ giữ ghế 10:00 theo mốc của server EvenFlow.",
  robots: {
    index: false,
    follow: false,
  },
};

export default async function CheckoutPage({ params }: PageProps) {
  const { eventId } = await params;
  return <CheckoutPageClient eventId={eventId} />;
}
