"use client";

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";

import { Alert, type AlertVariant } from "./alert";

export interface ToastOptions {
  variant?: AlertVariant;
  title: string;
  description?: string;
  /** ms. <= 0 thi khong tu dong an. */
  duration?: number;
}

interface ToastItem extends ToastOptions {
  id: string;
  variant: AlertVariant;
}

interface ToastApi {
  toast: (options: ToastOptions) => string;
  dismiss: (id: string) => void;
}

const DEFAULT_DURATION = 5000;

const ToastContext = createContext<ToastApi | null>(null);

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);
  const timers = useRef(new Map<string, ReturnType<typeof setTimeout>>());
  const counter = useRef(0);

  const dismiss = useCallback((id: string) => {
    setItems((prev) => prev.filter((item) => item.id !== id));
    const timer = timers.current.get(id);
    if (timer) {
      clearTimeout(timer);
      timers.current.delete(id);
    }
  }, []);

  const toast = useCallback(
    (options: ToastOptions) => {
      counter.current += 1;
      const id = `ef-toast-${counter.current}`;
      // Noi vao cuoi danh sach: toast moi KHONG ghi de toast dang hien (AC-6).
      setItems((prev) => [...prev, { ...options, id, variant: options.variant ?? "info" }]);

      const duration = options.duration ?? DEFAULT_DURATION;
      if (duration > 0) {
        timers.current.set(
          id,
          setTimeout(() => dismiss(id), duration),
        );
      }
      return id;
    },
    [dismiss],
  );

  // Don timer khi unmount, neu khong se setState tren component da thao.
  useEffect(() => {
    const pending = timers.current;
    return () => {
      pending.forEach((timer) => clearTimeout(timer));
      pending.clear();
    };
  }, []);

  const api = useMemo<ToastApi>(() => ({ toast, dismiss }), [toast, dismiss]);

  return (
    <ToastContext.Provider value={api}>
      {children}
      <div
        aria-live="polite"
        data-slot="toast-region"
        className="pointer-events-none fixed top-6 right-6 z-50 flex flex-col items-end gap-sm"
      >
        {items.map((item) => (
          <Alert
            key={item.id}
            variant={item.variant}
            title={item.title}
            onDismiss={() => dismiss(item.id)}
            className="pointer-events-auto w-[380px] max-w-[90vw] shadow-float ef-toast-enter"
          >
            {item.description}
          </Alert>
        ))}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast(): ToastApi {
  const context = useContext(ToastContext);
  if (!context) {
    throw new Error("useToast phải dùng trong ToastProvider");
  }
  return context;
}
