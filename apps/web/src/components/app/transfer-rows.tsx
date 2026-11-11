import Link from "next/link";
import type { Schemas } from "@/lib/api/client";
import { StatusBadge } from "@/components/ui/badge";
import { Icon } from "@/components/ui/icons";
import { describeFailure, explorerTxUrl, formatWhen, shorten } from "@/lib/format";
import { formatUsdt } from "@/lib/money";

type Transfer = Schemas["IncomingTransfer"];
type Settlement = Schemas["Settlement"];

/** One payment seen on the network. Pending ones read as "not yet confirmed", never as money in hand. */
export function TransferRow({ transfer, showLink = false }: { transfer: Transfer; showLink?: boolean }) {
  const explorer = explorerTxUrl(transfer.network, transfer.tx_hash);
  const confirmed = transfer.status === "confirmed";
  return (
    <li className="flex flex-col gap-2 px-5 py-4 sm:px-6">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span
            aria-hidden="true"
            className={`flex size-9 flex-none items-center justify-center rounded-[12px] ${
              confirmed ? "bg-jar-green text-paper" : transfer.status === "failed" ? "bg-danger-soft text-danger" : "bg-lid-orange-soft text-[#7a4510]"
            }`}
          >
            <Icon name="send" className="size-[18px] rotate-90" />
          </span>
          <div className="min-w-0">
            <p className="m-0 truncate text-[15px] font-semibold text-ink" title={transfer.from_address}>
              From {shorten(transfer.from_address, 5, 5)}
            </p>
            <p className="m-0 text-sm text-muted">{formatWhen(transfer.confirmed_at ?? transfer.observed_at)}</p>
          </div>
        </div>
        <div className="flex flex-none flex-col items-end gap-1.5">
          <p
            className={`m-0 font-display text-lg leading-none font-bold tracking-[-0.02em] tabular-nums ${
              confirmed ? "text-jar-green" : transfer.status === "failed" ? "text-muted line-through" : "text-ink-soft"
            }`}
          >
            {confirmed ? "+" : ""}
            {formatUsdt(transfer.amount)}
          </p>
          <StatusBadge status={transfer.status} />
        </div>
      </div>
      {transfer.status === "pending" ? (
        <p className="m-0 pl-12 text-sm text-ink-soft">Seen on the network. It counts once it is confirmed.</p>
      ) : null}
      {transfer.status === "failed" ? (
        <p className="m-0 pl-12 text-sm text-danger">Not credited. {describeFailure(transfer.failure_reason)}</p>
      ) : null}
      {showLink ? (
        <p className="m-0 flex flex-wrap items-center gap-x-4 gap-y-1 pl-12 text-sm text-muted">
          <span title={transfer.tx_hash} className="font-mono">
            {shorten(transfer.tx_hash, 8, 6)}
          </span>
          {explorer ? (
            <a href={explorer} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-ink-soft underline underline-offset-4">
              View on explorer <Icon name="external" className="size-3.5" />
            </a>
          ) : null}
          {transfer.payment_link_id ? (
            <Link href={`/app/links/${transfer.payment_link_id}`} className="text-ink-soft underline underline-offset-4">
              Payment link
            </Link>
          ) : null}
        </p>
      ) : null}
    </li>
  );
}

/** One sweep from the jar to Cleva. */
export function SettlementCard({ settlement }: { settlement: Settlement }) {
  const explorer = settlement.tx_id ? explorerTxUrl(settlement.network, settlement.tx_id) : null;
  return (
    <li className="flex flex-col gap-4 px-5 py-5 sm:px-6">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="m-0 text-sm text-muted">{formatWhen(settlement.created_at)}</p>
          <p className="m-0 font-display text-[28px] leading-tight font-bold tracking-[-0.03em] text-ink tabular-nums">
            {formatUsdt(settlement.net_amount)}
            <span className="ml-2 text-sm font-medium tracking-normal text-muted">to Cleva</span>
          </p>
        </div>
        <StatusBadge status={settlement.state} kind="settlement" />
      </div>

      <dl className="m-0 grid grid-cols-2 gap-x-4 gap-y-3 text-sm sm:grid-cols-4">
        <Detail label="Taken from jar" value={formatUsdt(settlement.amount)} />
        <Detail label="Fee" value={formatUsdt(settlement.fee)} />
        <Detail label="Sent to" value={shorten(settlement.destination_address, 5, 5)} title={settlement.destination_address} mono />
        <Detail label={settlement.confirmed_at ? "Confirmed" : "Status"} value={settlement.confirmed_at ? formatWhen(settlement.confirmed_at) : settlementHint(settlement.state)} />
      </dl>

      {settlement.tx_id ? (
        <p className="m-0 flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted">
          <span title={settlement.tx_id} className="font-mono">
            {shorten(settlement.tx_id, 8, 6)}
          </span>
          {explorer ? (
            <a href={explorer} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-ink-soft underline underline-offset-4">
              View on explorer <Icon name="external" className="size-3.5" />
            </a>
          ) : (
            <span>Simulated, not on a real chain</span>
          )}
        </p>
      ) : null}

      {settlement.state === "failed" ? (
        <div className="rounded-2xl bg-lid-orange-soft px-4 py-3 text-sm leading-relaxed text-ink">
          <p className="m-0 font-semibold">This payout did not go through.</p>
          <p className="m-0">
            {describeFailure(settlement.failure_reason)} Your {formatUsdt(settlement.amount)} is back in your jar, and GigJar will try
            again on its own.
          </p>
        </div>
      ) : null}
    </li>
  );
}

function settlementHint(state: Settlement["state"]): string {
  switch (state) {
    case "queued":
      return "Getting ready";
    case "signing":
      return "Signing";
    case "broadcast":
      return "Waiting for the network";
    case "failed":
      return "Returned to jar";
    case "confirmed":
      return "Confirmed";
  }
}

function Detail({ label, value, title, mono = false }: { label: string; value: string; title?: string; mono?: boolean }) {
  return (
    <div className="min-w-0">
      <dt className="text-muted">{label}</dt>
      <dd className={`m-0 truncate font-medium text-ink ${mono ? "font-mono text-[13px]" : ""}`} title={title}>
        {value}
      </dd>
    </div>
  );
}
