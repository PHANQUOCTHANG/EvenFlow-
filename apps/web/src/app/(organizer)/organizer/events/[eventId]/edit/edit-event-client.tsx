"use client";

import { useRouter } from "next/navigation";

import { EventWizard, type WizardSubmitResult } from "@/components/organizer/event-wizard";
import { Card } from "@/components/ui/card";
import {
  addTier,
  canEditEvent,
  submitOrganizerEvent,
  updateOrganizerEvent,
  updateTierQuota,
  type CreateEventInput,
  type OrganizerEvent,
} from "@/lib/organizer-service";

import { OrganizerEventView } from "../../../_lib/organizer-event-view";

function EditForm({ eventId, event }: { eventId: string; event: OrganizerEvent }) {
  const router = useRouter();

  if (!canEditEvent(event.state)) {
    return (
      <Card className="p-xl text-center space-y-sm my-lg">
        <h1 className="text-heading-md font-bold text-fg">Không thể chỉnh sửa sự kiện</h1>
        <p className="text-body-sm text-fg-muted">
          Chỉ sự kiện ở trạng thái DRAFT, SCHEDULED hoặc REJECTED mới được chỉnh sửa (hiện tại:{" "}
          {event.state}).
        </p>
      </Card>
    );
  }

  const handleSubmit = async (data: CreateEventInput): Promise<WizardSubmitResult | void> => {
    // 1. PUT đầy đủ các trường sự kiện. Lỗi ở đây được wizard hiển thị nguyên văn.
    await updateOrganizerEvent(eventId, data);

    // 2. Hạng vé: hạng mới -> POST; hạng cũ -> chỉ đổi quota (tên/giá là chỉ đọc).
    const existing = new Map(event.tiers.map((t) => [t.id, t]));
    const draftHref = `/organizer/events/${eventId}/edit`;
    for (const tier of data.tiers) {
      try {
        const old = existing.get(tier.id);
        if (!old) await addTier(eventId, tier);
        else if (old.quota !== tier.quota) await updateTierQuota(eventId, tier.id, tier.quota);
      } catch (err) {
        return {
          error: `Hạng vé "${tier.name}": ${err instanceof Error ? err.message : String(err)}`,
          failedStep: "tiers",
          draftHref,
        };
      }
    }

    // 3. Nộp duyệt nếu được chọn.
    if (data.submitForReview) {
      try {
        await submitOrganizerEvent(eventId);
      } catch (err) {
        return {
          error: err instanceof Error ? err.message : String(err),
          failedStep: "submit",
          draftHref,
        };
      }
    }
    router.push(`/organizer/events/${eventId}/review-status`);
  };

  const handleCancel = () => {
    router.push("/organizer/events");
  };

  return <EventWizard initialEvent={event} onSubmit={handleSubmit} onCancel={handleCancel} />;
}

export function EditEventClient({ eventId }: { eventId: string }) {
  return (
    <OrganizerEventView eventId={eventId}>
      {(event) => <EditForm eventId={eventId} event={event} />}
    </OrganizerEventView>
  );
}
