import type { Schemas } from "./api/client";

type Network = Schemas["Network"];

const WHEN = new Intl.DateTimeFormat("en", { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" });
const DAY = new Intl.DateTimeFormat("en", { month: "short", day: "numeric", year: "numeric" });

export function formatWhen(iso: string): string {
  return WHEN.format(new Date(iso));
}

export function formatDay(iso: string): string {
  return DAY.format(new Date(iso));
}

/** `TXyz…9abc`, for tables where the full value is in a title attribute or a copy button. */
export function shorten(value: string, head = 6, tail = 6): string {
  return value.length <= head + tail + 1 ? value : `${value.slice(0, head)}…${value.slice(-tail)}`;
}

export function networkLabel(network: Network): string {
  switch (network) {
    case "simulated":
      return "Simulated network";
    case "tron-nile":
      return "TRON Nile test network";
    case "tron-mainnet":
      return "TRON mainnet";
  }
}

/** Block explorer page for a transaction. Simulated transactions are not on any chain. */
export function explorerTxUrl(network: Network, txId: string): string | null {
  switch (network) {
    case "tron-mainnet":
      return `https://tronscan.org/#/transaction/${txId}`;
    case "tron-nile":
      return `https://nile.tronscan.org/#/transaction/${txId}`;
    default:
      return null;
  }
}

const FAILURE_REASONS: Record<string, string> = {
  receipt_revert: "The network rejected the transfer.",
  receipt_out_of_energy: "The transfer ran out of network energy.",
  not_solidified: "It never became final within 24 hours.",
  no_matching_transfer: "No matching transfer was found on the network.",
};

export function describeFailure(reason: string | null): string {
  if (!reason) {
    return "It did not go through.";
  }
  return FAILURE_REASONS[reason] ?? `${reason.replace(/_/g, " ")}.`;
}
