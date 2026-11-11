import type { InputHTMLAttributes, ReactNode, TextareaHTMLAttributes } from "react";

const CONTROL =
  "block w-full rounded-2xl border bg-paper/85 px-4 text-[16px] text-ink placeholder:text-muted focus:border-ink focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ink disabled:opacity-60";

type Shared = {
  label: string;
  /** Message from the API's `error.fields`, shown under the control. */
  error?: string;
  hint?: string;
};

function Wrapper({ id, label, error, hint, children }: Shared & { id: string; children: ReactNode }) {
  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="block text-sm font-medium text-ink-soft">
        {label}
      </label>
      {children}
      {hint && !error ? (
        <p id={`${id}-hint`} className="m-0 text-sm text-muted">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={`${id}-error`} className="m-0 text-sm text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}

function describedBy(id: string, error?: string, hint?: string) {
  return [error ? `${id}-error` : null, hint && !error ? `${id}-hint` : null].filter(Boolean).join(" ") || undefined;
}

type FieldProps = InputHTMLAttributes<HTMLInputElement> &
  Shared & {
    /** Short unit shown inside the right edge, such as "USDT". */
    suffix?: string;
  };

export function Field({ label, error, hint, suffix, id, className = "", ...rest }: FieldProps) {
  const inputId = id ?? rest.name ?? label;
  return (
    <Wrapper id={inputId} label={label} error={error} hint={hint}>
      <div className="relative">
        <input
          {...rest}
          id={inputId}
          aria-invalid={error ? true : undefined}
          aria-describedby={describedBy(inputId, error, hint)}
          className={`${CONTROL} min-h-12 py-2.5 ${suffix ? "pr-16" : ""} ${error ? "border-danger" : "border-line"} ${className}`}
        />
        {suffix ? (
          <span className="pointer-events-none absolute inset-y-0 right-4 flex items-center text-sm font-medium text-muted">
            {suffix}
          </span>
        ) : null}
      </div>
    </Wrapper>
  );
}

type TextAreaProps = TextareaHTMLAttributes<HTMLTextAreaElement> & Shared;

export function TextArea({ label, error, hint, id, className = "", ...rest }: TextAreaProps) {
  const inputId = id ?? rest.name ?? label;
  return (
    <Wrapper id={inputId} label={label} error={error} hint={hint}>
      <textarea
        {...rest}
        id={inputId}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy(inputId, error, hint)}
        className={`${CONTROL} min-h-28 resize-y py-3 ${error ? "border-danger" : "border-line"} ${className}`}
      />
    </Wrapper>
  );
}
