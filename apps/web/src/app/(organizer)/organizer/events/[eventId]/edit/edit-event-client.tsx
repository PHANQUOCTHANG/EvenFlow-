"use client";

import { useRouter } from "next/navigation";
import { useRef } from "react";

import { EventWizard, type WizardSubmitResult } from "@/components/organizer/event-wizard";
import { Card } from "@/components/ui/card";
import {
  addTier,
  canEditEvent,
  canOpenEventEditor,
  getOrganizerEventById,
  submitOrganizerEvent,
  updateOrganizerEvent,
  updateTierQuota,
  type CreateEventInput,
  type OrganizerEvent,
  type TicketTierConfig,
} from "@/lib/organizer-service";

import { OrganizerEventView } from "../../../_lib/organizer-event-view";

function EditForm({
  eventId,
  event,
  setEvent,
}: {
  eventId: string;
  event: OrganizerEvent;
  setEvent: (event: OrganizerEvent) => void;
}) {
  const router = useRouter();
  // Hạng vé tạo trong phiên này: id cục bộ của wizard -> hạng vé trên máy chủ (tránh POST lại khi thử lại).
  const created = useRef(new Map<string, TicketTierConfig>());

  if (!canOpenEventEditor(event.state)) {
    return (
      <Card className="p-xl text-center space-y-sm my-lg">
        <h1 className="text-heading-md font-bold text-fg">Không thể chỉnh sửa sự kiện</h1>
        <p className="text-body-sm text-fg-muted">
          Chỉ sự kiện ở trạng thái DRAFT, SCHEDULED hoặc REJECTED mới được chỉnh sửa, và ON_SALE / SOLD_OUT chỉ
          được tăng số lượng vé (hiện tại: {event.state}).
        </p>
      </Card>
    );
  }

  const fullEdit = canEditEvent(event.state);

  /** Tải lại sự kiện từ máy chủ (nguồn sự thật cho phần so sánh hạng vé); lỗi mạng thì dùng bản đang có. */
  const reload = async (): Promise<OrganizerEvent> => {
    try {
      const fresh = await getOrganizerEventById(eventId);
      if (fresh) return fresh;
    } catch {
      // dùng bản hiện có
    }
    return event;
  };

  const createdSnapshot = () => Object.fromEntries(created.current);

  const handleSubmit = async (data: CreateEventInput): Promise<WizardSubmitResult | void> => {
    // 1. PUT đầy đủ các trường sự kiện (bỏ qua với ON_SALE: chỉ tăng quota).
    if (fullEdit) await updateOrganizerEvent(eventId, data);

    // 2. Hạng vé: so với hạng vé HIỆN TẠI trên máy chủ. Hạng mới -> POST; hạng cũ -> chỉ đổi quota.
    const current = await reload();
    const existing = new Map(current.tiers.map((t) => [t.id, t]));
    for (const tier of data.tiers) {
      try {
        const serverTier = created.current.get(tier.id);
        const key = serverTier ? serverTier.id : tier.id;
        const old = existing.get(key) ?? serverTier;
        if (!old) {
          const made = await addTier(eventId, tier);
          // H-3: khoa theo ca id cuc bo lan id may chu (sau khi wizard doi id, hang ve khong bi POST lai).
          created.current.set(tier.id, made);
          created.current.set(made.id, made);
        } else if (old.quota !== tier.quota) {
          await updateTierQuota(eventId, key, tier.quota);
        }
      } catch (err) {
        setEvent(await reload());
        return {
          error: `Hạng vé "${tier.name}": ${err instanceof Error ? err.message : String(err)}`,
          failedStep: "tiers",
          createdTiers: createdSnapshot(),
        };
      }
    }

    // 3. Nộp duyệt nếu được chọn.
    if (data.submitForReview) {
      try {
        await submitOrganizerEvent(eventId);
      } catch (err) {
        setEvent(await reload());
        return {
          error: err instanceof Error ? err.message : String(err),
          failedStep: "submit",
          createdTiers: createdSnapshot(),
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
      {(event, setEvent) => <EditForm eventId={eventId} event={event} setEvent={setEvent} />}
    </OrganizerEventView>
  );
}
