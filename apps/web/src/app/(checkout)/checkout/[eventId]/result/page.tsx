import type { Metadata } from "next";

import { ResultPageClient } from "@/components/checkout/result-page-client";

interface PageProps {
  params: Promise<{ eventId: string }>;
  searchParams: Promise<{
    orderId?: string;
  }>;
}

// Tieu de tinh: su kien duoc tai phia client (AC-18), server khong goi backend.
export const metadata: Metadata = {
  title: "Kết quả đơn hàng | EvenFlow",
  description: "Trạng thái và chi tiết vé điện tử của đơn hàng trên EvenFlow.",
  robots: {
    index: false,
    follow: false,
  },
};

export default async function OrderResultPage({ params, searchParams }: PageProps) {
  const { eventId } = await params;
  const query = await searchParams;

  return <ResultPageClient eventId={eventId} orderId={query.orderId} />;
}
