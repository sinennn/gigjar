"use client";

import type { Schemas } from "@/lib/api/client";
import { errorText, useCursorList } from "@/lib/api/hooks";
import { formatUsdt } from "@/lib/money";
import { useJar } from "@/components/app/jar-context";
import { SettlementCard } from "@/components/app/transfer-rows";
import { Button } from "@/components/ui/button";
import { Glass } from "@/components/ui/glass";
import { EmptyState, ErrorCard, LoadingCard, PageHeader } from "@/components/ui/page";

export function SettlementsScreen() {
  const list = useCursorList<Schemas["Settlement"]>("/v1/settlements?limit=15");
  const { jar } = useJar();

  return (
    <>
      <PageHeader title="Settlements" summary="Payouts from your jar to Cleva." />

      {list.error && list.items.length === 0 ? <ErrorCard message={errorText(list.error)} onRetry={list.reload} /> : null}
      {list.loading && list.items.length === 0 && !list.error ? <LoadingCard lines={5} /> : null}

      {!list.loading && !list.error && list.items.length === 0 ? (
        <Glass size="lg" className="overflow-hidden">
          <EmptyState title="No payouts yet">
            {jar
              ? `Your jar pays out on its own once it holds ${formatUsdt(jar.settlement_threshold)}, minus a flat ${formatUsdt(jar.settlement_fee)} fee.`
              : "Your jar pays out on its own once it is full."}
          </EmptyState>
        </Glass>
      ) : null}

      {list.items.length > 0 ? (
        <Glass as="section" size="lg" aria-label="Settlements" className="overflow-hidden">
          <ul className="m-0 list-none divide-y divide-line/70 p-0">
            {list.items.map((settlement) => (
              <SettlementCard key={settlement.id} settlement={settlement} />
            ))}
          </ul>
          {list.error ? <p className="m-0 px-6 py-3 text-sm text-danger">{errorText(list.error)}</p> : null}
          {list.hasMore ? (
            <div className="border-t border-line/70 p-4 text-center">
              <Button variant="secondary" loading={list.loadingMore} onClick={list.loadMore}>
                Load more
              </Button>
            </div>
          ) : null}
        </Glass>
      ) : null}
    </>
  );
}
