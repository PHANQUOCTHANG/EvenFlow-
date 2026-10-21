"use client";

import { CheckoutView } from "@/components/checkout/checkout-view";
import { EventLoader } from "@/components/event/event-loader";

/** Trang chon ve: tai su kien phia client roi hien CheckoutView (AC-18). */
export function CheckoutPageClient({ eventId }: { eventId: string }) {
  return <EventLoader eventId={eventId}>{(event) => <CheckoutView event={event} />}</EventLoader>;
}
