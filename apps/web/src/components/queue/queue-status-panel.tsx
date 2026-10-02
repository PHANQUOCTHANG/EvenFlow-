"use client";

import { Badge, Button, type BadgeVariant } from "@/components/ui";
import { ServerExpiryCountdown } from "@/components/ui/server-expiry-countdown";
import { cn } from "@/lib/cn";
import { spellDuration } from "@/lib/format";

/** 8 trang thai. Handoff muc 11.3 liet 7 (thieu DROPPED), nhung state machine o
 *  docs/01-nghiep-vu.md dong 48 co `QUEUED --rot ket noi > 90s--> (giu cho 5 phut) --> DROPPED`.
 *  docs/01 la nguon chuan nghiep vu, handoff tu nhan la "de xuat UX/UI", nen lam theo docs/01.
 *  DROPPED KHAC EXPIRED: EXPIRED la het TTL suat mua (da toi luot ma khong mua kip),
 *  DROPPED la mat ket noi qua lau khi con dang xep hang. Thong diep cho khach khac hoan toan. */
export type QueueState =
  | "LOBBY"
  | "QUEUED"
  | "ADMITTED"
  | "EXPIRED"
  | "SOLD_OUT"
  | "RECONNECTING"
  | "DROPPED"
  | "UNKNOWN";

export type ConnectionState = "live" | "reconnecting" | "offline";

interface StateCopy {
  badge: BadgeVariant;
  badgeText: string;
  title: string;
  description: string;
}

const COPY: Record<QueueState, StateCopy> = {
  LOBBY: {
    badge: "lobby",
    badgeText: "Phòng chờ",
    title: "Bạn đang ở phòng chờ",
    // BR-Q1: phai noi THANG hai dieu.
    // (1) "Ban chua co so thu tu" — neu khong khach se tuong he thong loi hoac an so cua ho.
    // (2) Vao som khong co loi the — neu khong khach tu suy ra dieu nguoc lai va di bam F5,
    //     dung cai ma co che xao tron sinh ra de triet tieu.
    description:
      "Bạn chưa có số thứ tự. Khi đến giờ mở bán, toàn bộ phòng chờ được xáo trộn ngẫu nhiên rồi mới cấp số. Vào sớm hơn không tạo lợi thế, nên bạn không cần tải lại trang.",
  },
  QUEUED: {
    badge: "queued",
    badgeText: "Đang xếp hàng",
    title: "Bạn đã có số thứ tự",
    description: "Giữ nguyên tab này. Trang tự cập nhật theo nhịp do hệ thống quyết định.",
  },
  ADMITTED: {
    badge: "admitted",
    badgeText: "Tới lượt bạn",
    title: "Đến lượt bạn mua vé",
    description: "Hãy hoàn tất chọn vé và thanh toán trước khi suất mua hết hạn.",
  },
  EXPIRED: {
    badge: "expired",
    badgeText: "Hết hạn",
    title: "Suất mua đã hết hạn",
    // BR-Q7: het TTL thi khong duoc uu tien khi xep lai. Phai noi ro, khong de khach tuong
    // ho giu duoc cho cu.
    description: "Bạn cần xếp hàng lại và sẽ không được ưu tiên so với người mới vào.",
  },
  SOLD_OUT: {
    badge: "soldout",
    badgeText: "Hết vé",
    // BR-Q6: het ve thi khong day thêm ai vao checkout -> khong co CTA vao thanh toan.
    title: "Đã hết vé",
    description: "Không còn ai được đưa vào thanh toán cho đợt mở bán này.",
  },
  RECONNECTING: {
    badge: "pending",
    badgeText: "Đang kết nối lại",
    // docs/01 dong 48: rot ket noi > 90s thi VAN giu cho 5 phut. Mat ket noi khong dong
    // nghia mat cho — day la dieu khach lo nhat nen phai noi truoc.
    title: "Đang kết nối lại",
    description: "Bạn chưa mất chỗ. Hệ thống vẫn giữ vị trí của bạn trong lúc kết nối lại.",
  },
  DROPPED: {
    badge: "failed",
    badgeText: "Mất chỗ",
    title: "Chỗ của bạn đã được trả lại",
    description:
      "Kết nối bị ngắt quá lâu nên vị trí trong hàng đợi đã được trả lại. Bạn cần vào xếp hàng lại từ đầu.",
  },
  UNKNOWN: {
    badge: "neutral",
    badgeText: "Chưa rõ",
    // Handoff muc 5 nguyen tac 3: thieu du lieu thi noi la thieu, khong bia trang thai.
    title: "Chưa xác định được trạng thái",
    description: "Hệ thống chưa trả về trạng thái hàng đợi. Vui lòng thử lại sau một lát.",
  },
};

const CONNECTION_TEXT: Record<ConnectionState, string> = {
  live: "Đang cập nhật trực tiếp",
  reconnecting: "Đang kết nối lại",
  offline: "Mất kết nối",
};

export interface QueueStatusPanelProps {
  state: QueueState;
  /** Rank HIEN TAI. BI BO QUA khi state = LOBBY (BR-Q1). */
  rank?: number;
  /**
   * Rank cua chinh nguoi dung LUC VAO hang doi — mau so co dinh cua thanh tien trinh.
   *
   * Co y KHONG dung "so nguoi dang o phia truoc": con so do GIAM dan theo thoi gian, nen
   * lay lam mau so thi thanh tien trinh se nhay nguoc. Ngoai ra `rank` va "so nguoi phia
   * truoc" chenh nhau dung 1, nen `soNguoiPhiaTruoc - rank` luon ra khoang 0.
   *
   * Thieu prop nay thi KHONG ve progress (handoff muc 11.3: "no fake progress").
   */
  initialRank?: number;
  /** Chi do SERVER cung cap. Thieu thi hien "dang cap nhat", KHONG tu tinh tu rank. */
  etaSeconds?: number;
  admissionExpiresAt?: number | string;
  /** Nguong canh bao cho dong ho suat mua. Khong truyen = khong canh bao (policy nghiep vu,
   *  component khong tu bia nguong). */
  admissionWarningThresholdMs?: number;
  /** BR-Q6: het ve thi QUEUED "duoc moi vao waitlist". Chi render nut khi caller thuc su co
   *  cho de moi — khong co thi khong nhac waitlist, tranh hua mot tinh nang chua ton tai. */
  onJoinWaitlist?: () => void;
  lastUpdatedAt?: number | string;
  connectionState?: ConnectionState;
  offsetMs?: number;
  className?: string;
}

function formatClockTime(value: number | string): string | null {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

export function QueueStatusPanel({
  state,
  rank,
  initialRank,
  etaSeconds,
  admissionExpiresAt,
  admissionWarningThresholdMs,
  onJoinWaitlist,
  lastUpdatedAt,
  connectionState,
  offsetMs,
  className,
}: QueueStatusPanelProps) {
  const copy = COPY[state];

  // BR-Q1: o LOBBY thi rank/ETA/progress bi BO QUA ngay ca khi caller truyen vao. Chot o
  // day thay vi tin caller, vi mot cho hien nham so thu tu la pha ca co che chong bot.
  const showRank = state === "QUEUED" && typeof rank === "number";
  const showEta = state === "QUEUED";
  const showProgress =
    state === "QUEUED" &&
    typeof rank === "number" &&
    typeof initialRank === "number" &&
    initialRank > 0;

  // Da duoc goi bao nhieu nguoi ke tu luc minh vao. `rank` giam dan nen gia tri nay chi
  // tang — thanh tien trinh khong bao gio nhay nguoc.
  const cleared = showProgress
    ? Math.min(initialRank as number, Math.max(0, (initialRank as number) - (rank as number)))
    : 0;
  const updatedAt = lastUpdatedAt === undefined ? null : formatClockTime(lastUpdatedAt);

  return (
    <section
      data-state={state}
      className={cn(
        "flex flex-col gap-md rounded-card border border-border bg-surface p-md shadow-card",
        className,
      )}
    >
      {/* Vung live RIENG chi chua tieu de trang thai.
        * KHONG dat aria-live tren ca section: ben trong co dong ho, va text thay the cua no
        * doi moi tick — screen reader se bi doc lai lien tuc moi giay. */}
      <span role="status" aria-live="polite" className="sr-only">
        {copy.title}
      </span>

      <div className="flex flex-wrap items-center justify-between gap-sm">
        <Badge variant={copy.badge}>{copy.badgeText}</Badge>
        {connectionState ? (
          <span className="text-body-sm text-fg-muted">{CONNECTION_TEXT[connectionState]}</span>
        ) : null}
      </div>

      <div className="flex flex-col gap-xs">
        <h2 className="text-headline-md text-fg">{copy.title}</h2>
        <p className="text-body-md text-fg-muted">{copy.description}</p>
      </div>

      {showRank ? (
        <div className="flex flex-col gap-xs">
          <span className="text-label-md text-fg-muted">Số thứ tự của bạn</span>
          <span className="text-numeric-metric tabular-nums text-fg">{rank}</span>
        </div>
      ) : null}

      {showProgress ? (
        <div
          role="progressbar"
          aria-valuemin={0}
          aria-valuemax={initialRank}
          aria-valuenow={cleared}
          aria-label="Tiến trình hàng đợi"
          className="h-2 w-full overflow-hidden rounded-full bg-neutral-soft"
        >
          <div
            className="h-full bg-primary"
            style={{ width: `${Math.min(100, (cleared / (initialRank as number)) * 100)}%` }}
          />
        </div>
      ) : null}

      {showEta ? (
        <p className="text-body-md text-fg-muted">
          {typeof etaSeconds === "number"
            ? // Chi hien con so khi SERVER cung cap. Khong tu suy ra tu rank.
              `Thời gian ước tính: ~${spellDuration(etaSeconds * 1000)}`
            : "Thời gian ước tính: đang cập nhật"}
        </p>
      ) : null}

      {state === "ADMITTED" && admissionExpiresAt !== undefined ? (
        // Variant "admission" (TTL 15 phut), KHONG phai "hold" (10 phut) — BR-Q7.
        <ServerExpiryCountdown
          variant="admission"
          expiresAt={admissionExpiresAt}
          offsetMs={offsetMs}
          warningThresholdMs={admissionWarningThresholdMs}
        />
      ) : null}

      {state === "SOLD_OUT" && onJoinWaitlist ? (
        // BR-Q6 chi noi "duoc moi vao waitlist", khong noi waitlist luon san co. Nen nut nay
        // chi xuat hien khi caller thuc su co cho de moi.
        <Button variant="secondary" onClick={onJoinWaitlist}>
          Vào danh sách chờ
        </Button>
      ) : null}

      {updatedAt ? (
        <p className="text-body-sm text-fg-muted">Cập nhật lúc {updatedAt}</p>
      ) : null}
    </section>
  );
}
