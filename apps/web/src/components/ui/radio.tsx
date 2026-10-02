"use client";

import { useId } from "react";

import { cn } from "@/lib/cn";

import { FieldError, FieldHelp } from "./field";

export interface RadioOption {
  value: string;
  label: string;
  disabled?: boolean;
}

export interface RadioGroupProps {
  name: string;
  legend: string;
  options: RadioOption[];
  value?: string;
  onValueChange?: (value: string) => void;
  error?: string;
  helpText?: string;
  className?: string;
}

export function RadioGroup({
  name,
  legend,
  options,
  value,
  onValueChange,
  error,
  helpText,
  className,
}: RadioGroupProps) {
  const base = useId();
  const legendId = `${base}-legend`;
  const errorId = `${base}-error`;
  const helpId = `${base}-help`;

  const described = [error ? errorId : null, helpText ? helpId : null].filter(Boolean).join(" ");

  return (
    <fieldset
      role="radiogroup"
      aria-labelledby={legendId}
      aria-invalid={error ? true : undefined}
      aria-describedby={described || undefined}
      className={cn("flex flex-col gap-xs border-0 p-0", className)}
    >
      <legend id={legendId} className="text-label-lg text-fg">
        {legend}
      </legend>
      {options.map((option) => {
        const optionId = `${base}-${option.value}`;
        return (
          <div key={option.value} className="flex items-center gap-sm">
            <input
              id={optionId}
              type="radio"
              name={name}
              value={option.value}
              disabled={option.disabled}
              checked={value === undefined ? undefined : value === option.value}
              onChange={() => onValueChange?.(option.value)}
              className={cn(
                "ef-focus-ring size-5 border accent-primary",
                "disabled:cursor-not-allowed disabled:opacity-60",
                error ? "border-danger" : "border-border-strong",
              )}
            />
            <label htmlFor={optionId} className="text-body-md text-fg">
              {option.label}
            </label>
          </div>
        );
      })}
      {error ? <FieldError id={errorId}>{error}</FieldError> : null}
      {helpText ? <FieldHelp id={helpId}>{helpText}</FieldHelp> : null}
    </fieldset>
  );
}
