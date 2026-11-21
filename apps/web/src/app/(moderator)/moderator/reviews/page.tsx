import { ModeratorOnly } from "@/components/moderator/moderator-only";
import { ReviewQueue } from "@/components/moderator/review-queue";

export const metadata = {
  title: "Hàng đợi thẩm định — Kiểm duyệt EvenFlow",
  description: "Sự kiện cần Moderator xem xét: AI chuyển người duyệt hoặc AI đã từ chối",
};

export default function ReviewQueuePage() {
  return (
    <ModeratorOnly>
      <ReviewQueue />
    </ModeratorOnly>
  );
}
