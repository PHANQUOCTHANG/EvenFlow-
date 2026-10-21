import { ModeratorOnly } from "@/components/moderator/moderator-only";
import { ReviewDetail } from "@/components/moderator/review-detail";

export const metadata = {
  title: "Thẩm định hồ sơ — Kiểm duyệt EvenFlow",
  description: "Nội dung gốc của ban tổ chức, phân tích AI và quyết định của Moderator",
};

export default async function ReviewDetailPage({ params }: { params: Promise<{ eventId: string }> }) {
  const { eventId } = await params;
  return (
    <ModeratorOnly>
      <ReviewDetail eventId={eventId} />
    </ModeratorOnly>
  );
}
