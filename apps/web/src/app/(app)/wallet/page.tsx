import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Cleva wallet" };

export default function WalletPage() {
  return (
    <ScreenScaffold
      title={"Cleva wallet"}
      summary={"Where your jar sends money once it's full."}
      mustShow={["Current Cleva USDT TRC20 deposit address", "Form to change it, validated as a TRON address", "Warning that only the TRC20 deposit address works", "Note that in-flight settlements keep their original destination"]}
      endpoints={["GET /v1/wallet/cleva", "PUT /v1/wallet/cleva"]}
    />
  );
}
