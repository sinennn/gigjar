"use client";

import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { errorText, useCursorList } from "@/lib/api/hooks";
import { formatWhen } from "@/lib/format";
import { formatUsdt, formatUsdtSigned, isZeroUsdt } from "@/lib/money";
import { TransferRow } from "@/components/app/transfer-rows";
import { Button } from "@/components/ui/button";
import { Glass } from "@/components/ui/glass";
import { EmptyState, ErrorCard, LoadingCard, PageHeader } from "@/components/ui/page";

type Movement = Schemas["LedgerMovement"];
type Tab = "activity" | "incoming";

const TABS: { value: Tab; label: string }[] = [
  { value: "activity", label: "Jar activity" },
  { value: "incoming", label: "Incoming payments" },
];

export function TransactionsScreen() {
  const [tab, setTab] = useState<Tab>("activity");
  return (
    <>
      <PageHeader title="Transactions" summary="Every movement in and out of your jar." />
      <div role="group" aria-label="Choose a list" className="mb-4 flex flex-wrap gap-2">
        {TABS.map((item) => (
          <button
            key={item.value}
            type="button"
            aria-pressed={tab === item.value}
            onClick={() => setTab(item.value)}
            className={`relative inline-flex min-h-11 items-center rounded-full px-5 text-[15px] font-semibold transition-colors ${
              tab === item.value ? "bg-ink text-paper" : "glass text-ink-soft hover:text-ink"
            }`}
          >
            {item.label}
          </button>
        ))}
      </div>
      {tab === "activity" ? <Activity /> : <Incoming />}
    </>
  );
}

function Activity() {
  const list = useCursorList<Movement>("/v1/transactions?limit=25");
  return (
    <ListShell
      list={list}
      empty={{ title: "No activity yet", body: "Payments and payouts appear here as they happen." }}
      label="Jar activity"
    >
      {list.items.map((movement) => (
        <MovementRow key={movement.id} movement={movement} />
      ))}
    </ListShell>
  );
}

function Incoming() {
  const list = useCursorList<Schemas["IncomingTransfer"]>("/v1/incoming-transfers?limit=25");
  return (
    <ListShell
      list={list}
      empty={{ title: "No incoming payments yet", body: "Every payment the network sees for your jar shows here, even before it is confirmed." }}
      label="Incoming payments"
    >
      {list.items.map((transfer) => (
        <TransferRow key={transfer.id} transfer={transfer} showLink />
      ))}
    </ListShell>
  );
}

type ListLike = {
  items: unknown[];
  error: Error | null;
  loading: boolean;
  hasMore: boolean;
  loadingMore: boolean;
  loadMore: () => void;
  reload: () => void;
};

function ListShell({
  list,
  empty,
  label,
  children,
}: {
  list: ListLike;
  empty: { title: string; body: string };
  label: string;
  children: React.ReactNode;
}) {
  if (list.error && list.items.length === 0) {
    return <ErrorCard message={errorText(list.error)} onRetry={list.reload} />;
  }
  if (list.loading && list.items.length === 0) {
    return <LoadingCard lines={5} />;
  }
  if (list.items.length === 0) {
    return (
      <Glass size="lg" className="overflow-hidden">
        <EmptyState title={empty.title}>{empty.body}</EmptyState>
      </Glass>
    );
  }
  return (
    <Glass as="section" size="lg" aria-label={label} className="overflow-hidden">
      <ul className="m-0 list-none divide-y divide-line/70 p-0">{children}</ul>
      {list.error ? <p className="m-0 px-6 py-3 text-sm text-danger">{errorText(list.error)}</p> : null}
      {list.hasMore ? (
        <div className="border-t border-line/70 p-4 text-center">
          <Button variant="secondary" loading={list.loadingMore} onClick={list.loadMore}>
            Load more
          </Button>
        </div>
      ) : null}
    </Glass>
  );
}

const KINDS: Record<Movement["kind"], { title: string; note: string }> = {
  deposit: { title: "Payment received", note: "A confirmed payment landed in your jar." },
  settlement_reserved: { title: "Payout started", note: "Money set aside to send to Cleva." },
  settlement_completed: { title: "Payout sent", note: "The payout reached your Cleva address." },
  settlement_released: { title: "Payout returned to jar", note: "The payout did not go through, so the money is back in your jar." },
};

function MovementRow({ movement }: { movement: Movement }) {
  const kind = KINDS[movement.kind];
  const deltas = [
    { label: "Available", value: movement.available_delta },
    { label: "Settling", value: movement.settling_delta },
    { label: "Settled", value: movement.settled_delta },
  ].filter((delta) => !isZeroUsdt(delta.value));

  return (
    <li className="flex flex-col gap-2 px-5 py-4 sm:flex-row sm:items-start sm:justify-between sm:gap-6 sm:px-6">
      <div className="min-w-0">
        <p className="m-0 text-[15px] font-semibold text-ink">{kind.title}</p>
        <p className="m-0 text-sm text-ink-soft">{kind.note}</p>
        <p className="m-0 mt-1 text-sm text-muted">{formatWhen(movement.created_at)}</p>
      </div>
      <ul className="m-0 flex list-none flex-wrap gap-2 p-0 sm:flex-none sm:justify-end">
        {deltas.map((delta) => {
          const gain = !delta.value.startsWith("-");
          return (
            <li
              key={delta.label}
              className="inline-flex items-baseline gap-1.5 rounded-full bg-ink/5 px-3 py-1 text-sm tabular-nums"
            >
              <span className="text-muted">{delta.label}</span>
              <span className={`font-semibold ${gain ? "text-jar-green" : "text-ink"}`}>
                {delta.value.startsWith("-") ? formatUsdt(delta.value) : formatUsdtSigned(delta.value)}
              </span>
            </li>
          );
        })}
      </ul>
    </li>
  );
}
