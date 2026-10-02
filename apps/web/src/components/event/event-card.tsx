import Link from "next/link";

import { Badge, type BadgeVariant } from "@/components/ui";
import { cn } from "@/lib/cn";
import { toEpochMs } from "@/lib/server-time";

/** 6 trang thai cong khai theo state machine o docs/01-nghiep-vu.md dong 22-23:
 *  SCHEDULED -> ON_SALE -> SOLD_OUT | CLOSED -> COMPLETED, va nhanh CANCELLED.
 *
 *  Handoff muc 11.2 chi liet 4 nhan (scheduled / on-sale / sold-out / cancelled), thieu
 *  CLOSED va COMPLETED. Hai cai do KHAC nhau va khac SOLD_OUT: CLOSED la het gio ban (ve co
 *  the van con), COMPLETED la su kien da dien ra. docs/01 la nguon chuan nghiep vu. */
export type EventPublicState =
  | "SCHEDULED"
  | "ON_SALE"
  | "SOLD_OUT"
  | "CLOSED"
  | "COMPLETED"
  | "CANCELLED";

interface StateCopy {
  badge: BadgeVariant;
  badgeText: string;
  note: string;
  /** Giam nhan thi giac nhung VAN hien thi — DESIGN.md: giu bo cuc trang, khong an the. */
  dimmed: boolean;
}

const STATE: Record<EventPublicState, StateCopy> = {
  SCHEDULED: {
    badge: "pending",
    badgeText: "Chưa mở bán",
    note: "Sự kiện đã được duyệt, chưa tới giờ mở bán.",
    dimmed: false,
  },
  ON_SALE: {
    badge: "admitted",
    badgeText: "Đang mở bán",
    note: "",
    dimmed: false,
  },
  SOLD_OUT: {
    badge: "soldout",
    badgeText: "Hết vé",
    note: "Mọi hạng vé đã bán hết.",
    dimmed: true,
  },
  CLOSED: {
    badge: "neutral",
    badgeText: "Đã đóng bán",
    note: "Đã hết thời gian bán vé cho sự kiện này.",
    dimmed: true,
  },
  COMPLETED: {
    badge: "neutral",
    badgeText: "Đã diễn ra",
    note: "Sự kiện đã kết thúc.",
    dimmed: true,
  },
  CANCELLED: {
    badge: "failed",
    badgeText: "Đã huỷ",
    // Dung dung cach noi cua docs/01 dong 23: "CANCELLED (hoan tien toan bo)".
    note: "Sự kiện bị huỷ. Hoàn tiền toàn bộ cho mọi vé đã mua.",
    dimmed: true,
  },
};

/** Gio dia diem, co dinh +07:00.
 *
 * Hai ly do, khong phai de cho tien:
 * 1. Gio su kien la gio CUA DIA DIEM. Mot buoi dien 20:00 o Ha Noi van la 20:00 du khach dang
 *    xem tu Tokyo — doi sang gio cua nguoi xem moi la sai.
 * 2. Trang nay prerender tinh. Neu format theo mui gio cua MOI TRUONG dang render thi build
 *    (UTC) va browser (ICT) cho ra hai chuoi khac nhau -> React bao hydration mismatch. Doc
 *    cac phan UTC sau khi dich epoch la cach duy nhat cho ket qua xac dinh o ca hai phia.
 *
 * GIOI HAN DA BIET: +07:00 dang hard-code. Su kien ngoai Viet Nam se can server tra ve mui gio
 * cua dia diem. Ghi lai o spec, chua co field do trong ERD. */
const VENUE_OFFSET_MINUTES = 7 * 60;

function formatVenueTime(value: number | string): string | null {
  const ms = toEpochMs(value);
  if (ms === null) return null;

  const shifted = new Date(ms + VENUE_OFFSET_MINUTES * 60_000);
  const pad = (n: number) => String(n).padStart(2, "0");

  return (
    `${pad(shifted.getUTCDate())}/${pad(shifted.getUTCMonth() + 1)}/${shifted.getUTCFullYear()}` +
    ` ${pad(shifted.getUTCHours())}:${pad(shifted.getUTCMinutes())}`
  );
}

export interface EventCardProps {
  state: EventPublicState;
  title: string;
  venue?: string;
  /** Epoch ms hoac ISO-8601 CO mui gio. */
  startsAt?: number | string;
  imageUrl?: string;
  /** Khong co thi KHONG render link — mot link dan toi 404 te hon khong co link. */
  href?: string;
  loading?: boolean;
  className?: string;
}

const SHELL = "flex flex-col overflow-hidden rounded-card border border-border bg-surface shadow-card";

export function EventCard({
  state,
  title,
  venue,
  startsAt,
  imageUrl,
  href,
  loading = false,
  className,
}: EventCardProps) {
  if (loading) {
    return (
      <div aria-busy="true" className={cn(SHELL, className)}>
        <span className="sr-only">Đang tải thông tin sự kiện</span>
        {/* ef-pulse tat o prefers-reduced-motion, xem globals.css */}
        <div aria-hidden="true" className="ef-pulse aspect-[16/9] w-full bg-neutral-soft" />
        <div aria-hidden="true" className="flex flex-col gap-sm p-md">
          <div className="ef-pulse h-5 w-3/4 rounded-sm bg-neutral-soft" />
          <div className="ef-pulse h-4 w-1/2 rounded-sm bg-neutral-soft" />
        </div>
      </div>
    );
  }

  const copy = STATE[state];
  const time = startsAt === undefined ? null : formatVenueTime(startsAt);
  // Phai trim: `imageUrl="   "` la truthy nen se sinh <img src="   ">, browser gui mot request
  // rong roi bao loi anh hong. Chuoi toan khoang trang phai duoc coi nhu khong co anh.
  const image = typeof imageUrl === "string" && imageUrl.trim() !== "" ? imageUrl.trim() : null;

  return (
    <div data-state={state} className={cn(SHELL, copy.dimmed && "opacity-70", className)}>
      {image ? (
        /* Anh poster do organizer upload nen domain khong biet truoc; `next/image` can khai bao
           `images.remotePatterns`, ma backend chua chot nguon luu anh. Doi sang next/image khi
           co quyet dinh do. `alt=""` vi anh la trang tri — ten su kien da o <h3> ngay duoi. */
        // eslint-disable-next-line @next/next/no-img-element
        <img src={image} alt="" className="aspect-[16/9] w-full object-cover" />
      ) : (
        // Khong render <img src=""> khi thieu anh: browser se gui mot request rong va bao loi.
        <div aria-hidden="true" className="aspect-[16/9] w-full bg-neutral-soft" />
      )}

      <div className="flex flex-1 flex-col gap-sm p-md">
        <Badge variant={copy.badge}>{copy.badgeText}</Badge>
        <h3 className="text-headline-sm text-fg">{title}</h3>

        {venue ? <p className="text-body-md text-fg-muted">{venue}</p> : null}
        {time ? (
          <p className="text-body-md tabular-nums text-fg-muted">
            {time} <span className="text-body-sm">(giờ địa điểm)</span>
          </p>
        ) : null}
        {copy.note ? <p className="text-body-sm text-fg-muted">{copy.note}</p> : null}

        {href ? (
          <Link
            href={href}
            className="ef-focus-ring mt-auto self-start rounded-control text-label-lg text-primary"
          >
            Xem chi tiết
          </Link>
        ) : null}
      </div>
    </div>
  );
}
