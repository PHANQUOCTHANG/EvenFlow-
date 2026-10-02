"use client";

import { forwardRef, useState, type InputHTMLAttributes } from "react";

import { cn } from "@/lib/cn";

import { FieldShell, describedBy, useFieldIds } from "./field";

export type InputType = "text" | "email" | "password" | "otp";

export interface InputProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "type"> {
  /** Bat buoc. Handoff muc 6: KHONG duoc dung placeholder thay cho label. */
  label: string;
  type?: InputType;
  error?: string;
  helpText?: string;
  /** Chi ap dung khi type="otp". */
  otpLength?: number;
}

const CONTROL =
  "ef-focus-ring h-11 w-full rounded-control border bg-surface px-sm text-body-lg text-fg " +
  "placeholder:text-fg-muted disabled:cursor-not-allowed disabled:opacity-60";

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { label, type = "text", error, helpText, otpLength = 6, id, className, required, ...rest },
  ref,
) {
  const ids = useFieldIds(id);
  const [revealed, setRevealed] = useState(false);

  const isOtp = type === "otp";
  const isPassword = type === "password";
  const resolvedType = isOtp ? "text" : isPassword && revealed ? "text" : type;

  const control = (
    <input
      ref={ref}
      id={ids.inputId}
      type={resolvedType}
      required={required}
      aria-invalid={error ? true : undefined}
      aria-describedby={describedBy(ids, { hasHelp: Boolean(helpText), hasError: Boolean(error) })}
      className={cn(
        CONTROL,
        error ? "border-danger" : "border-border-strong",
        // OTP va moi truong so luong: tabular numbers de chu so khong nhay be rong.
        isOtp && "tabular-nums tracking-[0.3em] text-center",
        isPassword && "pr-20",
        className,
      )}
      {...(isOtp
        ? { inputMode: "numeric" as const, autoComplete: "one-time-code", maxLength: otpLength }
        : {})}
      {...rest}
    />
  );

  return (
    <FieldShell ids={ids} label={label} required={required} error={error} helpText={helpText}>
      {isPassword ? (
        <div className="relative">
          {control}
          <button
            type="button"
            onClick={() => setRevealed((v) => !v)}
            aria-label={revealed ? "Ẩn mật khẩu" : "Hiện mật khẩu"}
            aria-pressed={revealed}
            className="ef-focus-ring absolute inset-y-0 right-0 rounded-control px-sm text-label-md text-primary"
          >
            {revealed ? "Ẩn" : "Hiện"}
          </button>
        </div>
      ) : (
        control
      )}
    </FieldShell>
  );
});
