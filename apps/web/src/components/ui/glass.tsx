import type { HTMLAttributes } from "react";

const SIZES = {
  pill: { radius: "rounded-full", lens: 16 },
  md:{ radius: "rounded-[20px]", lens: 18 },
  lg: { radius: "rounded-[28px]", lens: 24 },
  xl: { radius: "rounded-[32px]", lens: 26 },
};

type GlassProps = HTMLAttributes<HTMLElement> & {
  as?: "div" | "section" | "article" | "aside" | "nav" | "header" | "li" | "ul";
  size?: keyof typeof SIZES;
  /** More milk behind text: use for cards that carry lists and forms. */
  dense?: boolean;
  /** Edge refraction on wide screens. On by default; pass false for glass that repeats many times. */
  lens?: boolean;
};

/** The glass pane. Cards, panels and nav all use this, so the rim and lift stay identical. */
export function Glass({ as: Tag = "div", size = "lg", dense = true, lens = true, className = "", ...rest }: GlassProps) {
  const { radius, lens: strength } = SIZES[size];
  return (
    <Tag
      {...rest}
      data-lens={lens ? strength : undefined}
      className={`glass relative ${dense ? "glass-dense" : ""} ${radius} ${className}`}
    />
  );
}
