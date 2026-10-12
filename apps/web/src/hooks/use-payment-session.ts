"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { useServerCountdown } from "@/hooks/use-server-countdown";
import { clearActiveHold } from "@/lib/hold-client";
import {
  initiatePayment,
  PaymentClientError,
  type InitiatePaymentParams,
  type PaymentInitiationResult,
  type PaymentMethod,
} from "@/lib/payment-client";

export interface PaymentFormData {
  buyerName: string;
  buyerEmail: string;
  buyerPhone: string;
  method: PaymentMethod;
}

export interface UsePaymentSessionOptions {
  eventId: string;
  orderId: string;
  holdId: string;
  amount: number;
  expiresAt?: string;
  offsetMs?: number;
  apiBaseUrl?: string;
  onSuccess?: (result: PaymentInitiationResult) => void;
}

export interface UsePaymentSessionReturn {
  formData: PaymentFormData;
  setFormData: React.Dispatch<React.SetStateAction<PaymentFormData>>;
  fieldErrors: Partial<Record<keyof PaymentFormData, string>>;
  submitting: boolean;
  holdExpired: boolean;
  remainingMs: number;
  error: string | null;
  submitPayment: () => Promise<PaymentInitiationResult | null>;
}

export function validatePaymentForm(data: PaymentFormData): Partial<Record<keyof PaymentFormData, string>> {
  const errors: Partial<Record<keyof PaymentFormData, string>> = {};

  if (!data.buyerName.trim()) {
    errors.buyerName = "Vui lòng nhập họ và tên người nhận vé";
  }

  if (!data.buyerEmail.trim()) {
    errors.buyerEmail = "Vui lòng nhập email nhận vé";
  } else if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(data.buyerEmail)) {
    errors.buyerEmail = "Địa chỉ email không đúng định dạng";
  }

  if (!data.buyerPhone.trim()) {
    errors.buyerPhone = "Vui lòng nhập số điện thoại";
  } else if (!/^[0-9+() -]{9,15}$/.test(data.buyerPhone.trim())) {
    errors.buyerPhone = "Số điện thoại không hợp lệ";
  }

  return errors;
}

/** Hook quản lý phiên thanh toán và theo dõi hạn giữ vé (EVF-114, BR-O2, BR-O5).
 *
 * 1. BR-O2: Khóa thanh toán và chuyển holdExpired = true ngay khi đồng hồ server chạm 0.
 * 2. BR-O5: Chống submit trùng (submitting + ref lock), đính kèm Idempotency-Key.
 */
export function usePaymentSession({
  eventId,
  orderId,
  holdId,
  amount,
  expiresAt,
  offsetMs = 0,
  apiBaseUrl,
  onSuccess,
}: UsePaymentSessionOptions): UsePaymentSessionReturn {
  const [formData, setFormData] = useState<PaymentFormData>({
    buyerName: "",
    buyerEmail: "",
    buyerPhone: "",
    method: "sandbox",
  });
  const [fieldErrors, setFieldErrors] = useState<Partial<Record<keyof PaymentFormData, string>>>({});
  const [submitting, setSubmitting] = useState(false);
  const [holdExpired, setHoldExpired] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const isSubmittingRef = useRef(false);

  // Dong ho dem nguoc giu ve tu server (BR-O2)
  const countdown = useServerCountdown(expiresAt, offsetMs);

  useEffect(() => {
    if (countdown.ready && countdown.expired) {
      setHoldExpired(true);
      clearActiveHold(eventId);
    }
  }, [countdown.ready, countdown.expired, eventId]);

  const submitPayment = useCallback(async (): Promise<PaymentInitiationResult | null> => {
    if (holdExpired || countdown.expired) {
      setError("Phiên giữ vé đã hết hạn. Vui lòng quay lại chọn vé mới.");
      return null;
    }

    if (submitting || isSubmittingRef.current) {
      return null;
    }

    const validation = validatePaymentForm(formData);
    setFieldErrors(validation);
    if (Object.keys(validation).length > 0) {
      return null;
    }

    isSubmittingRef.current = true;
    setSubmitting(true);
    setError(null);

    try {
      const params: InitiatePaymentParams = {
        eventId,
        orderId,
        holdId,
        amount,
        method: formData.method,
        buyerName: formData.buyerName.trim(),
        buyerEmail: formData.buyerEmail.trim(),
        buyerPhone: formData.buyerPhone.trim(),
        apiBaseUrl,
      };

      const result = await initiatePayment(params);
      if (onSuccess) {
        onSuccess(result);
      }
      return result;
    } catch (err) {
      if (err instanceof PaymentClientError) {
        if (err.type === "HOLD_EXPIRED") {
          setHoldExpired(true);
          clearActiveHold(eventId);
          setError("Phiên giữ vé đã hết hạn trong lúc thanh toán (BR-O2)");
        } else {
          setError(err.message);
        }
      } else {
        setError("Đã xảy ra lỗi không xác định khi bắt đầu thanh toán");
      }
      return null;
    } finally {
      isSubmittingRef.current = false;
      setSubmitting(false);
    }
  }, [
    holdExpired,
    countdown.expired,
    submitting,
    formData,
    eventId,
    orderId,
    holdId,
    amount,
    apiBaseUrl,
    onSuccess,
  ]);

  return {
    formData,
    setFormData,
    fieldErrors,
    submitting,
    holdExpired: holdExpired || countdown.expired,
    remainingMs: countdown.remainingMs,
    error,
    submitPayment,
  };
}
