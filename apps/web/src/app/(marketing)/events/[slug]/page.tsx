import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";

import { EventActionPanel } from "@/components/event/event-action-panel";
import { LobbyNotice } from "@/components/event/lobby-notice";
import { TICKET_TIERS_HEADING_ID, TicketTierList } from "@/components/event/ticket-tier-list";
import { Alert, Badge, type BadgeVariant } from "@/components/ui";
import { getEventBySlug } from "@/lib/event-service";
import { toEpochMs } from "@/lib/server-time";

export const revalidate = 60;

interface PageProps {
  params: Promise<{ slug: string }>;
}

/** Khong doc backend luc build: trang sinh theo yeu cau dau tien roi ISR (revalidate = 60). */
export async function generateStaticParams() {
  return [];
}

/** Ton kho chi mang tinh goi y: fetch availability bao lam moi 10s (trang van revalidate = 60). */
const FETCH_OPTS = { revalidate: 10 } as const;

export async function generateMetadata({
  params,
}: PageProps): Promise<Metadata> {
  const { slug } = await params;
  // Backend loi: dung metadata chung, khong lam hong trang.
  const event = await getEventBySlug(slug, FETCH_OPTS).catch(() => null);

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
  let event: Awaited<ReturnType<typeof getEventBySlug>>;
  try {
    event = await getEventBySlug(slug, FETCH_OPTS);
  } catch {
    // Mang / 5xx / timeout: hien trang loi co lien ket thu lai, khong nem ra ngoai render.
    return (
      <div className="py-xl" lang="vi">
        <Alert variant="error" title="Không tải được sự kiện">
          Hệ thống đang bận hoặc mất kết nối. Vui lòng{" "}
          <Link href={`/events/${encodeURIComponent(slug)}`} className="underline">
            thử lại
          </Link>{" "}
          sau ít phút.
        </Alert>
      </div>
    );
  }

  if (!event) {
    notFound();
  }

  const badgeInfo = STATE_BADGE[event.state] ?? {
    label: event.state,
    variant: "neutral" as BadgeVariant,
  };

  // Cung dieu kien voi TicketTierList (khong co hang ve thi khong render h2).
  const hasTiers = Array.isArray(event.tiers) && event.tiers.length > 0;

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
      {/* Chi tro aria-labelledby khi TicketTierList that su render h2#ticket-tiers-heading (co hang ve);
        * neu khong id khong ton tai va axe bao aria-valid-attr-value. */}
      <section aria-labelledby={hasTiers ? TICKET_TIERS_HEADING_ID : undefined}>
        <TicketTierList tiers={event.tiers} headingId={TICKET_TIERS_HEADING_ID} />
      </section>
      {hasTiers ? (
        <p className="mt-md text-body-sm text-fg-muted" lang="vi" data-testid="availability-note">
          Tình trạng vé chỉ mang tính tham khảo và có thể thay đổi; hạng vé &ldquo;Chưa rõ tình trạng&rdquo; đang
          được cập nhật.
        </p>
      ) : null}
    </article>
  );
}
