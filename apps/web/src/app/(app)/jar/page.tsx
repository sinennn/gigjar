import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Your jar" };

export default function JarPage() {
  return (
    <ScreenScaffold
      title={"Your jar"}
      summary={"Payments pool here until they're worth sending to Cleva."}
      mustShow={["Pending, available, settling and settled balances, each explained", "Progress toward the settlement threshold", "Jar address, network, and a USDT TRC20 only warning"]}
      endpoints={["GET /v1/jar"]}
    />
  );
}
