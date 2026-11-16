"use client";

import Link from "next/link";
import { useState, type ReactNode } from "react";
import { LensRoot } from "@/components/lens-root";
import { Backdrop } from "@/components/backdrop";
import { JarMark } from "@/components/jar-mark";
import { Badge, SimulatedBadge } from "@/components/ui/badge";
import { CopyButton } from "@/components/ui/copy-button";
import { Glass } from "@/components/ui/glass";
import { Icon } from "@/components/ui/icons";
import { useApi } from "@/lib/api/hooks";
import type { PublicPayment } from "@/lib/api/public";
import { networkLabel } from "@/lib/format";
import { formatUsdtNumber, plainUsdt } from "@/lib/money";
import { QrCode } from "./qr-code";

/**
 * What a client sees when they open a shared link. The server sends the first copy so the page arrives
 * complete; from there it checks again every few seconds while the link is open, so "waiting",
 * "we can see your payment" and "paid" change on their own. Once the link is paid or closed it stops.
 */
export function PayView({ initial }: { initial: PublicPayment }) {
  // Checking stops for good once the link is paid or closed, so a finished page costs nothing.
  const [stopped, setStopped] = useState(initial.status !== "open");
  // Starts from the server's copy. The live copy replaces it as soon as it arrives; a failed check
  // keeps whatever is on screen rather than blanking a page someone is paying from.
  const live = useApi<PublicPayment>(`/v1/public/pay/${encodeURIComponent(initial.reference)}`, {
    pollMs: stopped ? undefined : 6000,
  });
  const payment = live.data ?? initial;
  const finished = payment.status !== "open";
  if (finished && !stopped) {
    setStopped(true);
  }
  const amount = formatUsdtNumber(payment.amount);

  return (
    <LensRoot className="relative isolate min-h-dvh px-4 pt-5 pb-14 sm:pt-8">
      <Backdrop fixed />
      <div className="mx-auto flex w-full max-w-[520px] flex-col gap-4 sm:gap-5">
        <header className="flex items-center justify-between gap-3">
          <Link
            href="/"
            className="inline-flex min-h-11 items-center gap-2.5 font-display text-lg font-bold tracking-[-0.02em] text-ink no-underline"
          >
            <JarMark className="h-7 w-7 text-ink" />
            GigJar
          </Link>
          <NetworkNotice network={payment.network} />
        </header>

        <Glass as="section" size="xl" aria-labelledby="gig-title" className="flex flex-col gap-5 p-6 sm:p-8">
          <div className="flex flex-col gap-2">
            <p className="m-0 text-sm font-medium text-ink-soft">Pay {payment.recipient_display_name}</p>
            <h1 id="gig-title" className="m-0 font-display text-[30px] leading-[1.1] font-bold tracking-[-0.03em] text-ink sm:text-[34px]">
              {payment.title}
            </h1>
            {payment.description ? (
              <p className="m-0 text-[15px] leading-relaxed whitespace-pre-line text-ink-soft">{payment.description}</p>
            ) : null}
          </div>

          <div className="flex flex-wrap items-end justify-between gap-x-4 gap-y-3">
            <p className="m-0 flex items-baseline gap-2 text-ink">
              <span className="font-display text-[52px] leading-none font-bold tracking-[-0.04em] tabular-nums sm:text-[60px]">
                {amount}
              </span>
              <span className="text-lg font-semibold text-ink-soft">USDT</span>
            </p>
            {!finished ? <CopyButton value={plainUsdt(payment.amount)} label="Copy amount" variant="quiet" className="-ml-5 sm:ml-0" /> : null}
          </div>

          <StatusPanel payment={payment} />
        </Glass>

        {!finished ? (
          <>
            <Glass as="section" size="lg" aria-labelledby="pay-to" className="flex flex-col gap-5 p-6 sm:p-7">
              <div role="note" className="rounded-2xl bg-lid-orange-soft px-4 py-3.5 text-[15px] leading-relaxed text-ink">
                <p className="m-0 font-semibold">USDT on TRON (TRC20) only.</p>
                <p className="m-0">
                  Send exactly {amount} USDT. Money sent on another network, or in another coin, is lost and cannot be recovered.
                </p>
              </div>

              <div className="flex flex-col gap-2">
                <h2 id="pay-to" className="m-0 font-display text-xl font-bold tracking-[-0.02em]">
                  Send it to this address
                </h2>
                {/* The address is 34 characters. The size follows the screen so it stays on one line, never one
                    stray letter on a second line, and stays one selectable piece of text. */}
                <p
                  className="m-0 rounded-2xl bg-paper/80 px-4 py-3 font-mono leading-relaxed break-all text-ink select-all"
                  style={{ fontSize: "clamp(10.5px, calc((100vw - 112px) / 21), 15px)" }}
                >
                  {payment.pay_to_address}
                </p>
                <div>
                  <CopyButton value={payment.pay_to_address} label="Copy address" />
                </div>
              </div>

              <div className="flex flex-col items-center gap-3">
                <QrCode
                  value={payment.pay_to_address}
                  label={`QR code of the payment address ${payment.pay_to_address}`}
                  className="w-full max-w-[220px] shadow-[0_0_0_1px_rgb(19_38_58/0.08)]"
                />
                <p className="m-0 text-sm text-muted">Scan it with your wallet app, or copy the address.</p>
              </div>

              <ol className="m-0 flex list-none flex-col gap-2.5 p-0 text-[15px] leading-relaxed text-ink-soft">
                <Step n="1">Open your wallet and choose USDT on the TRON (TRC20) network.</Step>
                <Step n="2">
                  Send exactly <strong className="font-semibold text-ink">{amount} USDT</strong> to the address above.
                </Step>
                <Step n="3">This page updates on its own once the network confirms your payment. You can leave it open.</Step>
              </ol>
            </Glass>

            <Details payment={payment} />
          </>
        ) : null}

        <footer className="px-1 pt-1 text-center text-sm text-muted">
          Payment page by{" "}
          <Link href="/" className="font-medium text-ink-soft underline underline-offset-4">
            GigJar
          </Link>
        </footer>
      </div>
    </LensRoot>
  );
}

function NetworkNotice({ network }: { network: PublicPayment["network"] }) {
  if (network === "simulated") {
    return <SimulatedBadge />;
  }
  if (network === "tron-nile") {
    return <Badge tone="orange">Test network</Badge>;
  }
  return null;
}

/** The live part: waiting, "we can see it", paid, or closed. It is a live region, so it is read out as it changes. */
function StatusPanel({ payment }: { payment: PublicPayment }) {
  const base = "flex items-start gap-3 rounded-2xl px-4 py-3.5 text-[15px] leading-relaxed";

  if (payment.status === "paid") {
    return (
      <div role="status" className={`${base} bg-jar-green-soft text-ink`}>
        <span aria-hidden="true" className="mt-0.5 flex size-6 flex-none items-center justify-center rounded-full bg-jar-green text-paper">
          <Icon name="check" className="size-4" />
        </span>
        <p className="m-0">
          <strong className="font-semibold">Paid. Thank you.</strong> {payment.recipient_display_name} has received this payment, so there is
          nothing more to send.
        </p>
      </div>
    );
  }
  if (payment.status === "closed") {
    return (
      <div role="status" className={`${base} bg-ink/6 text-ink`}>
        <p className="m-0">
          <strong className="font-semibold">This link is closed.</strong> It is not taking payments any more. Please do not send anything to
          it, and ask {payment.recipient_display_name} for a new link if you still need to pay.
        </p>
      </div>
    );
  }
  if (payment.pending_payment_observed) {
    return (
      <div role="status" className={`${base} bg-lid-orange-soft text-ink`}>
        <span aria-hidden="true" className="mt-1.5 size-2.5 flex-none rounded-full bg-lid-orange motion-safe:animate-pulse" />
        <p className="m-0">
          <strong className="font-semibold">We can see your payment.</strong> It is waiting for the network to confirm it. You do not need
          to send it again.
        </p>
      </div>
    );
  }
  return (
    <div role="status" className={`${base} bg-ink/6 text-ink-soft`}>
      <span aria-hidden="true" className="mt-1.5 size-2.5 flex-none rounded-full bg-muted motion-safe:animate-pulse" />
      <p className="m-0">Waiting for your payment. Send it with the details below.</p>
    </div>
  );
}

function Step({ n, children }: { n: string; children: ReactNode }) {
  return (
    <li className="flex gap-3">
      <span
        aria-hidden="true"
        className="flex size-6 flex-none items-center justify-center rounded-full bg-ink font-display text-[13px] font-bold text-paper"
      >
        {n}
      </span>
      <span>{children}</span>
    </li>
  );
}

/** For people who want to check the token before sending. The contract is null on the simulated network. */
function Details({ payment }: { payment: PublicPayment }) {
  const contract = payment.token.contract_address;
  return (
    <Glass as="section" size="lg" lens={false} className="p-2">
      <details className="group">
        <summary className="flex min-h-12 cursor-pointer list-none items-center justify-between gap-3 rounded-2xl px-4 text-[15px] font-semibold text-ink [&::-webkit-details-marker]:hidden">
          Check the token and network
          <Icon name="back" className="size-4 -rotate-90 text-ink-soft transition-transform group-open:rotate-90" />
        </summary>
        <dl className="m-0 flex flex-col gap-3 px-4 pt-1 pb-4 text-sm">
          <Row label="Coin">
            {payment.token.symbol} ({payment.token.standard})
          </Row>
          <Row label="Network">{networkLabel(payment.network)}</Row>
          {contract ? (
            <div className="flex flex-col gap-2">
              <dt className="text-muted">USDT contract address</dt>
              <dd className="m-0 break-all font-mono text-[13px] text-ink">{contract}</dd>
              <div>
                <CopyButton value={contract} label="Copy contract" variant="quiet" />
              </div>
            </div>
          ) : (
            <Row label="Contract">None on the simulated network, where no real money moves.</Row>
          )}
        </dl>
      </details>
    </Glass>
  );
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-0.5 sm:flex-row sm:gap-4">
      <dt className="text-muted sm:w-28 sm:flex-none">{label}</dt>
      <dd className="m-0 text-ink">{children}</dd>
    </div>
  );
}
