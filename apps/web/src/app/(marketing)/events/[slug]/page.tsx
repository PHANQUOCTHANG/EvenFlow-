import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { EventActionPanel } from "@/components/event/event-action-panel";
import { LobbyNotice } from "@/components/event/lobby-notice";
import { TicketTierList } from "@/components/event/ticket-tier-list";
import { Badge, type BadgeVariant } from "@/components/ui";
import {
  getAllEventSlugs,
  getEventBySlug,
} from "@/lib/event-service";
import { toEpochMs } from "@/lib/server-time";

export const revalidate = 60;

interface PageProps {
  params: Promise<{ slug: string }>;
}

export async function generateStaticParams() {
  const slugs = await getAllEventSlugs();
  return slugs.map((slug) => ({ slug }));
}

export async function generateMetadata({
  params,
}: PageProps): Promise<Metadata> {
  const { slug } = await params;
  const event = await getEventBySlug(slug);

  if (!event) {
    return {
      title: "Không tìm thấy sự kiện | EvenFlow",
    };
  }

  return {
    title: `${event.title} | EvenFlow`,
    description: event.description,
    openGraph: {
      title: event.title,
      description: event.description,
      type: "website",
    },
  };
}

const STATE_BADGE: Record<string, { label: string; variant: BadgeVariant }> = {
  SCHEDULED: { label: "Chưa mở bán", variant: "pending" },
  ON_SALE: { label: "Đang mở bán", variant: "admitted" },
  SOLD_OUT: { label: "Hết vé", variant: "soldout" },
  CLOSED: { label: "Đã đóng bán", variant: "neutral" },
  COMPLETED: { label: "Đã kết thúc", variant: "neutral" },
  CANCELLED: { label: "Đã huỷ", variant: "failed" },
};

function formatVenueTime(value: string): string {
  const ms = toEpochMs(value);
  if (ms === null) return value;
  const shifted = new Date(ms + 7 * 60 * 60_000);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(shifted.getUTCDate())}/${pad(shifted.getUTCMonth() + 1)}/${shifted.getUTCFullYear()} ${pad(shifted.getUTCHours())}:${pad(shifted.getUTCMinutes())}`;
}

export default async function EventDetailPage({ params }: PageProps) {
  const { slug } = await params;
  const event = await getEventBySlug(slug);

  if (!event) {
    notFound();
  }

  const badgeInfo = STATE_BADGE[event.state] ?? {
    label: event.state,
    variant: "neutral" as BadgeVariant,
  };

  return (
    <article className="py-xl">
      {/* Event Header Banner */}
      <header className="mb-xl">
        <div className="flex flex-wrap items-center gap-sm mb-sm">
          <Badge variant={badgeInfo.variant}>{badgeInfo.label}</Badge>
          <span className="text-body-sm text-fg-muted">
            Địa điểm: {event.venue}
          </span>
        </div>

        <h1 className="text-heading-xl font-bold text-fg mb-md">
          {event.title}
        </h1>

        <div className="flex flex-wrap gap-lg text-body-md text-fg-muted mb-lg">
          <div>
            <span className="font-medium text-fg">Thời gian diễn ra: </span>
            <time dateTime={event.startsAt}>{formatVenueTime(event.startsAt)}</time>
          </div>
          <div>
            <span className="font-medium text-fg">Mở bán lúc: </span>
            <time dateTime={event.saleStartAt}>{formatVenueTime(event.saleStartAt)}</time>
          </div>
        </div>

        <p className="text-body-lg text-fg max-w-3xl leading-relaxed">
          {event.description}
        </p>
      </header>

      {/* Fair Queue Lobby notice (BR-Q1) */}
      <div className="mb-xl">
        <LobbyNotice />
      </div>

      {/* Ticket Action Panel with Countdown & Jitter (BR-O2, BR-Q5) */}
      <div className="mb-2xl">
        <EventActionPanel
          eventId={event.id}
          state={event.state}
          saleStartAt={event.saleStartAt}
        />
      </div>

      {/* Ticket Tiers (BR-A4) */}
      <section aria-labelledby="ticket-tiers-heading">
        <TicketTierList tiers={event.tiers} />
      </section>
    </article>
  );
}
