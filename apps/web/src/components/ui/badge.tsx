import type { ReactNode } from "react";

const TONES = {
  green: "bg-jar-green-soft text-[#0f5f48]",
  orange: "bg-lid-orange-soft text-[#7a4510]",
  danger: "bg-danger-soft text-danger",
  neutral: "bg-ink/8 text-ink-soft",
};

const DOTS = {
  green: "bg-jar-green",
  orange: "bg-lid-orange",
  danger: "bg-danger",
  neutral: "bg-muted",
};

export type BadgeTone = keyof typeof TONES;

export function Badge({ tone = "neutral", children }: { tone?: BadgeTone; children: ReactNode }) {
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold whitespace-nowrap ${TONES[tone]}`}>
      <span aria-hidden="true" className={`size-1.5 rounded-full ${DOTS[tone]}`} />
      {children}
    </span>
  );
}

/** Shown on every screen whenever the API reports `network: "simulated"`. No real funds move. */
export function SimulatedBadge({ className = "" }: { className?: string }) {
  return (
    <span
      title="No real money moves on the simulated network"
      className={`inline-flex items-center gap-1.5 rounded-full bg-lid-orange px-3 py-1 text-xs font-bold tracking-wide text-ink uppercase ${className}`}
    >
      <span aria-hidden="true" className="size-1.5 rounded-full bg-ink" />
      Simulated
    </span>
  );
}

const STATUS_TONES: Record<string, { tone: BadgeTone; label: string }> = {
  open: { tone: "neutral", label: "Open" },
  paid: { tone: "green", label: "Paid" },
  closed: { tone: "neutral", label: "Closed" },
  pending: { tone: "orange", label: "Pending" },
  confirmed: { tone: "green", label: "Confirmed" },
  failed: { tone: "danger", label: "Failed" },
  queued: { tone: "orange", label: "Queued" },
  signing: { tone: "orange", label: "Signing" },
  broadcast: { tone: "orange", label: "On its way" },
};

/** One badge for every status the API returns: payment links, transfers and settlements. */
export function StatusBadge({ status, kind }: { status: string; kind?: "settlement" }) {
  if (kind === "settlement" && status === "confirmed") {
    return <Badge tone="green">Sent</Badge>;
  }
  const entry = STATUS_TONES[status] ?? { tone: "neutral" as const, label: status };
  return <Badge tone={entry.tone}>{entry.label}</Badge>;
}
