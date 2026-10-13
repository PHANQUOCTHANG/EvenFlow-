import { Card } from "@/components/ui/card";
import { getOrganizerEventById } from "@/lib/organizer-service";

import { ReviewStatusClient } from "./review-status-client";

export const metadata = {
  title: "Trạng thái kiểm duyệt — Ban tổ chức EvenFlow",
  description: "Chi tiết kiểm duyệt và đánh giá AI 4 trục",
};

export default async function ReviewStatusPage({
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
      <ReviewStatusClient initialEvent={event} />
    </div>
  );
}
