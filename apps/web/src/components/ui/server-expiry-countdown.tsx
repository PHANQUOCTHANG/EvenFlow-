"use client";

import { useEffect, useState } from "react";

import { useServerCountdown } from "@/hooks/use-server-countdown";
import { cn } from "@/lib/cn";
import { formatClock, spellDuration } from "@/lib/format";

/** Ba dong ho khac nhau trong he thong. BR-Q7: suat admit co TTL 15 phut, giu ve 10 phut —
 *  HAI dong ho khac nhau, khong bao gio duoc gop. */
export type CountdownVariant = "sale-start" | "admission" | "hold";

/** Nhan co dinh trong component, khong cho caller tu dat: neu caller dat nhan thi hai dong ho
 *  checkout se co luc bi mo ta giong nhau va khach khong phan biet duoc minh con bao nhieu
 *  thoi gian cho viec gi (handoff muc 11.4).
 *
 *  Nhan theo docs/01 BR-Q7: 15 phut la TTL cua SUAT MUA. DESIGN.md goi cung con so do la
 *  "Phien truy cap (Session TTL) — thoi han cua token xac thuc luong", day la thu KHAC;
 *  dung cach goi do se lam khach hieu sai minh dang dem cai gi. */
const LABEL: Record<CountdownVariant, string> = {
  "sale-start": "Mở bán sau",
  admission: "Suất mua của bạn còn",
  hold: "Thời gian giữ vé còn",
};

const EXPIRED_LABEL: Record<CountdownVariant, string> = {
  "sale-start": "Đã tới giờ mở bán",
  admission: "Suất mua đã hết hạn",
  hold: "Đã hết thời gian giữ vé",
};

export interface ServerExpiryCountdownProps {
  /** BAT BUOC, khong co default: xem comment o LABEL. */
  variant: CountdownVariant;
  /** Moc tuyet doi cua SERVER: epoch ms, hoac chuoi ISO-8601 CO mui gio. Khong nhan thoi luong. */
  expiresAt: number | string;
  offsetMs?: number;
  /** Nguong canh bao la POLICY nghiep vu, nen khong co gia tri mac dinh tu bia.
   *  Khong truyen = khong co trang thai canh bao. */
  warningThresholdMs?: number;
  className?: string;
}

const STATE_STYLE = {
  unknown: "border-border bg-surface text-fg-muted",
  running: "border-border bg-surface text-fg",
  // Vien dung accent (amber), CHU dung warning-fg: amber chi dat 2.19:1 tren nen trang
  // nen khong duoc lam mau chu (EVF-1801 spec muc 3.2).
  warning: "border-accent bg-warning-soft text-warning-fg",
  expired: "border-danger bg-danger-soft text-danger",
} as const;

/** DESIGN.md: moi chi bao countdown bat buoc ket hop van ban ro rang VA icon ngu nghia. */
const STATE_ICON = {
  unknown: "⏱",
  running: "⏱",
  warning: "⚠",
  expired: "⚠",
} as const;

export function ServerExpiryCountdown({
  variant,
  expiresAt,
  offsetMs,
  warningThresholdMs,
  className,
}: ServerExpiryCountdownProps) {
  const { remainingMs, expired, ready } = useServerCountdown(expiresAt, offsetMs);

  const warning =
    ready && !expired && warningThresholdMs !== undefined && remainingMs <= warningThresholdMs;

  const visualState = !ready ? "unknown" : expired ? "expired" : warning ? "warning" : "running";

  // Vung live CHI doi khi vao canh bao va khi het han. Neu thong bao moi giay thi screen
  // reader bi spam lien tuc va nguoi dung khong nghe duoc gi khac — dong ho phan tac dung.
  const [announcement, setAnnouncement] = useState("");
  useEffect(() => {
    if (expired) {
      setAnnouncement(EXPIRED_LABEL[variant]);
    } else if (warning && warningThresholdMs !== undefined) {
      setAnnouncement(`${LABEL[variant]} dưới ${spellDuration(warningThresholdMs)}`);
    } else {
      setAnnouncement("");
    }
  }, [expired, warning, variant, warningThresholdMs]);

  const visibleLabel = expired ? EXPIRED_LABEL[variant] : LABEL[variant];
  const srSentence = !ready
    ? `${LABEL[variant]} đang được xác định`
    : expired
      ? EXPIRED_LABEL[variant]
      : `${LABEL[variant]} ${spellDuration(remainingMs)}`;

  return (
    <div
      data-state={visualState}
      className={cn(
        "inline-flex items-center gap-sm rounded-control border px-md py-sm",
        STATE_STYLE[visualState],
        className,
      )}
    >
      {/* Ca phan nhin thay deu aria-hidden va duoc thay bang DUNG MOT cau o duoi. Neu khong,
        * screen reader doc nhan 2-3 lan ("Suat mua cua ban con. Suat mua cua ban con 9 phut"). */}
      <span aria-hidden="true">{STATE_ICON[visualState]}</span>
      <span aria-hidden="true" className="flex flex-col">
        <span className="text-label-md">{visibleLabel}</span>
        {expired ? null : (
          <span className="text-numeric-timer tabular-nums">
            {ready ? formatClock(remainingMs) : "--:--"}
          </span>
        )}
      </span>

      <span className="sr-only">{srSentence}</span>

      {/* Het han la su kien khach MAT ve: polite bi xep sau moi output dang doc (vi du khach
        * dang go so the), nen luc do phai assertive. Vao canh bao thi polite la dung. */}
      <span
        role={expired ? "alert" : "status"}
        aria-live={expired ? "assertive" : "polite"}
        className="sr-only"
      >
        {announcement}
      </span>
    </div>
  );
}
