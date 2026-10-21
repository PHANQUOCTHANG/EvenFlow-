import type { Metadata } from "next";

import { EventListView } from "@/components/event/event-list-view";
import { EventsBrowserList } from "@/components/event/events-browser-list";
import { getAllPublicEvents } from "@/lib/event-service";
import type { EventDetailSnapshot } from "@/lib/types/event";

export const revalidate = 60;

export const metadata: Metadata = {
  title: "Sự kiện nổi bật | EvenFlow",
  description:
    "Danh sách các sự kiện âm nhạc, công nghệ và thể thao đỉnh cao trên nền tảng bán vé EvenFlow.",
};

export default async function EventsListPage() {
  // Luc `next build` khong goi backend: prerender vo trang, danh sach duoc tai o trinh duyet.
  // Luc chay, ISR (revalidate = 60) goi backend; loi -> error state, khong bao gio du lieu mau.
  const isBuild = process.env.NEXT_PHASE === "phase-production-build";
  let events: EventDetailSnapshot[] = [];
  let failed = false;
  if (!isBuild) {
    try {
      events = await getAllPublicEvents({ revalidate: 60 });
    } catch {
      failed = true;
    }
  }

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

      {isBuild ? <EventsBrowserList /> : <EventListView events={events} error={failed} />}
    </div>
  );
}
