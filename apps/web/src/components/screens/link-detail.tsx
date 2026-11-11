"use client";

import Link from "next/link";
import { useState } from "react";
import { ApiError, apiRequest, describeError, type Schemas } from "@/lib/api/client";
import { errorText, useApi } from "@/lib/api/hooks";
import { formatDay } from "@/lib/format";
import { formatUsdt, isZeroUsdt } from "@/lib/money";
import { TransferRow } from "@/components/app/transfer-rows";
import { Alert } from "@/components/ui/alert";
import { StatusBadge } from "@/components/ui/badge";
import { Button, ButtonLink } from "@/components/ui/button";
import { CopyButton } from "@/components/ui/copy-button";
import { Glass } from "@/components/ui/glass";
import { Icon } from "@/components/ui/icons";
import { EmptyState, ErrorCard, LoadingCard, PageHeader, Section } from "@/components/ui/page";

type Detail = Schemas["PaymentLinkDetail"];

export function LinkDetailScreen({ id }: { id: string }) {
  const { data: link, error, loading, reload } = useApi<Detail>(`/v1/payment-links/${encodeURIComponent(id)}`, { pollMs: 10_000 });
  const [confirming, setConfirming] = useState(false);
  const [closing, setClosing] = useState(false);
  const [closeError, setCloseError] = useState<string | null>(null);

  const back = (
    <Link href="/app/links" className="inline-flex min-h-11 items-center gap-1.5 text-sm font-medium text-ink-soft no-underline hover:text-ink">
      <Icon name="back" className="size-4" /> Payment links
    </Link>
  );

  if (error && !link) {
    const missing = error instanceof ApiError && (error.status === 404 || error.status === 422);
    return (
      <>
        <PageHeader title={missing ? "Link not found" : "Payment details"} back={back} />
        {missing ? (
          <Glass size="lg" className="overflow-hidden">
            <EmptyState title="We could not find that link" action={<ButtonLink href="/app/links">Back to your links</ButtonLink>}>
              It may have been mistyped or belong to another account.
            </EmptyState>
          </Glass>
        ) : (
          <ErrorCard message={errorText(error)} onRetry={reload} />
        )}
      </>
    );
  }
  if (!link) {
    return (
      <>
        <PageHeader title="Payment details" back={back} />
        {loading ? <LoadingCard lines={4} /> : null}
      </>
    );
  }

  async function close() {
    setClosing(true);
    setCloseError(null);
    try {
      await apiRequest<Schemas["PaymentLink"]>(`/v1/payment-links/${encodeURIComponent(id)}/close`, { method: "POST" });
      setConfirming(false);
      reload();
    } catch (failure) {
      setCloseError(describeError(failure).message);
    } finally {
      setClosing(false);
    }
  }

  const received = !isZeroUsdt(link.amount_received);

  return (
    <>
      <PageHeader
        title={link.title}
        summary={link.description || undefined}
        back={back}
        actions={<StatusBadge status={link.status} />}
      />

      <div className="flex flex-col gap-4 sm:gap-5">
        <div className="grid gap-4 sm:grid-cols-2 sm:gap-5">
          <Glass size="lg" className="flex flex-col gap-1.5 p-5">
            <p className="m-0 text-sm font-medium text-ink-soft">Price</p>
            <p className="m-0 font-display text-[32px] leading-none font-bold tracking-[-0.03em] text-ink tabular-nums">
              {formatUsdt(link.amount)}
            </p>
            <p className="m-0 text-sm text-muted">Created {formatDay(link.created_at)}</p>
          </Glass>
          <Glass size="lg" className="flex flex-col gap-1.5 p-5">
            <p className="m-0 flex items-center gap-2 text-sm font-medium text-ink-soft">
              {received ? <span aria-hidden="true" className="size-2 rounded-full bg-jar-green" /> : null}
              Received
            </p>
            <p
              className={`m-0 font-display text-[32px] leading-none font-bold tracking-[-0.03em] tabular-nums ${
                received ? "text-jar-green" : "text-ink"
              }`}
            >
              {formatUsdt(link.amount_received)}
            </p>
            <p className="m-0 text-sm text-muted">{link.paid_at ? `Paid ${formatDay(link.paid_at)}` : "Confirmed payments only."}</p>
          </Glass>
        </div>

        <Glass as="section" size="lg" className="space-y-4 p-5 sm:p-6">
          <h2 className="m-0 font-display text-xl font-bold tracking-[-0.02em]">Share this link</h2>
          <p className="m-0 rounded-2xl bg-paper/80 px-4 py-3 font-mono text-[14px] leading-relaxed break-all text-ink">{link.pay_url}</p>
          <div className="flex flex-wrap gap-3">
            <CopyButton value={link.pay_url} label="Copy link" />
            <ButtonLink href={`/pay/${encodeURIComponent(link.reference)}`} variant="secondary" target="_blank" rel="noreferrer">
              See what your client sees <Icon name="external" className="size-4" />
            </ButtonLink>
          </div>
          <p className="m-0 text-sm text-muted">Clients pay in USDT on TRON (TRC20) only. The page tells them so.</p>
        </Glass>

        <Section title="Payments for this gig">
          {link.transfers.length > 0 ? (
            <ul className="m-0 list-none divide-y divide-line/70 p-0">
              {link.transfers.map((transfer) => (
                <TransferRow key={transfer.id} transfer={transfer} showLink={false} />
              ))}
            </ul>
          ) : (
            <EmptyState title="No payments yet">
              {link.status === "open" ? "When your client pays, it shows here once the network confirms it." : "Nothing was paid to this link."}
            </EmptyState>
          )}
        </Section>

        {link.status === "open" ? (
          <Glass as="section" size="lg" lens={false} className="space-y-4 p-5 sm:p-6">
            <h2 className="m-0 font-display text-xl font-bold tracking-[-0.02em]">Close this link</h2>
            {confirming ? (
              <div className="space-y-4">
                <Alert tone="warning">
                  Your client will see this link as closed. Money already received stays in your jar.
                </Alert>
                {closeError ? <Alert>{closeError}</Alert> : null}
                <div className="flex flex-wrap gap-3">
                  <Button variant="danger" loading={closing} onClick={close}>
                    Yes, close it
                  </Button>
                  <Button variant="quiet" onClick={() => setConfirming(false)} disabled={closing}>
                    Keep it open
                  </Button>
                </div>
              </div>
            ) : (
              <>
                <p className="m-0 max-w-xl text-[15px] leading-relaxed text-ink-soft">
                  Done with this gig? Closing the link stops it from taking new payments.
                </p>
                <Button variant="secondary" onClick={() => setConfirming(true)}>
                  Close link
                </Button>
              </>
            )}
          </Glass>
        ) : null}
      </div>
    </>
  );
}
