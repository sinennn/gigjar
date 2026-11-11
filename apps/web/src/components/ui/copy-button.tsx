"use client";

import { useEffect, useRef, useState } from "react";
import { buttonClass } from "./button";
import { Icon } from "./icons";

/** Copies text to the clipboard and says so. Falls back to a hidden textarea on plain http. */
export function CopyButton({
  value,
  label = "Copy",
  copiedLabel = "Copied",
  variant = "secondary",
  className = "",
}: {
  value: string;
  label?: string;
  copiedLabel?: string;
  variant?: "secondary" | "quiet";
  className?: string;
}) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  useEffect(() => () => clearTimeout(timer.current), []);

  async function copy() {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      const area = document.createElement("textarea");
      area.value = value;
      area.setAttribute("readonly", "");
      area.style.position = "fixed";
      area.style.opacity = "0";
      document.body.append(area);
      area.select();
      document.execCommand("copy");
      area.remove();
    }
    setCopied(true);
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setCopied(false), 2000);
  }

  return (
    <button type="button" onClick={copy} className={buttonClass(variant, "md", className)}>
      <Icon name={copied ? "check" : "copy"} className="size-4" />
      <span aria-live="polite">{copied ? copiedLabel : label}</span>
    </button>
  );
}
