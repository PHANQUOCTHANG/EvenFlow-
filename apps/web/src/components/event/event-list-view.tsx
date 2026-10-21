import Link from "next/link";

import { EventCard } from "@/components/event/event-card";
import { Alert } from "@/components/ui";
import type { EventDetailSnapshot } from "@/lib/types/event";

export interface EventListViewProps {
  events: EventDetailSnapshot[];
  /** Co loi khi tai danh sach: hien error state (KHONG bao gio thay bang du lieu mau). */
  error?: boolean;
}

/** Danh sach the su kien cong khai: co 3 trang thai ro rang (co du lieu / rong / loi) (AC-17). */
export function EventListView({ events, error = false }: EventListViewProps) {
  if (error) {
    return (
      <Alert variant="error" title="Không tải được danh sách sự kiện" lang="vi">
        Hệ thống đang bận hoặc mất kết nối.{" "}
        <Link href="/events" className="underline">
          Tải lại trang
        </Link>{" "}
        sau ít phút.
      </Alert>
    );
  }

  if (events.length === 0) {
    return (
      <p className="text-body-lg text-fg-muted" lang="vi">
        Hiện chưa có sự kiện nào đang mở bán. Vui lòng quay lại sau.
      </p>
    );
  }

  return (
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
  );
}
