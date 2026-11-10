import { JarMark } from "@/components/jar-mark";
import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Pay for a gig" };

export default function PublicPaymentPage() {
  return (
    <div className="mx-auto max-w-xl space-y-6 px-4 py-10">
      <JarMark className="h-10 w-10 text-ink" />
      <ScreenScaffold
        title="Pay for a gig"
        summary="Public page a client opens from a shared payment link. No login."
        mustShow={[
          "Gig title, description and amount in USDT",
          "Recipient display name",
          "Pay-to address with copy button and QR code",
          "Prominent warning: send USDT on TRON (TRC20) only; other networks are lost",
          "USDT contract address for verification",
          "Live status once a transfer is observed (pending, then confirmed)",
          "Simulated badge when the network is simulated",
        ]}
        endpoints={["GET /v1/public/pay/{reference}"]}
      />
    </div>
  );
}
