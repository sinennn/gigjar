"use client";

import { useEffect, useState, type ReactNode } from "react";
import { Alert } from "./alert";
import { Button } from "./button";
import { Glass } from "./glass";

export function PageHeader({
  title,
  summary,
  actions,
  back,
}: {
  title: string;
  summary?: string;
  actions?: ReactNode;
  back?: ReactNode;
}) {
  return (
    <header className="mb-6 flex flex-col gap-4 sm:mb-8 sm:flex-row sm:items-end sm:justify-between">
      <div className="min-w-0 space-y-2">
        {back}
        <h1 className="m-0 font-display text-[32px] leading-[1.05] font-bold tracking-[-0.03em] break-words text-ink sm:text-[40px]">
          {title}
        </h1>
        {summary ? <p className="m-0 max-w-xl text-base leading-relaxed text-ink-soft">{summary}</p> : null}
      </div>
      {actions ? <div className="flex flex-none flex-wrap gap-3">{actions}</div> : null}
    </header>
  );
}

/** Placeholder card while data loads. After a few seconds it explains why it is slow. */
export function LoadingCard({ lines = 3 }: { lines?: number }) {
  const [slow, setSlow] = useState(false);
  useEffect(() => {
    const timer = setTimeout(() => setSlow(true), 5000);
    return () => clearTimeout(timer);
  }, []);
  return (
    <Glass size="lg" lens={false} role="status" aria-label="Loading" className="space-y-3 p-6">
      {Array.from({ length: lines }, (_, i) => (
        <div
          key={i}
          className="h-4 rounded-full bg-ink/8 motion-safe:animate-pulse"
          style={{ width: `${88 - i * 14}%` }}
        />
      ))}
      {slow ? (
        <p className="m-0 pt-1 text-sm text-muted">Still loading. The server may be waking up, which can take a moment.</p>
      ) : null}
    </Glass>
  );
}

export function ErrorCard({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <Glass size="lg" lens={false} className="space-y-4 p-6">
      <Alert>{message}</Alert>
      {onRetry ? (
        <Button variant="secondary" onClick={onRetry}>
          Try again
        </Button>
      ) : null}
    </Glass>
  );
}

export function EmptyState({ title, children, action }: { title: string; children?: ReactNode; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-start gap-3 px-6 py-10 sm:items-center sm:text-center">
      <p className="m-0 font-display text-xl font-bold tracking-[-0.02em] text-ink">{title}</p>
      {children ? <p className="m-0 max-w-sm text-[15px] leading-relaxed text-ink-soft">{children}</p> : null}
      {action}
    </div>
  );
}

/** Card with a heading row and a body, used by every list on the app screens. */
export function Section({
  title,
  action,
  children,
  className = "",
}: {
  title: string;
  action?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <Glass as="section" size="lg" className={`overflow-hidden ${className}`}>
      <div className="flex items-center justify-between gap-3 px-5 pt-5 pb-3 sm:px-6">
        <h2 className="m-0 font-display text-xl font-bold tracking-[-0.02em] text-ink">{title}</h2>
        {action}
      </div>
      {children}
    </Glass>
  );
}
