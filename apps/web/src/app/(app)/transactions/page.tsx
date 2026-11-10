import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Transactions" };

export default function TransactionsPage() {
  return (
    <ScreenScaffold
      title={"Transactions"}
      summary={"Every movement in and out of your jar."}
      mustShow={["Ledger movements with kind, signed deltas and time", "Incoming transfers including pending ones", "Cursor pagination"]}
      endpoints={["GET /v1/transactions", "GET /v1/incoming-transfers"]}
    />
  );
}
