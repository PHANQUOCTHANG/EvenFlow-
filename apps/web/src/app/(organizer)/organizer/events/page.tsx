import { OrganizerEventsClient } from "./events-client";

export const metadata = {
  title: "Sự kiện của tôi — Ban tổ chức EvenFlow",
  description: "Quản lý sự kiện, theo dõi duyệt và doanh số realtime",
};

export default function OrganizerEventsPage() {
  return <OrganizerEventsClient />;
}
