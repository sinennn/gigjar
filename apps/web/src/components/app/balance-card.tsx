"use client";

import Link from "next/link";
import type { Jar } from "./jar-context";
import { Glass } from "@/components/ui/glass";
import { Icon } from "@/components/ui/icons";
import { addUsdt, formatUsdt, isZeroUsdt, usdtRatio } from "@/lib/money";

/**
 * The big "In your jar" card: the total, the progress toward the payout line, and what is
 * happening right now. The threshold, remaining amount and fee all come from the API.
 */
export function BalanceCard({ jar }: { jar: Jar }) {
  const total = addUsdt(jar.balances.available, jar.balances.pending);
  const progress = usdtRatio(jar.balances.available, jar.settlement_threshold);
  const settling = !isZeroUsdt(jar.balances.settling);
  const reached = isZeroUsdt(jar.remaining_until_settlement);
  const threshold = formatUsdt(jar.settlement_threshold);

  let status: React.ReactNode;
  if (settling) {
    status = (
      <>
        <strong className="font-semibold text-ink">{formatUsdt(jar.balances.settling)}</strong> is on its way to Cleva now.
      </>
    );
  } else if (reached && !jar.cleva_destination_set) {
    status = (
      <>
        Your jar is full, but it can&apos;t pay out without a Cleva address.{" "}
        <Link href="/app/wallet" className="font-semibold text-ink underline underline-offset-4">
          Add one
        </Link>
        .
      </>
    );
  } else if (reached) {
    status = "Your jar is full. The payout starts on its own shortly.";
  } else {
    status = (
      <>
        <strong className="font-semibold text-ink">{formatUsdt(jar.remaining_until_settlement)}</strong> more and your jar pays out.
      </>
    );
  }

  return (
    <Glass as="section" size="xl" aria-labelledby="jar-total" className="p-6 sm:p-8">
      <div className="flex flex-col gap-6">
        <div className="flex flex-col gap-1">
          <h2 id="jar-total" className="m-0 text-sm font-medium text-ink-soft">
            In your jar
          </h2>
          <p className="m-0 font-display text-[56px] leading-none font-bold tracking-[-0.04em] text-ink tabular-nums sm:text-[72px]">
            {formatUsdt(total)}
          </p>
          <p className="m-0 text-sm text-muted">Available plus pending, in USDT</p>
        </div>

        <div className="flex flex-col gap-2.5">
          <div
            role="progressbar"
            aria-label="Progress toward your payout"
            aria-valuemin={0}
            aria-valuemax={100}
            aria-valuenow={Math.round(progress * 100)}
            className="relative h-3 overflow-hidden rounded-full bg-ink/10"
          >
            <span
              className="absolute inset-y-0 left-0 rounded-full bg-jar-green transition-[width] duration-700"
              style={{ width: `${Math.max(progress * 100, progress > 0 ? 2 : 0)}%` }}
            />
          </div>
          <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-1 text-sm">
            <p className="m-0 text-ink-soft">{status}</p>
            <p className="m-0 inline-flex items-center gap-1.5 font-medium text-ink-soft">
              <span aria-hidden="true" className="size-2 rounded-full bg-lid-orange" />
              Pays out at {threshold}
            </p>
          </div>
        </div>
      </div>
    </Glass>
  );
}

export function StatCard({
  label,
  value,
  note,
  tone = "ink",
  icon,
}: {
  label: string;
  value: string;
  note: string;
  tone?: "ink" | "green" | "orange";
  icon?: "send";
}) {
  const color = tone === "green" ? "text-jar-green" : tone === "orange" ? "text-[#a35f12]" : "text-ink";
  return (
    <Glass size="lg" className="flex flex-col gap-1.5 p-5">
      <p className="m-0 flex items-center gap-2 text-sm font-medium text-ink-soft">
        {tone === "orange" ? <span aria-hidden="true" className="size-2 rounded-full bg-lid-orange" /> : null}
        {tone === "green" ? <span aria-hidden="true" className="size-2 rounded-full bg-jar-green" /> : null}
        {icon ? <Icon name={icon} className="size-4" /> : null}
        {label}
      </p>
      <p className={`m-0 font-display text-[32px] leading-none font-bold tracking-[-0.03em] tabular-nums ${color}`}>{value}</p>
      <p className="m-0 text-sm leading-snug text-muted">{note}</p>
    </Glass>
  );
}
