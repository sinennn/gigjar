const USDT_AMOUNT_PATTERN = /^(-?)(\d+)\.(\d{6})$/;

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
