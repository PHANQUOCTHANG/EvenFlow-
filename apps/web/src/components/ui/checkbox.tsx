"use client";

import { forwardRef, useEffect, useRef, type InputHTMLAttributes } from "react";

import { cn } from "@/lib/cn";

import { FieldError, FieldHelp, describedBy, useFieldIds } from "./field";

export interface CheckboxProps extends Omit<InputHTMLAttributes<HTMLInputElement>, "type"> {
  label: string;
  /** `indeterminate` khong ton tai duoi dang attribute HTML — phai set qua DOM property. */
  indeterminate?: boolean;
  error?: string;
  helpText?: string;
}

export const Checkbox = forwardRef<HTMLInputElement, CheckboxProps>(function Checkbox(
  { label, indeterminate = false, error, helpText, id, className, ...rest },
  ref,
) {
  const ids = useFieldIds(id);
  const inner = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (inner.current) {
      inner.current.indeterminate = indeterminate;
    }
  }, [indeterminate]);

  return (
    <div className="flex flex-col gap-xs">
      <div className="flex items-center gap-sm">
        <input
          ref={(node) => {
            inner.current = node;
            if (typeof ref === "function") {
              ref(node);
            } else if (ref) {
              ref.current = node;
            }
          }}
          id={ids.inputId}
          type="checkbox"
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy(ids, {
            hasHelp: Boolean(helpText),
            hasError: Boolean(error),
          })}
          className={cn(
            "ef-focus-ring size-5 rounded-sm border accent-primary",
            "disabled:cursor-not-allowed disabled:opacity-60",
            error ? "border-danger" : "border-border-strong",
            className,
          )}
          {...rest}
        />
        <label htmlFor={ids.inputId} className="text-body-md text-fg">
          {label}
        </label>
      </div>
      {error ? <FieldError id={ids.errorId}>{error}</FieldError> : null}
      {helpText ? <FieldHelp id={ids.helpId}>{helpText}</FieldHelp> : null}
    </div>
  );
});
