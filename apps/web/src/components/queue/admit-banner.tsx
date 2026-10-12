"use client";

import Link from "next/link";
import { Button, ServerExpiryCountdown } from "@/components/ui";
import { cn } from "@/lib/cn";

export interface AdmitBannerProps {
  expiresAt?: number | string | null;
  offsetMs?: number;
  onProceed?: () => void;
  checkoutUrl?: string;
  className?: string;
}

/** Banner thông báo khi khách đã được cấp quyền vào lượt mua vé (EV-182 / AC-6).
 *
 * Chứa:
 * - role="alert" để thông báo ngay lập tức cho screen reader.
 * - Đồng hồ đếm ngược suất mua 15 phút (BR-Q7) theo expires_at của server.
 * - Nút hành động dẫn trực tiếp sang bước chọn vé. */
export function AdmitBanner({
  expiresAt,
  offsetMs,
  onProceed,
  checkoutUrl,
  className,
}: AdmitBannerProps) {
  return (
    <section
      role="alert"
      aria-live="assertive"
      className={cn(
        "flex flex-col md:flex-row items-center justify-between gap-md p-lg rounded-container border-2 border-success bg-success-soft shadow-card",
        className
      )}
    >
      <div className="flex flex-col gap-xs text-center md:text-left">
        <h2 className="text-heading-md font-bold text-success-fg">
          🎉 Đã đến lượt bạn mua vé!
        </h2>
        <p className="text-body-md text-fg">
          Suất mua của bạn có hiệu lực trong thời gian giới hạn. Vui lòng hoàn tất trong thời gian quy định.
        </p>

        {expiresAt ? (
          <div className="mt-xs">
            <ServerExpiryCountdown
              variant="admission"
              expiresAt={expiresAt}
              offsetMs={offsetMs}
            />
          </div>
        ) : null}
      </div>

      <div className="flex-shrink-0 w-full md:w-auto">
        {checkoutUrl && !onProceed ? (
          <Link href={checkoutUrl} className="w-full md:w-auto">
            <Button variant="primary" size="lg" className="w-full md:w-auto font-bold shadow-md">
              Tiến hành chọn vé
            </Button>
          </Link>
        ) : (
          <Button
            variant="primary"
            size="lg"
            onClick={onProceed}
            className="w-full md:w-auto font-bold shadow-md"
          >
            Tiến hành chọn vé
          </Button>
        )}
      </div>
    </section>
  );
}
