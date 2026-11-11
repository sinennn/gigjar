import type { ButtonHTMLAttributes } from "react";

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: "primary" | "secondary";
  loading?: boolean;
};

const VARIANTS = {
  primary: "bg-ink text-cream hover:bg-ink-soft",
  secondary: "border border-line bg-paper text-ink hover:border-ink-soft",
};

export function Button({ variant = "primary", loading = false, disabled, className = "", children, ...rest }: ButtonProps) {
  return (
    <button
      {...rest}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={`w-full rounded-xl px-4 py-3 font-medium transition focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-jar-green disabled:cursor-not-allowed disabled:opacity-60 ${VARIANTS[variant]} ${className}`}
    >
      {children}
    </button>
  );
}
