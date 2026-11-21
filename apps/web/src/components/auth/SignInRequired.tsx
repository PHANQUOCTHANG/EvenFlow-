"use client";

/** Thong bao phien dang nhap da het kem lien ket dang nhap lai (ops-warroom R-5, X-7). Dung chung i18n voi checkout. */

import Link from "next/link";

import { useT } from "@/components/i18n/locale-provider";

export function SignInRequired({ className }: { className?: string }) {
  const t = useT();
  return (
    <div
      role="alert"
      className={
        "flex flex-col items-center justify-center gap-md rounded-container border border-danger bg-danger-soft p-xl text-center " +
        (className ?? "")
      }
    >
      <p className="text-body-md text-fg">{t("checkout.authRequired")}</p>
      <Link
        href="/auth/sign-in"
        className="ef-focus-ring inline-flex items-center justify-center rounded-control border border-transparent bg-primary px-md py-sm text-label-md text-primary-fg hover:bg-primary-hover"
      >
        {t("checkout.signInAction")}
      </Link>
    </div>
  );
}
