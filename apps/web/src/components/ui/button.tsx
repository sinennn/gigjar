import Link from "next/link";
import type { ButtonHTMLAttributes, ComponentProps } from "react";

type Variant = "primary" | "secondary" | "quiet" | "danger";
type Size = "md" | "lg";

const BASE =
  "relative inline-flex items-center justify-center gap-2 rounded-full font-semibold no-underline transition-colors disabled:cursor-not-allowed disabled:opacity-60";

const VARIANTS: Record<Variant, string> = {
  primary: "bg-ink text-paper shadow-[0_10px_24px_-10px_rgb(19_38_58/0.6)] hover:bg-ink-soft",
  secondary: "glass text-ink hover:text-ink-soft",
  quiet: "text-ink-soft hover:bg-ink/5 hover:text-ink",
  danger: "glass text-danger hover:bg-danger-soft/60",
};

const SIZES: Record<Size, string> = {
  md: "min-h-11 px-5 text-[15px]",
  lg: "min-h-[52px] px-7 text-base",
};

export function buttonClass(variant: Variant = "primary", size: Size = "md", className = ""): string {
  return `${BASE} ${VARIANTS[variant]} ${SIZES[size]} ${className}`;
}

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  size?: Size;
  loading?: boolean;
};

export function Button({ variant = "primary", size = "md", loading = false, disabled, className = "", children, ...rest }: ButtonProps) {
  return (
    <button
      type="button"
      {...rest}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={buttonClass(variant, size, className)}
    >
      {loading ? (
        <span aria-hidden="true" className="size-4 rounded-full border-2 border-current border-t-transparent motion-safe:animate-spin" />
      ) : null}
      {children}
    </button>
  );
}

type ButtonLinkProps = ComponentProps<typeof Link> & { variant?: Variant; size?: Size };

export function ButtonLink({ variant = "primary", size = "md", className = "", ...rest }: ButtonLinkProps) {
  return <Link {...rest} className={buttonClass(variant, size, className)} />;
}
