import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "New payment link" };

export default function NewPaymentLinkPage() {
  return (
    <ScreenScaffold
      title={"New payment link"}
      summary={"Describe the gig and set the price in USDT."}
      mustShow={["Title, description and amount fields", "Amount sent as a decimal string, never a float", "Idempotency-Key header per submit to avoid duplicates", "USDT on TRC20 only notice"]}
      endpoints={["POST /v1/payment-links"]}
    />
  );
}
