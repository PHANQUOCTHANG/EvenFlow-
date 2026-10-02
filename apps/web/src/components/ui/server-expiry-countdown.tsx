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
 *  thoi gian cho viec gi (handoff muc 11.4). */
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
  /** Moc tuyet doi cua SERVER (epoch ms hoac ISO). Khong nhan thoi luong. */
  expiresAt: number | string;
  offsetMs?: number;
  /** Nguong canh bao la POLICY nghiep vu, nen khong co gia tri mac dinh tu bia.
   *  Khong truyen = khong co trang thai canh bao. */
  warningThresholdMs?: number;
  className?: string;
}

const STATE_STYLE = {
  running: "border-border bg-surface text-fg",
  // Vien dung accent (amber), CHU dung warning-fg: amber chi dat 2.19:1 tren nen trang
  // nen khong duoc lam mau chu (EVF-1801 spec muc 3.2).
  warning: "border-accent bg-warning-soft text-warning-fg",
  expired: "border-danger bg-danger-soft text-danger",
} as const;

export function ServerExpiryCountdown({
  variant,
  expiresAt,
  offsetMs,
  warningThresholdMs,
  className,
}: ServerExpiryCountdownProps) {
  const { remainingMs, expired } = useServerCountdown(expiresAt, offsetMs);

  const warning =
    !expired && warningThresholdMs !== undefined && remainingMs <= warningThresholdMs;

  const visualState = expired ? "expired" : warning ? "warning" : "running";

  // Vung live CHI doi khi vao canh bao va khi het han. Neu thong bao moi giay thi screen
  // reader bi spam lien tuc va nguoi dung khong nghe duoc gi khac — dong hoi phan tac dung.
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

  return (
    <div
      data-state={visualState}
      className={cn(
        "inline-flex flex-col gap-xs rounded-control border px-md py-sm",
        STATE_STYLE[visualState],
        className,
      )}
    >
      <span className="text-label-md">{expired ? EXPIRED_LABEL[variant] : LABEL[variant]}</span>

      {/* Vung so an khoi screen reader va thay bang chuoi doc duoc ben duoi:
        * doc "09:30" theo tung ky tu la vo nghia. */}
      {expired ? null : (
        <span aria-hidden="true" className="text-numeric-timer tabular-nums">
          {formatClock(remainingMs)}
        </span>
      )}

      <span className="sr-only">
        {expired ? EXPIRED_LABEL[variant] : `${LABEL[variant]} ${spellDuration(remainingMs)}`}
      </span>

      <span role="status" aria-live="polite" className="sr-only">
        {announcement}
      </span>
    </div>
  );
}
