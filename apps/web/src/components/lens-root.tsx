"use client";

import { useEffect, useRef, type ReactNode } from "react";
import { liquidLens, supportsLiquidLens } from "@/lib/liquid-glass";

/**
 * Wraps a page and turns on edge refraction for every `[data-lens]` inside it, including cards
 * that mount later once their data arrives. Same rule as the landing: Chromium on a wide screen
 * with a mouse only; everywhere else the plain frosted glass stays. Nothing here follows the pointer.
 */
export function LensRoot({ children, className }: { children: ReactNode; className?: string }) {
  const root = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = root.current;
    if (!el || !supportsLiquidLens()) {
      return;
    }
    const wide = window.matchMedia("(min-width: 768px) and (pointer: fine)");
    const active = new Map<HTMLElement, () => void>();

    const clear = () => {
      active.forEach((cleanup) => cleanup());
      active.clear();
    };
    const apply = () => {
      active.forEach((cleanup, node) => {
        if (!node.isConnected) {
          cleanup();
          active.delete(node);
        }
      });
      el.querySelectorAll<HTMLElement>("[data-lens]").forEach((node) => {
        if (!active.has(node)) {
          active.set(node, liquidLens(node, { strength: Number(node.dataset.lens) || 28 }));
        }
      });
    };
    const sync = () => (wide.matches ? apply() : clear());

    sync();
    wide.addEventListener("change", sync);
    const observer = new MutationObserver(() => {
      if (wide.matches) apply();
    });
    observer.observe(el, { childList: true, subtree: true });
    return () => {
      observer.disconnect();
      wide.removeEventListener("change", sync);
      clear();
    };
  }, []);

  return (
    <div ref={root} className={className}>
      {children}
    </div>
  );
}
