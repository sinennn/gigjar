import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Settlements" };

export default function SettlementsPage() {
  return (
    <ScreenScaffold
      title={"Settlements"}
      summary={"Sweeps from your jar to Cleva."}
      mustShow={["Amount, destination, state and tx id with an explorer link", "Failure reason for failed sweeps, with funds shown as returned to the jar", "Cursor pagination"]}
      endpoints={["GET /v1/settlements"]}
    />
  );
}
