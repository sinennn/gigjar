const USDT_AMOUNT_PATTERN = /^(-?)(\d+)\.(\d{6})$/;
const USDT_INPUT_PATTERN = /^(\d{1,9})(?:\.(\d{1,6}))?$/;

export type UsdtAmount = string;

export function isUsdtAmount(value: string): value is UsdtAmount {
  return USDT_AMOUNT_PATTERN.test(value);
}

export function formatUsdt(amount: UsdtAmount): string {
  const match = USDT_AMOUNT_PATTERN.exec(amount);
  if (!match) {
    throw new Error(`invalid USDT amount: ${amount}`);
  }
  const [, sign, whole, fraction] = match;
  const groupedWhole = whole.replace(/\B(?=(\d{3})+(?!\d))/g, ",");
  const trimmedFraction = fraction.replace(/0{1,4}$/, "").padEnd(2, "0");
  return `${sign}$${groupedWhole}.${trimmedFraction}`;
}

/** Like `formatUsdt` but with an explicit "+" on gains, for ledger deltas. */
export function formatUsdtSigned(amount: UsdtAmount): string {
  const text = formatUsdt(amount);
  return text.startsWith("-") ? text : `+${text}`;
}

/** Whole base units (1e-6 USDT) as a BigInt, so sums and ratios never touch floating point. */
export function toMicros(amount: UsdtAmount): bigint {
  const match = USDT_AMOUNT_PATTERN.exec(amount);
  if (!match) {
    throw new Error(`invalid USDT amount: ${amount}`);
  }
  const [, sign, whole, fraction] = match;
  const micros = BigInt(whole) * 1_000_000n + BigInt(fraction);
  return sign === "-" ? -micros : micros;
}

export function fromMicros(micros: bigint): UsdtAmount {
  const negative = micros < 0n;
  const abs = negative ? -micros : micros;
  const whole = abs / 1_000_000n;
  const fraction = (abs % 1_000_000n).toString().padStart(6, "0");
  return `${negative ? "-" : ""}${whole}.${fraction}`;
}

export function addUsdt(...amounts: UsdtAmount[]): UsdtAmount {
  return fromMicros(amounts.reduce((sum, amount) => sum + toMicros(amount), 0n));
}

export function isZeroUsdt(amount: UsdtAmount): boolean {
  return toMicros(amount) === 0n;
}

/** `part / whole` as a number from 0 to 1, for progress bars only (never for money). */
export function usdtRatio(part: UsdtAmount, whole: UsdtAmount): number {
  const total = toMicros(whole);
  if (total <= 0n) {
    return 0;
  }
  const permille = Number((toMicros(part) * 1000n) / total);
  return Math.min(1, Math.max(0, permille / 1000));
}

/**
 * Turns what someone typed ("4", "4.5", "12.250000") into the API's six-digit decimal string
 * without going through a float. Returns null when it is not a plain positive amount.
 */
export function parseUsdtInput(text: string): UsdtAmount | null {
  const match = USDT_INPUT_PATTERN.exec(text.trim());
  if (!match) {
    return null;
  }
  const amount = `${match[1]}.${(match[2] ?? "").padEnd(6, "0")}`;
  return toMicros(amount) > 0n ? amount : null;
}
