import type { Metadata } from "next";

import { EventCard } from "@/components/event/event-card";
import { getAllPublicEvents } from "@/lib/event-service";

export const revalidate = 60;

export const metadata: Metadata = {
  title: "Sự kiện nổi bật | EvenFlow",
  description:
    "Danh sách các sự kiện âm nhạc, công nghệ và thể thao đỉnh cao trên nền tảng bán vé EvenFlow.",
};

export default async function EventsListPage() {
  const events = await getAllPublicEvents();

  return (
    <div className="py-xl">
      <header className="mb-xl">
        <h1 className="text-heading-xl font-bold text-fg mb-sm">
          Sự kiện nổi bật
        </h1>
        <p className="text-body-lg text-fg-muted">
          Khám phá và đặt vé các sự kiện quy mô lớn với trải nghiệm xếp hàng công bằng, không quá tải.
        </p>
      </header>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-lg">
        {events.map((event) => (
          <EventCard
            key={event.id}
            title={event.title}
            venue={event.venue}
            startsAt={event.startsAt}
            state={event.state}
            href={`/events/${event.slug}`}
          />
        ))}
      </div>
    </div>
  );
}
