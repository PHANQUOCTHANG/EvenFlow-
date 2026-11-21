"use client";

import { useEffect, useId, useRef } from "react";

import { Button } from "@/components/ui";
import { useT } from "@/components/i18n/locale-provider";

export interface CancelHoldDialogProps {
  /** DELETE dang chay: nut xac nhan quay spinner, "Giu lai" va Esc bi chan (R2-4). */
  busy: boolean;
  onConfirm: () => void;
  onDismiss: () => void;
}

/** Hop thoai xac nhan "Huy giu ve" (AC-11). Chi mount khi dang mo; dong = parent unmount.
 *
 * - Nhanh that: `<dialog>.showModal()` (top layer, Esc -> su kien `cancel`).
 * - Trinh duyet/jsdom khong co `showModal`: dat thuoc tinh `open`, bat Escape o muc document (ca khi focus nam ngoai
 *   hop thoai - X-5; van bi chan khi dang huy) va bay focus Tab trong cac nut cua hop thoai (R2-2, X-5).
 * - Focus ban dau vao "Giu lai" (lua chon an toan). Tra focus do parent lo (trigger co the da bi go - R2-3). */
export function CancelHoldDialog({ busy, onConfirm, onDismiss }: CancelHoldDialogProps) {
  const t = useT();
  const titleId = useId();
  const bodyId = useId();
  const ref = useRef<HTMLDialogElement>(null);
  const keepRef = useRef<HTMLButtonElement>(null);
  const busyRef = useRef(busy);
  busyRef.current = busy;
  const onDismissRef = useRef(onDismiss);
  onDismissRef.current = onDismiss;

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const dismiss = () => {
      if (!busyRef.current) onDismissRef.current();
    };
    const onCancel = (e: Event) => {
      // Khong de trinh duyet tu dong: trang thai React quyet dinh (va chan khi dang huy).
      e.preventDefault();
      dismiss();
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key !== "Escape") return;
      e.preventDefault();
      dismiss();
    };
    // Nhanh du phong: Escape + bay focus Tab o muc document (focus co the da roi ra ngoai hop thoai).
    const onDocKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        onKeyDown(e);
        return;
      }
      if (e.key !== "Tab") return;
      const items = Array.from(el.querySelectorAll<HTMLButtonElement>("button")).filter((b) => !b.disabled);
      if (items.length === 0) {
        e.preventDefault();
        return;
      }
      const first = items[0];
      const last = items[items.length - 1];
      const active = document.activeElement;
      const inside = active instanceof Node && el.contains(active);
      if (e.shiftKey && (!inside || active === first)) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && (!inside || active === last)) {
        e.preventDefault();
        first.focus();
      }
    };
    // Dong boi trinh duyet (khong qua React) -> dong bo trang thai.
    const onClose = () => dismiss();

    const native = typeof el.showModal === "function";
    if (native) {
      if (!el.open) el.showModal();
      el.addEventListener("keydown", onKeyDown);
    } else {
      el.setAttribute("open", "");
      document.addEventListener("keydown", onDocKeyDown);
    }
    el.addEventListener("cancel", onCancel);
    el.addEventListener("close", onClose);
    keepRef.current?.focus();

    return () => {
      el.removeEventListener("cancel", onCancel);
      el.removeEventListener("keydown", onKeyDown);
      document.removeEventListener("keydown", onDocKeyDown);
      el.removeEventListener("close", onClose);
      if (typeof el.close === "function") {
        if (el.open) el.close();
      } else {
        el.removeAttribute("open");
      }
    };
  }, []);

  return (
    <dialog
      ref={ref}
      role="alertdialog"
      aria-modal="true"
      aria-labelledby={titleId}
      aria-describedby={bodyId}
      className="m-auto w-[min(28rem,calc(100%-2rem))] rounded-card border border-border bg-surface p-lg text-fg shadow-float backdrop:bg-black/40"
    >
      <h2 id={titleId} className="text-headline-sm text-fg">
        {t("checkout.cancelHoldConfirmTitle")}
      </h2>
      <p id={bodyId} className="mt-sm text-body-md text-fg-muted">
        {t("checkout.cancelHoldConfirmBody")}
      </p>
      <div className="mt-lg flex flex-col-reverse gap-sm sm:flex-row sm:justify-end">
        <Button ref={keepRef} variant="secondary" disabled={busy} onClick={() => onDismissRef.current()}>
          {t("checkout.cancelHoldKeep")}
        </Button>
        <Button variant="destructive" loading={busy} onClick={onConfirm}>
          {t("checkout.cancelHoldConfirm")}
        </Button>
      </div>
    </dialog>
  );
}
