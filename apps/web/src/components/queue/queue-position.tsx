"use client";

import type { QueueState } from "@/components/queue/queue-status-panel";
import { ProgressRing } from "@/components/queue/progress-ring";
import { spellDuration } from "@/lib/format";
import { cn } from "@/lib/cn";

export interface QueuePositionProps {
  state: QueueState;
  rank: number | null;
  etaSeconds: number | null;
  initialRank: number | null;
  className?: string;
}

/** Component hiển thị thứ hạng, ETA và tiến trình người dùng trong hàng đợi (EV-182 / AC-5).
 *
 * Tuân thủ triệt để:
 * - BR-Q1: Ở LOBBY không hiển thị rank, không đoán thứ tự.
 * - AC-5: ETA chỉ hiển thị khi server cung cấp (etaSeconds >= 0), nếu không ghi 'đang cập nhật'.
 * - Tiến trình trung thực qua ProgressRing. */
export function QueuePosition({
  state,
  rank,
  etaSeconds,
  initialRank,
  className,
}: QueuePositionProps) {
  if (state === "LOBBY") {
    return (
      <div
        className={cn(
          "flex flex-col items-center justify-center p-lg rounded-container border border-border bg-surface text-center gap-sm",
          className
        )}
      >
        <span className="text-display-xs text-primary font-bold">Phòng chờ chung</span>
        <p className="text-body-md text-fg-muted max-w-md">
          Bạn chưa có số thứ tự. Khi đến giờ mở bán, hệ thống sẽ xáo trộn ngẫu nhiên để đảm bảo công bằng.
        </p>
      </div>
    );
  }

  const showRank = state === "QUEUED" && rank !== null;
  const validEta = typeof etaSeconds === "number" && etaSeconds >= 0 ? etaSeconds : null;

  return (
    <div
      className={cn(
        "flex flex-col sm:flex-row items-center justify-around gap-lg p-lg rounded-container border border-border bg-surface shadow-card",
        className
      )}
    >
      {/* Vòng tiến trình */}
      <div className="flex flex-col items-center gap-xs">
        <ProgressRing rank={rank} initialRank={initialRank} size={130} />
      </div>

      {/* Thông số vị trí và ETA */}
      <div className="flex flex-col items-center sm:items-start text-center sm:text-left gap-sm">
        {showRank ? (
          <div>
            <span className="text-label-md text-fg-muted block">Số thứ tự của bạn</span>
            <span className="text-numeric-metric font-extrabold text-primary tabular-nums">
              {rank}
            </span>
          </div>
        ) : (
          <div>
            <span className="text-label-md text-fg-muted block">Trạng thái</span>
            <span className="text-headline-md font-bold text-fg">
              {state === "ADMITTED" ? "Đã tới lượt bạn" : "Đang cập nhật vị trí..."}
            </span>
          </div>
        )}

        {state === "QUEUED" && (
          <p className="text-body-md text-fg-muted">
            {validEta !== null
              ? `Thời gian ước tính: ~${spellDuration(validEta * 1000)}`
              : "Thời gian ước tính: đang cập nhật"}
          </p>
        )}
      </div>
    </div>
  );
}
