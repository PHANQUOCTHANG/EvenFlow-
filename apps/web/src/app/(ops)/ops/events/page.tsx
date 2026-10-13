import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Card } from "@/components/ui/card";
import { listOpsEvents, saleStateLabel } from "@/lib/ops-service";

export const metadata = {
  title: "War room — Vận hành EvenFlow",
  description: "Chọn sự kiện để theo dõi phễu, hàng chờ, tồn kho và AI",
};

// So lieu song: khong prerender luc build.
export const dynamic = "force-dynamic";

export default async function OpsEventsPage() {
  const events = await listOpsEvents();

  return (
    <div className="mx-auto max-w-4xl space-y-lg py-md">
      <header className="space-y-xs">
        <h1 className="text-headline-xl font-bold text-fg">War room</h1>
        <p className="text-body-md text-fg-muted">Chọn một sự kiện để mở màn hình vận hành.</p>
      </header>

      <ul className="space-y-sm">
        {events.map((event) => (
          <li key={event.eventId}>
            <Card className="p-0">
              <Link
                href={`/ops/events/${event.eventId}`}
                className="ef-focus-ring flex items-center justify-between gap-md rounded-card p-md hover:bg-surface-subtle"
              >
                <span className="font-semibold text-fg">{event.eventTitle}</span>
                <Badge variant={event.saleState === "ON_SALE" ? "admitted" : "neutral"}>
                  {saleStateLabel(event.saleState)}
                </Badge>
              </Link>
            </Card>
          </li>
        ))}
      </ul>
    </div>
  );
}
