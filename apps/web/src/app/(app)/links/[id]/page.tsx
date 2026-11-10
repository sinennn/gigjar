import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Payment details" };

export default function PaymentLinkDetailPage() {
  return (
    <ScreenScaffold
      title={"Payment details"}
      summary={"Everything about one gig payment."}
      mustShow={["Link details, status and shareable pay URL", "Transfers attributed to this link with tx hash and status", "Close link action"]}
      endpoints={["GET /v1/payment-links/{id}", "POST /v1/payment-links/{id}/close"]}
    />
  );
}
