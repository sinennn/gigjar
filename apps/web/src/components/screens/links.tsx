"use client";

import Link from "next/link";
import { useState } from "react";
import type { Schemas } from "@/lib/api/client";
import { errorText, useCursorList } from "@/lib/api/hooks";
import { formatDay } from "@/lib/format";
import { formatUsdt, isZeroUsdt } from "@/lib/money";
import { Button, ButtonLink } from "@/components/ui/button";
import { StatusBadge } from "@/components/ui/badge";
import { CopyButton } from "@/components/ui/copy-button";
import { Glass } from "@/components/ui/glass";
import { EmptyState, ErrorCard, LoadingCard, PageHeader } from "@/components/ui/page";

type PaymentLink = Schemas["PaymentLink"];
type Filter = "all" | Schemas["PaymentLinkStatus"];

const FILTERS: { value: Filter; label: string }[] = [
  { value: "all", label: "All" },
  { value: "open", label: "Open" },
  { value: "paid", label: "Paid" },
  { value: "closed", label: "Closed" },
];

export function LinksScreen() {
  const [filter, setFilter] = useState<Filter>("all");
  const path = `/v1/payment-links?limit=20${filter === "all" ? "" : `&status=${filter}`}`;
  const list = useCursorList<PaymentLink>(path);

  return (
    <>
      <PageHeader
        title="Payment links"
        summary="One link per gig. Share it and get paid."
        actions={
          <ButtonLink href="/app/links/new" size="lg">
            New link
          </ButtonLink>
        }
      />

      <div role="group" aria-label="Filter by status" className="mb-4 flex flex-wrap gap-2">
        {FILTERS.map((item) => (
          <button
            key={item.value}
            type="button"
            aria-pressed={filter === item.value}
            onClick={() => setFilter(item.value)}
            className={`relative inline-flex min-h-11 items-center rounded-full px-5 text-[15px] font-semibold transition-colors ${
              filter === item.value ? "bg-ink text-paper" : "glass text-ink-soft hover:text-ink"
            }`}
          >
            {item.label}
          </button>
        ))}
      </div>

      {list.error && list.items.length === 0 ? <ErrorCard message={errorText(list.error)} onRetry={list.reload} /> : null}
      {list.loading && list.items.length === 0 && !list.error ? <LoadingCard lines={4} /> : null}

      {!list.loading && !list.error && list.items.length === 0 ? (
        <Glass size="lg" className="overflow-hidden">
          <EmptyState
            title={filter === "all" ? "No payment links yet" : `No ${filter} links`}
            action={
              filter === "all" ? (
                <ButtonLink href="/app/links/new" variant="primary">
                  Create your first link
                </ButtonLink>
              ) : undefined
            }
          >
            {filter === "all"
              ? "Make one link for each gig, send it to your client, and the payment lands in your jar."
              : "Nothing matches this filter."}
          </EmptyState>
        </Glass>
      ) : null}

      {list.items.length > 0 ? (
        <Glass as="section" size="lg" aria-label="Payment links" className="overflow-hidden">
          <ul className="m-0 list-none divide-y divide-line/70 p-0">
            {list.items.map((link) => (
              <LinkRow key={link.id} link={link} />
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

function LinkRow({ link }: { link: PaymentLink }) {
  const received = !isZeroUsdt(link.amount_received);
  return (
    <li className="flex flex-col gap-3 px-5 py-4 sm:flex-row sm:items-center sm:justify-between sm:gap-6 sm:px-6">
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <Link
            href={`/app/links/${link.id}`}
            className="min-w-0 text-[17px] font-semibold break-words text-ink no-underline hover:underline hover:underline-offset-4"
          >
            {link.title}
          </Link>
          <StatusBadge status={link.status} />
        </div>
        {link.description ? <p className="m-0 line-clamp-1 text-sm text-ink-soft">{link.description}</p> : null}
        <p className="m-0 text-sm text-muted">Created {formatDay(link.created_at)}</p>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3 sm:flex-none sm:justify-end sm:gap-6">
        <div className="sm:text-right">
          <p className="m-0 font-display text-xl leading-none font-bold tracking-[-0.02em] text-ink tabular-nums">
            {formatUsdt(link.amount)}
          </p>
          <p className={`m-0 mt-1 text-sm tabular-nums ${received ? "font-medium text-jar-green" : "text-muted"}`}>
            {received ? `${formatUsdt(link.amount_received)} received` : "Nothing received yet"}
          </p>
        </div>
        <CopyButton value={link.pay_url} label="Copy link" />
      </div>
    </li>
  );
}
