"use client";

import { errorText } from "@/lib/api/hooks";
import { networkLabel } from "@/lib/format";
import { formatUsdt } from "@/lib/money";
import { BalanceCard } from "@/components/app/balance-card";
import { useJar } from "@/components/app/jar-context";
import { Alert } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { CopyButton } from "@/components/ui/copy-button";
import { Glass } from "@/components/ui/glass";
import { ErrorCard, LoadingCard, PageHeader } from "@/components/ui/page";

export function JarScreen() {
  const { jar, error, loading, reload } = useJar();

  return (
    <>
      <PageHeader title="Your jar" summary="Payments pool here until they are worth sending to Cleva." />

      {error && !jar ? <ErrorCard message={errorText(error)} onRetry={reload} /> : null}
      {loading && !jar && !error ? <LoadingCard lines={4} /> : null}

      {jar ? (
        <div className="flex flex-col gap-4 sm:gap-5">
          <BalanceCard jar={jar} />

          <ul className="m-0 grid list-none gap-4 p-0 sm:grid-cols-2 sm:gap-5">
            <Balance
              label="Pending"
              value={formatUsdt(jar.balances.pending)}
              tone="orange"
              body="A payment the network has seen but not confirmed yet. It is not spendable and does not count toward your payout."
            />
            <Balance
              label="Available"
              value={formatUsdt(jar.balances.available)}
              tone="green"
              body={`Confirmed and waiting in the jar. When this reaches ${formatUsdt(jar.settlement_threshold)}, your payout starts.`}
            />
            <Balance
              label="Settling"
              value={formatUsdt(jar.balances.settling)}
              tone="orange"
              body="Reserved for a payout that is on its way to Cleva. If a payout fails, this goes back to available."
            />
            <Balance
              label="Settled"
              value={formatUsdt(jar.balances.settled)}
              tone="ink"
              body={`Everything delivered to Cleva so far. GigJar keeps a flat ${formatUsdt(jar.settlement_fee)} from each payout.`}
            />
          </ul>

          <Glass as="section" size="lg" className="space-y-4 p-5 sm:p-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <h2 className="m-0 font-display text-xl font-bold tracking-[-0.02em]">Jar address</h2>
              <Badge tone="neutral">{networkLabel(jar.network)}</Badge>
            </div>
            {jar.address ? (
              <>
                <p className="m-0 rounded-2xl bg-paper/80 px-4 py-3 font-mono text-[15px] leading-relaxed break-all text-ink">
                  {jar.address}
                </p>
                <div className="flex flex-wrap gap-3">
                  <CopyButton value={jar.address} label="Copy address" />
                </div>
              </>
            ) : (
              <Alert tone="info">Your jar&apos;s address is not ready yet. Payment links will work as soon as it is.</Alert>
            )}
            <Alert tone="warning">
              <strong className="font-semibold">USDT on TRON (TRC20) only.</strong> Anything else sent to this address, on another network
              or in another coin, cannot be recovered.
            </Alert>
          </Glass>

          <Glass as="section" size="lg" className="space-y-2 p-5 sm:p-6">
            <h2 className="m-0 font-display text-xl font-bold tracking-[-0.02em]">How payouts work</h2>
            <p className="m-0 max-w-2xl text-[15px] leading-relaxed text-ink-soft">
              When your available balance reaches {formatUsdt(jar.settlement_threshold)}, GigJar sends all of it to your Cleva address in
              one go and keeps {formatUsdt(jar.settlement_fee)}. You do not need to do anything.
              {jar.cleva_destination_set ? "" : " First, add your Cleva address on the Cleva wallet screen."}
            </p>
          </Glass>
        </div>
      ) : null}
    </>
  );
}

function Balance({ label, value, body, tone }: { label: string; value: string; body: string; tone: "ink" | "green" | "orange" }) {
  return (
    <Glass as="li" size="lg" className="flex flex-col gap-2 p-5">
      <p className="m-0 flex items-center gap-2 text-sm font-medium text-ink-soft">
        {tone !== "ink" ? (
          <span aria-hidden="true" className={`size-2 rounded-full ${tone === "green" ? "bg-jar-green" : "bg-lid-orange"}`} />
        ) : null}
        {label}
      </p>
      <p
        className={`m-0 font-display text-[32px] leading-none font-bold tracking-[-0.03em] tabular-nums ${
          tone === "green" ? "text-jar-green" : "text-ink"
        }`}
      >
        {value}
      </p>
      <p className="m-0 text-sm leading-relaxed text-muted">{body}</p>
    </Glass>
  );
}
