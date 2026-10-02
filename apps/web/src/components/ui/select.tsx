import { forwardRef, type SelectHTMLAttributes } from "react";

import { cn } from "@/lib/cn";

import { FieldShell, describedBy, useFieldIds } from "./field";

export interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

export interface SelectProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label: string;
  options: SelectOption[];
  /** Hien thi nhu mot option bi disable — KHONG phai de thay cho label. */
  placeholder?: string;
  error?: string;
  helpText?: string;
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select(
  { label, options, placeholder, error, helpText, id, className, required, ...rest },
  ref,
) {
  const ids = useFieldIds(id);

  return (
    <FieldShell ids={ids} label={label} required={required} error={error} helpText={helpText}>
      <select
        ref={ref}
        id={ids.inputId}
        required={required}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy(ids, { hasHelp: Boolean(helpText), hasError: Boolean(error) })}
        className={cn(
          "ef-focus-ring h-11 w-full rounded-control border bg-surface px-sm text-body-lg text-fg",
          "disabled:cursor-not-allowed disabled:opacity-60",
          error ? "border-danger" : "border-border-strong",
          className,
        )}
        {...rest}
      >
        {placeholder ? (
          <option value="" disabled>
            {placeholder}
          </option>
        ) : null}
        {options.map((option) => (
          <option key={option.value} value={option.value} disabled={option.disabled}>
            {option.label}
          </option>
        ))}
      </select>
    </FieldShell>
  );
});
