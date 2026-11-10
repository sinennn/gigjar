import { ScreenScaffold } from "@/components/screen-scaffold";

export const metadata = { title: "Payment links" };

export default function PaymentLinksPage() {
  return (
    <ScreenScaffold
      title={"Payment links"}
      summary={"One link per gig. Share it and get paid."}
      mustShow={["List of links with title, amount, received amount and status", "Filter by status", "Create link action", "Copy pay URL"]}
      endpoints={["GET /v1/payment-links"]}
    />
  );
}
