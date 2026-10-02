import { useId, type ReactNode } from "react";

import { cn } from "@/lib/cn";

export interface FieldIds {
  inputId: string;
  helpId: string;
  errorId: string;
}

/** Id dung chung cho label / help / error cua mot field.
 *  Luon goi useId (hook khong duoc goi co dieu kien), chi uu tien id do caller truyen. */
export function useFieldIds(providedId?: string): FieldIds {
  const auto = useId();
  const base = providedId ?? auto;
  return { inputId: base, helpId: `${base}-help`, errorId: `${base}-error` };
}

/** Thu tu error truoc help: screen reader doc loi truoc huong dan. */
export function describedBy(
  ids: FieldIds,
  opts: { hasHelp: boolean; hasError: boolean },
): string | undefined {
  const list = [opts.hasError ? ids.errorId : null, opts.hasHelp ? ids.helpId : null].filter(Boolean);
  return list.length > 0 ? list.join(" ") : undefined;
}

export function FieldLabel({
  htmlFor,
  children,
  required,
  className,
}: {
  htmlFor: string;
  children: ReactNode;
  required?: boolean;
  className?: string;
}) {
  // Dau * nam NGOAI <label>. Neu de ben trong thi textContent cua label thanh
  // "Ten field *" — lam sai ten hien thi cua field va lam vo moi truy van theo label.
  return (
    <span className={cn("flex items-baseline gap-xs text-label-lg text-fg", className)}>
      <label htmlFor={htmlFor}>{children}</label>
      {required ? (
        <span className="text-danger" aria-hidden="true">
          *
        </span>
      ) : null}
    </span>
  );
}

export function FieldHelp({ id, children }: { id: string; children: ReactNode }) {
  return (
    <p id={id} className="text-body-sm text-fg-muted">
      {children}
    </p>
  );
}

/** role="alert" de loi validation duoc thong bao ngay, khong cho tuong tac tiep. */
export function FieldError({ id, children }: { id: string; children: ReactNode }) {
  return (
    <p id={id} role="alert" className="text-body-sm text-danger">
      {children}
    </p>
  );
}

/** Khung dung chung: label tren, control giua, help/error duoi. */
export function FieldShell({
  ids,
  label,
  required,
  error,
  helpText,
  className,
  children,
}: {
  ids: FieldIds;
  label: string;
  required?: boolean;
  error?: string;
  helpText?: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <div className={cn("flex flex-col gap-xs", className)}>
      <FieldLabel htmlFor={ids.inputId} required={required}>
        {label}
      </FieldLabel>
      {children}
      {error ? <FieldError id={ids.errorId}>{error}</FieldError> : null}
      {helpText ? <FieldHelp id={ids.helpId}>{helpText}</FieldHelp> : null}
    </div>
  );
}
