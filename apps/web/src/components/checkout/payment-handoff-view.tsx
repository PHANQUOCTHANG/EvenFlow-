"use client";

import Link from "next/link";
import { useEffect } from "react";

import { Alert, Button, Card, Input, RadioGroup, ServerExpiryCountdown } from "@/components/ui";
import { useT } from "@/components/i18n/locale-provider";
import { usePaymentMethods } from "@/hooks/use-payment-methods";
import { usePaymentSession, type PaymentFormData } from "@/hooks/use-payment-session";
import { cn } from "@/lib/cn";
import { formatMoney } from "@/lib/money";
import type { PaymentInitiationResult, PaymentMethod } from "@/lib/payment-client";

export interface PaymentHandoffViewProps {
  eventId: string;
  eventTitle: string;
  venue: string;
  orderId: string;
  holdId: string;
  tierName: string;
  quantity: number;
  unitPrice: number;
  expiresAt?: string;
  offsetMs?: number;
  onPaymentSuccess?: (result: PaymentInitiationResult) => void;
  className?: string;
}

export function PaymentHandoffView({
  eventId,
  eventTitle,
  venue,
  orderId,
  holdId,
  tierName,
  quantity,
  unitPrice,
  expiresAt,
  offsetMs = 0,
  onPaymentSuccess,
  className,
}: PaymentHandoffViewProps) {
  const totalAmount = unitPrice * quantity;
  const t = useT();
  const methodsState = usePaymentMethods();

  const {
    formData,
    setFormData,
    fieldErrors,
    submitting,
    holdExpired,
    error,
    authRequired,
    submitPayment,
  } = usePaymentSession({
    eventId,
    orderId,
    holdId,
    amount: totalAmount,
    expiresAt,
    offsetMs,
    onSuccess: (result) => {
      if (onPaymentSuccess) {
        onPaymentSuccess(result);
      } else if (result.redirectUrl) {
        window.location.href = result.redirectUrl;
      }
    },
  });

  // Phuong thuc dang chon phai nam trong danh sach dang bat do backend tra (AC-13).
  const { status: methodsStatus, methods } = methodsState;
  useEffect(() => {
    if (methodsStatus !== "ready" || methods.length === 0) return;
    setFormData((prev) =>
      methods.some((m) => m.code === prev.method) ? prev : { ...prev, method: methods[0].code },
    );
  }, [methodsStatus, methods, setFormData]);

  const methodsReady = methodsState.status === "ready" && methodsState.methods.length > 0;

  const handleInputChange = (field: keyof PaymentFormData, value: string) => {
    setFormData((prev) => ({ ...prev, [field]: value }));
  };

  const handleMethodChange = (value: string) => {
    setFormData((prev) => ({ ...prev, method: value as PaymentMethod }));
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!methodsReady) return;
    await submitPayment();
  };

  return (
    <div className={cn("mx-auto max-w-content space-y-lg py-md", className)}>
      <header className="space-y-xs">
        <h1 className="text-display-sm text-fg">Thanh toán đơn hàng</h1>
        <p className="text-body-md text-fg-muted">
          {eventTitle} &bull; {venue}
        </p>
      </header>

      {/* Cảnh báo khi hold hết hạn (BR-O2) */}
      {holdExpired ? (
        <Alert
          variant="error"
          title="Đã hết thời gian giữ vé (BR-O2)"
          className="mb-md"
        >
          <p className="mb-sm">
            Thời hạn giữ ghế 10:00 cho đơn hàng này đã kết thúc. Số vé đã được hoàn trả lại kho theo quy định. Vui lòng quay lại chọn lại vé.
          </p>
          <Link
            href={`/checkout/${eventId}`}
            className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-border-strong bg-surface px-md py-sm text-label-md text-fg hover:bg-surface-subtle"
          >
            Quay lại chọn vé
          </Link>
        </Alert>
      ) : null}

      {/* Lỗi submit khác nếu có */}
      {error && !holdExpired ? (
        <Alert
          variant="error"
          title={authRequired ? t("checkout.authRequired") : "Không thể khởi tạo thanh toán"}
          className="mb-md"
        >
          {authRequired ? (
            <Link
              href="/auth/sign-in"
              className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-border-strong bg-surface px-md py-sm text-label-md text-fg hover:bg-surface-subtle"
            >
              {t("checkout.signInAction")}
            </Link>
          ) : (
            error
          )}
        </Alert>
      ) : null}

      <div className="grid grid-cols-1 items-start gap-lg lg:grid-cols-3">
        {/* Cột trái: Form thông tin & Chọn phương thức thanh toán */}
        <div className="space-y-lg lg:col-span-2">
          {/* Thông tin người nhận vé */}
          <Card className="space-y-md p-lg">
            <h2 className="text-headline-sm text-fg">Thông tin người nhận vé</h2>
            <form onSubmit={handleSubmit} id="payment-form" className="space-y-md">
              <Input
                label="Họ và tên người nhận"
                placeholder="Ví dụ: Nguyễn Văn A"
                value={formData.buyerName}
                onChange={(e) => handleInputChange("buyerName", e.target.value)}
                error={fieldErrors.buyerName}
                disabled={submitting || holdExpired}
                required
              />

              <Input
                label="Email nhận vé điện tử"
                type="email"
                placeholder="tenban@email.com"
                value={formData.buyerEmail}
                onChange={(e) => handleInputChange("buyerEmail", e.target.value)}
                error={fieldErrors.buyerEmail}
                helpText="Vé và mã QR check-in sẽ được gửi tới email này."
                disabled={submitting || holdExpired}
                required
              />

              <Input
                label="Số điện thoại liên hệ"
                placeholder="0912 345 678"
                value={formData.buyerPhone}
                onChange={(e) => handleInputChange("buyerPhone", e.target.value)}
                error={fieldErrors.buyerPhone}
                disabled={submitting || holdExpired}
                required
              />
            </form>
          </Card>

          {/* Phương thức thanh toán */}
          <Card className="space-y-md p-lg">
            <h2 className="text-headline-sm text-fg">Phương thức thanh toán</h2>
            <p className="text-body-sm text-fg-muted">
              Chọn cổng thanh toán an toàn được liên kết với EvenFlow.
            </p>

            {methodsState.status === "loading" ? (
              <p role="status" className="text-body-sm text-fg-muted">
                {t("payment.methodsLoading")}
              </p>
            ) : null}

            {methodsState.status === "error" ? (
              <Alert variant="error" title={t("payment.methodsErrorTitle")}>
                <p className="mb-sm">{t("payment.methodsErrorBody")}</p>
                <Button variant="secondary" onClick={methodsState.retry}>
                  {t("payment.methodsRetry")}
                </Button>
              </Alert>
            ) : null}

            {methodsState.status === "ready" && methodsState.methods.length === 0 ? (
              <Alert variant="warning">{t("payment.methodsEmpty")}</Alert>
            ) : null}

            {methodsReady ? (
              <RadioGroup
                name="payment-method"
                legend="Chọn cổng thanh toán"
                value={formData.method}
                onValueChange={handleMethodChange}
                options={methodsState.methods.map((m) => ({
                  value: m.code,
                  label: m.description ? `${m.name} — ${m.description}` : m.name,
                  disabled: submitting || holdExpired,
                }))}
              />
            ) : null}
          </Card>
        </div>

        {/* Cột phải: Tóm tắt đơn hàng & Đồng hồ giữ vé */}
        <aside className="space-y-md lg:col-span-1">
          <Card className="space-y-md p-lg">
            {/* Đồng hồ giữ vé 10:00 (BR-O2) */}
            {expiresAt ? (
              <div className="border-b border-border pb-md">
                <ServerExpiryCountdown
                  expiresAt={expiresAt}
                  offsetMs={offsetMs}
                  variant="hold"
                />
              </div>
            ) : null}

            <h2 className="text-headline-sm text-fg">Tóm tắt đơn hàng</h2>

            <div className="space-y-sm text-body-sm">
              <div className="flex justify-between">
                <span className="text-fg-muted">Hạng vé</span>
                <span className="font-semibold text-fg">{tierName}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-fg-muted">Số lượng</span>
                <span className="font-semibold text-fg">{quantity} vé</span>
              </div>
              <div className="flex justify-between">
                <span className="text-fg-muted">Đơn giá</span>
                <span className="text-fg">{formatMoney(unitPrice, "VND")}</span>
              </div>
              <div className="flex justify-between border-t border-border pt-sm text-headline-sm">
                <span className="text-fg">Tổng thanh toán</span>
                {/* Khong dung text-accent: --ef-accent chi 2.18:1 tren nen trang (tokens.css cam dung lam mau chu). */}
                <span className="font-bold text-warning-fg">
                  {formatMoney(totalAmount, "VND")}
                </span>
              </div>
            </div>

            <div className="pt-sm">
              <Button
                type="submit"
                form="payment-form"
                variant="primary"
                disabled={submitting || holdExpired || !methodsReady}
                loading={submitting}
                onClick={handleSubmit}
                className="w-full py-md text-label-md"
              >
                {holdExpired
                  ? "Hết hạn giữ vé"
                  : `Thanh toán ${formatMoney(totalAmount, "VND")}`}
              </Button>
            </div>

            <p className="text-caption text-fg-subtle">
              Bằng việc bấm thanh toán, bạn đồng ý với Điều khoản mua vé của EvenFlow. Giao dịch được bảo vệ và mã hóa SSL 256-bit.
            </p>
          </Card>
        </aside>
      </div>
    </div>
  );
}
