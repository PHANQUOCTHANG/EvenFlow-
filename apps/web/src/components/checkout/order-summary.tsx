"use client";

import { useId, useState } from "react";

import { Alert, Button, ServerExpiryCountdown } from "@/components/ui";
import { cn } from "@/lib/cn";
import { formatMoney } from "@/lib/money";

/** `ambiguous` la trang thai THAT, khong phai truong hop bien.
 *
 * BR-O5 bat moi API ghi dung `Idempotency-Key` chinh vi request co the khong biet ket qua.
 * Khi do UI khong duoc noi thanh cong, khong duoc noi that bai, va khong duoc moi bam lai bang
 * mot request moi — bam lai voi key moi la nguy co tao hold trung, tuc la giu hai lan kho. */
export type HoldState = "idle" | "creating" | "active" | "expired" | "sold_out" | "ambiguous";

export interface OrderSummaryItem {
  tierName: string;
  quantity: number;
  unitAmount: number;
  currency: string;
}

export interface OrderSummaryProps {
  items: OrderSummaryItem[];
  holdState: HoldState;
  holdExpiresAt?: number | string;
  offsetMs?: number;
  holdWarningThresholdMs?: number;
  /** Thieu -> KHONG render dong phi. Mot dong "Phi: 0 đ" la khang dinh rang khong co phi,
   *  ma ta khong biet dieu do (handoff muc 11.6: "display fees/taxes only if supplied"). */
  fees?: number;
  primaryActionLabel?: string;
  onPrimaryAction?: () => void;
  className?: string;
}

interface StateNotice {
  variant: "info" | "success" | "warning" | "error";
  title: string;
  body: string;
}

const NOTICE: Partial<Record<HoldState, StateNotice>> = {
  creating: {
    variant: "info",
    title: "Đang giữ vé",
    // KHONG noi da giu duoc.
    body: "Hệ thống đang đặt chỗ cho bạn. Vui lòng không đóng trang.",
  },
  expired: {
    variant: "error",
    title: "Đã hết thời gian giữ vé",
    body: "Số vé bạn chọn đã được trả lại kho. Bạn cần chọn lại vé để tiếp tục.",
  },
  sold_out: {
    variant: "error",
    title: "Đã hết vé",
    body: "Phần vé bạn chọn vừa được người khác mua hết.",
  },
  ambiguous: {
    variant: "warning",
    title: "Chưa xác định được kết quả",
    // Khong thanh cong, khong that bai, va khong moi bam lai.
    body:
      "Yêu cầu giữ vé đã được gửi nhưng chưa có phản hồi. Vui lòng chờ hệ thống xác nhận, đừng gửi lại yêu cầu — gửi lại có thể tạo lượt giữ vé trùng.",
  },
};

const PRIMARY_LABEL: Partial<Record<HoldState, string>> = {
  idle: "Giữ vé và thanh toán",
  creating: "Đang giữ vé",
  active: "Tiếp tục thanh toán",
  expired: "Chọn lại vé",
};

export function OrderSummary({
  items,
  holdState,
  holdExpiresAt,
  offsetMs,
  holdWarningThresholdMs,
  fees,
  primaryActionLabel,
  onPrimaryAction,
  className,
}: OrderSummaryProps) {
  const detailsId = useId();
  const [expanded, setExpanded] = useState(false);

  const currencies = new Set(items.map((item) => item.currency));
  const mixedCurrency = currencies.size > 1;
  const currency = items[0]?.currency;

  const subtotal = items.reduce((sum, item) => sum + item.unitAmount * item.quantity, 0);
  const hasFees = Number.isFinite(fees);
  const total = hasFees ? subtotal + (fees as number) : subtotal;

  const notice = NOTICE[holdState];
  // sold_out khong moi thanh toan; ambiguous khong co nut nao co the tao hold moi.
  const showPrimary = holdState !== "sold_out" && holdState !== "ambiguous";

  // `expired` la trang thai DUY NHAT component tu giu nhan, de ghi de nhan cua caller.
  // Ly do: neu caller truyen "Thanh toan ngay" roi hold het han, nut van moi di thanh toan
  // trong khi ve DA duoc tra ve kho — hanh dong khach can lam luc do la CHON LAI, viec khac
  // han. Cung nguyen tac voi LOBBY bo qua `rank`: chot o component thay vi tin caller.
  //
  // O `creating` thi khong ghi de: nut da bi disable va co spinner, giu nguyen nhan cua caller
  // lam nguoi dung thay lien mach hon.
  const callerMayLabel = holdState !== "expired";
  const label =
    (callerMayLabel ? primaryActionLabel : undefined) ?? PRIMARY_LABEL[holdState] ?? "Tiếp tục";

  return (
    <section
      aria-label="Tóm tắt đơn hàng"
      data-hold-state={holdState}
      className={cn(
        "flex flex-col gap-md rounded-card border border-border bg-surface p-md shadow-float",
        "md:sticky md:top-md",
        className,
      )}
    >
      <h2 className="text-headline-sm text-fg">Tóm tắt đơn hàng</h2>

      {holdState === "active" && holdExpiresAt !== undefined ? (
        // Variant "hold" (10 phut), KHONG phai "admission" (15 phut) — BR-Q7.
        <ServerExpiryCountdown
          variant="hold"
          expiresAt={holdExpiresAt}
          offsetMs={offsetMs}
          warningThresholdMs={holdWarningThresholdMs}
        />
      ) : null}

      {notice ? (
        <Alert variant={notice.variant} title={notice.title}>
          {notice.body}
        </Alert>
      ) : null}

      {/* Mobile: thu gon duoc. Nut chinh nam NGOAI vung thu gon nen khong bao gio bi an
        * (handoff muc 11.6: "without hiding the primary action"). */}
      <button
        type="button"
        aria-expanded={expanded}
        aria-controls={detailsId}
        onClick={() => setExpanded((value) => !value)}
        className="ef-focus-ring rounded-control text-left text-label-lg text-primary md:hidden"
      >
        Chi tiết đơn hàng
      </button>

      <div id={detailsId} className={cn("flex-col gap-sm", expanded ? "flex" : "hidden md:flex")}>
        {items.length === 0 ? (
          // KHONG hien "0 đ" nhu mot tong hop le khi chua chon gi.
          <p className="text-body-md text-fg-muted">Bạn chưa chọn vé nào.</p>
        ) : (
          <>
            <ul className="flex flex-col gap-sm">
              {items.map((item) => (
                <li key={item.tierName} className="flex items-start justify-between gap-md">
                  <span className="text-body-md text-fg">
                    {item.tierName}
                    <span className="text-fg-muted">
                      {" × "}
                      <span className="tabular-nums">{item.quantity}</span>
                    </span>
                    <span className="block text-body-sm text-fg-muted">
                      Đơn giá {formatMoney(item.unitAmount, item.currency)}
                    </span>
                  </span>
                  <span className="tabular-nums text-body-md text-fg">
                    {formatMoney(item.unitAmount * item.quantity, item.currency)}
                  </span>
                </li>
              ))}
            </ul>

            {/* Tron nhieu tien te: VAN giu tung dong (khong mat thong tin cua khach), chi tu
              * choi CONG. Cong lan hai don vi ra mot con so sai hoan toan. */}
            {mixedCurrency ? (
              <Alert variant="error" title="Không thể tính tổng">
                Đơn hàng đang có nhiều loại tiền tệ khác nhau nên hệ thống không cộng tổng. Vui lòng
                tải lại trang.
              </Alert>
            ) : (
              <>
            <div className="flex items-center justify-between gap-md border-t border-border pt-sm">
              <span className="text-body-md text-fg-muted">Tạm tính</span>
              <span className="tabular-nums text-body-md text-fg">
                {formatMoney(subtotal, currency)}
              </span>
            </div>

            {hasFees ? (
              <div className="flex items-center justify-between gap-md">
                <span className="text-body-md text-fg-muted">Phí</span>
                <span className="tabular-nums text-body-md text-fg">
                  {formatMoney(fees as number, currency)}
                </span>
              </div>
            ) : null}

            <div className="flex items-center justify-between gap-md border-t border-border pt-sm">
              <span className="text-label-lg text-fg">Tổng cộng</span>
              <span className="tabular-nums text-numeric-metric text-fg">
                {formatMoney(total, currency)}
              </span>
            </div>
              </>
            )}
          </>
        )}
      </div>

      {showPrimary ? (
        <Button
          loading={holdState === "creating"}
          disabled={items.length === 0 || mixedCurrency}
          onClick={onPrimaryAction}
        >
          {label}
        </Button>
      ) : null}
    </section>
  );
}
