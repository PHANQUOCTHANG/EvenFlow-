"use client";

import { useCallback, useEffect, useState } from "react";

import { listPaymentMethods, type PaymentMethodInfo } from "@/lib/payment-client";

export type UsePaymentMethodsReturn =
  | { status: "loading"; methods: PaymentMethodInfo[]; retry: () => void }
  | { status: "ready"; methods: PaymentMethodInfo[]; retry: () => void }
  | { status: "error"; methods: PaymentMethodInfo[]; retry: () => void };

/** Cong thanh toan dang bat, lay tu GET /v1/payments/methods (AC-13). Loi -> `error` + `retry`, khong co danh sach cung. */
export function usePaymentMethods(): UsePaymentMethodsReturn {
  const [state, setState] = useState<{ status: "loading" | "ready" | "error"; methods: PaymentMethodInfo[] }>({
    status: "loading",
    methods: [],
  });
  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setState({ status: "loading", methods: [] });
    listPaymentMethods()
      .then((all) => {
        if (!cancelled) setState({ status: "ready", methods: all.filter((m) => m.enabled) });
      })
      .catch(() => {
        if (!cancelled) setState({ status: "error", methods: [] });
      });
    return () => {
      cancelled = true;
    };
  }, [attempt]);

  const retry = useCallback(() => setAttempt((n) => n + 1), []);
  return { ...state, retry };
}
