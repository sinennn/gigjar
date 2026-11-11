"use client";

import type { Schemas } from "@/lib/api/client";
import { errorText, useApi, type Page } from "@/lib/api/hooks";
import { formatUsdt } from "@/lib/money";
import { BalanceCard, StatCard } from "@/components/app/balance-card";
import { useJar, useUser } from "@/components/app/jar-context";
import { SettlementCard, TransferRow } from "@/components/app/transfer-rows";
import { ButtonLink } from "@/components/ui/button";
import { Glass } from "@/components/ui/glass";
import { EmptyState, ErrorCard, LoadingCard, PageHeader, Section } from "@/components/ui/page";

export function OverviewScreen() {
  const user = useUser();
  const { jar, error, loading, reload } = useJar();
  const transfers = useApi<Page<Schemas["IncomingTransfer"]>>("/v1/incoming-transfers?limit=5", { pollMs: 10_000 });
  const latest = useApi<Page<Schemas["Settlement"]>>("/v1/settlements?limit=1", { pollMs: 10_000 });

  const firstName = user.display_name.split(" ")[0];

  return (
    <>
      <PageHeader
        title={`Hi, ${firstName}`}
        summary="Everything your gigs have earned, at a glance."
        actions={
          <ButtonLink href="/app/links/new" size="lg">
            New payment link
          </ButtonLink>
        }
      />

      {error && !jar ? <ErrorCard message={errorText(error)} onRetry={reload} /> : null}
      {loading && !jar && !error ? <LoadingCard lines={4} /> : null}

      {jar ? (
        <div className="flex flex-col gap-4 sm:gap-5">
          <BalanceCard jar={jar} />

          <div className="grid gap-4 sm:grid-cols-3 sm:gap-5">
            <StatCard label="Available" value={formatUsdt(jar.balances.available)} note="Confirmed and waiting in your jar." tone="green" />
            <StatCard label="Pending" value={formatUsdt(jar.balances.pending)} note="Seen on the network, not confirmed yet." tone="orange" />
            <StatCard label="Sent to Cleva" value={formatUsdt(jar.balances.settled)} note="Paid out so far." icon="send" />
          </div>

          {!jar.cleva_destination_set ? (
            <Glass
              as="section"
              size="lg"
              dense={false}
              style={{ backgroundColor: "rgb(251 227 199 / 0.6)" }}
              className="flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between sm:p-6"
            >
              <div className="space-y-1">
                <h2 className="m-0 font-display text-xl font-bold tracking-[-0.02em]">Add your Cleva address</h2>
                <p className="m-0 max-w-lg text-[15px] leading-relaxed text-ink-soft">
                  Your jar pays out to Cleva once it holds {formatUsdt(jar.settlement_threshold)}. Without an address, it has nowhere to send
                  the money.
                </p>
              </div>
              <ButtonLink href="/app/wallet" variant="primary" className="flex-none">
                Add address
              </ButtonLink>
            </Glass>
          ) : null}

          <div className="grid gap-4 lg:grid-cols-[1.4fr_1fr] lg:gap-5">
            <Section
              title="Recent payments"
              action={
                <ButtonLink href="/app/transactions" variant="quiet">
                  See all
                </ButtonLink>
              }
            >
              {transfers.error && !transfers.data ? (
                <p className="m-0 px-6 pb-6 text-sm text-danger">{errorText(transfers.error)}</p>
              ) : transfers.loading ? (
                <p className="m-0 px-6 pb-6 text-sm text-muted">Loading…</p>
              ) : transfers.data && transfers.data.items.length > 0 ? (
                <ul className="m-0 list-none divide-y divide-line/70 p-0">
                  {transfers.data.items.map((transfer) => (
                    <TransferRow key={transfer.id} transfer={transfer} />
                  ))}
                </ul>
              ) : (
                <EmptyState
                  title="No payments yet"
                  action={
                    <ButtonLink href="/app/links/new" variant="secondary">
                      Create a payment link
                    </ButtonLink>
                  }
                >
                  Share a payment link with a client. Their payment shows up here the moment the network sees it.
                </EmptyState>
              )}
            </Section>

            <Section
              title="Latest payout"
              action={
                <ButtonLink href="/app/settlements" variant="quiet">
                  All payouts
                </ButtonLink>
              }
            >
              {latest.error && !latest.data ? (
                <p className="m-0 px-6 pb-6 text-sm text-danger">{errorText(latest.error)}</p>
              ) : latest.loading ? (
                <p className="m-0 px-6 pb-6 text-sm text-muted">Loading…</p>
              ) : latest.data && latest.data.items.length > 0 ? (
                <ul className="m-0 list-none p-0">
                  <SettlementCard settlement={latest.data.items[0]} />
                </ul>
              ) : (
                <EmptyState title="No payouts yet">
                  Your first payout starts when your jar reaches {formatUsdt(jar.settlement_threshold)}. GigJar keeps{" "}
                  {formatUsdt(jar.settlement_fee)} of each one.
                </EmptyState>
              )}
            </Section>
          </div>
        </div>
      ) : null}
    </>
  );
}
