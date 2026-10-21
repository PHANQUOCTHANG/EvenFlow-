"use client";

import { useRouter } from "next/navigation";

import { EventWizard, type WizardSubmitResult } from "@/components/organizer/event-wizard";
import { createOrganizerEvent, type CreateEventInput } from "@/lib/organizer-service";

import { OrganizerOnly } from "../../_lib/organizer-only";

export default function NewEventPage() {
  const router = useRouter();

  const handleSubmit = async (data: CreateEventInput): Promise<WizardSubmitResult | void> => {
    const result = await createOrganizerEvent(data);
    if (result.failedStep) {
      // Bản nháp đã được tạo: không bỏ mất, dẫn người dùng tới trang chỉnh sửa.
      return {
        error: result.error,
        failedStep: result.failedStep,
        draftHref: `/organizer/events/${result.event.id}/edit`,
        // Bản nháp đã có: không POST /events lần nữa (AC-21).
        blockResubmit: true,
      };
    }
    if (data.submitForReview) {
      router.push(`/organizer/events/${result.event.id}/review-status`);
    } else {
      router.push("/organizer/events");
    }
  };

  const handleCancel = () => {
    router.push("/organizer/events");
  };

  return (
    <OrganizerOnly>
      <div className="py-md">
        <EventWizard onSubmit={handleSubmit} onCancel={handleCancel} />
      </div>
    </OrganizerOnly>
  );
}
