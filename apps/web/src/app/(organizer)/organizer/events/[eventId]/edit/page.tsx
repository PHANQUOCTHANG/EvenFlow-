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
  return <EditEventClient eventId={eventId} />;
}
