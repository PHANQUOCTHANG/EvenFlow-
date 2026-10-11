import { EventList } from "@/components/organizer/event-list";
import { getOrganizerEvents } from "@/lib/organizer-service";

export const metadata = {
  title: "Sự kiện của tôi — Ban tổ chức EvenFlow",
  description: "Quản lý sự kiện, theo dõi duyệt và doanh số realtime",
};

export default async function OrganizerEventsPage() {
  const events = await getOrganizerEvents();

  return (
    <div className="py-md">
      <EventList events={events} />
    </div>
  );
}
