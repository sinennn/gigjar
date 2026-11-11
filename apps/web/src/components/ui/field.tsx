import type { InputHTMLAttributes } from "react";

type FieldProps = InputHTMLAttributes<HTMLInputElement> & {
  label: string;
  /** Message from the API's `error.fields`, shown under the input. */
  error?: string;
  hint?: string;
};

export function Field({ label, error, hint, id, className = "", ...rest }: FieldProps) {
  const inputId = id ?? rest.name;
  const describedBy = [error ? `${inputId}-error` : null, hint ? `${inputId}-hint` : null].filter(Boolean).join(" ");
  return (
    <div className="space-y-1.5">
      <label htmlFor={inputId} className="block text-sm font-medium text-ink-soft">
        {label}
      </label>
      <input
        {...rest}
        id={inputId}
        aria-invalid={error ? true : undefined}
        aria-describedby={describedBy || undefined}
        className={`w-full rounded-xl border bg-paper px-4 py-3 text-ink placeholder:text-muted focus:border-jar-green focus:outline-2 focus:outline-jar-green/30 ${
          error ? "border-danger" : "border-line"
        } ${className}`}
      />
      {hint && !error ? (
        <p id={`${inputId}-hint`} className="text-xs text-muted">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={`${inputId}-error`} className="text-sm text-danger">
          {error}
        </p>
      ) : null}
    </div>
  );
}
