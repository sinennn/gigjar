"use client";

import { useSyncExternalStore } from "react";
import { buttonClass } from "./button";
import { Icon } from "./icons";

const never = () => () => {};

/**
 * Opens the phone's own share sheet (WhatsApp, Messages, and so on) when the browser has one.
 * On desktop browsers without it, nothing is rendered and the copy button does the job.
 */
export function ShareButton({ title, text, url, label = "Share" }: { title: string; text: string; url: string; label?: string }) {
  const canShare = useSyncExternalStore(
    never,
    () => typeof navigator !== "undefined" && typeof navigator.share === "function",
    () => false,
  );
  if (!canShare) {
    return null;
  }
  async function share() {
    try {
      await navigator.share({ title, text, url });
    } catch {
      // Closing the share sheet rejects with AbortError. That is not a failure worth showing.
    }
  }
  return (
    <button type="button" onClick={share} className={buttonClass("secondary", "md")}>
      <Icon name="share" className="size-4" />
      {label}
    </button>
  );
}
