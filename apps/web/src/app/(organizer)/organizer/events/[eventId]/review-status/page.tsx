import { ReviewStatusClient } from "./review-status-client";

export const metadata = {
  title: "Trạng thái kiểm duyệt — Ban tổ chức EvenFlow",
  description: "Trạng thái duyệt thực của sự kiện; chi tiết đánh giá AI hiện là dữ liệu mẫu",
};

export default async function ReviewStatusPage({
  params,
}: {
  params: Promise<{ eventId: string }>;
}) {
  const { eventId } = await params;
  return <ReviewStatusClient eventId={eventId} />;
}
