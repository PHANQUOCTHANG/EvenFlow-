import { Card } from "@/components/ui/card";
import { getOrganizerEventById } from "@/lib/organizer-service";

import { EditEventClient } from "./edit-event-client";

export const metadata = {
  title: "Chỉnh sửa sự kiện — Ban tổ chức EvenFlow",
  description: "Cập nhật thông tin sự kiện, hạng vé và thời gian mở bán",
};

export default async function EditEventPage({
  params,
}: {
  params: Promise<{ eventId: string }>;
}) {
  const { eventId } = await params;
  const event = await getOrganizerEventById(eventId);

  if (!event) {
    return (
      <Card className="p-xl text-center space-y-sm my-lg">
        <h1 className="text-heading-md font-bold text-fg">Không tìm thấy sự kiện</h1>
        <p className="text-body-sm text-fg-muted">
          Sự kiện với mã &ldquo;{eventId}&rdquo; không tồn tại hoặc bạn không có quyền truy cập.
        </p>
      </Card>
    );
  }

  return (
    <div className="py-md">
      <EditEventClient eventId={eventId} initialEvent={event} />
    </div>
  );
}
