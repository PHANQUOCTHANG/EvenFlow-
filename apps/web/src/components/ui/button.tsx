"use client";

import { forwardRef, type ButtonHTMLAttributes, type MouseEvent } from "react";

import { cn } from "@/lib/cn";

import { Spinner } from "./spinner";

export type ButtonVariant = "primary" | "secondary" | "tertiary" | "destructive";
export type ButtonSize = "sm" | "md" | "lg";

const VARIANT: Record<ButtonVariant, string> = {
  primary: "border border-transparent bg-primary text-primary-fg hover:bg-primary-hover",
  secondary: "border border-border-strong bg-surface text-fg hover:bg-surface-subtle",
  tertiary: "border border-transparent bg-transparent text-primary hover:bg-surface-subtle",
  destructive: "border border-transparent bg-danger text-on-semantic hover:bg-danger-hover",
};

/** Chieu cao theo DESIGN.md muc Components.1: 48px tren di dong, 44px tren desktop. */
const SIZE: Record<ButtonSize, string> = {
  sm: "h-9 px-sm text-label-md",
  md: "h-12 px-md text-label-lg md:h-11",
  lg: "h-14 px-lg text-body-lg md:h-12",
};

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  {
    variant = "primary",
    size = "md",
    loading = false,
    disabled = false,
    type = "button",
    className,
    children,
    onClick,
    ...rest
  },
  ref,
) {
  const blocked = disabled || loading;

  function handleClick(event: MouseEvent<HTMLButtonElement>) {
    // Chan tuong minh: khong chi dua vao thuoc tinh `disabled`. Mot nut dang loading
    // van la nut thuc, double-submit o day la mat tien that (AC-3).
    if (blocked) {
      event.preventDefault();
      return;
    }
    onClick?.(event);
  }

  return (
    <button
      ref={ref}
      type={type}
      disabled={blocked}
      aria-busy={loading || undefined}
      onClick={handleClick}
      className={cn(
        "ef-focus-ring relative inline-flex items-center justify-center gap-sm rounded-control",
        "transition-colors disabled:cursor-not-allowed disabled:opacity-60",
        VARIANT[variant],
        SIZE[size],
        className,
      )}
      {...rest}
    >
      {loading ? (
        <span className="absolute inset-0 flex items-center justify-center">
          {/* `decorative`: trang thai bận da duoc truyen bang aria-busy tren chinh <button>.
            * Neu spinner giu role="status" + aria-label thi accessible name cua nut bi ban
            * thanh "Dang xu ly Mua ve", va getByRole("button", { name: "Mua ve" }) se truot
            * dung luc nut vao loading. */}
          <Spinner size="sm" decorative />
        </span>
      ) : null}
      {/* opacity-0 chu khong phai `invisible`/`hidden`: giu noi dung trong cay a11y
        * va giu nguyen be rong nut nen khong bi layout shift khi loading (AC-3). */}
      <span className={loading ? "opacity-0" : undefined}>{children}</span>
    </button>
  );
});
