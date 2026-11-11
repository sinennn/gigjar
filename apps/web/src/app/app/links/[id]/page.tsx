import { LinkDetailScreen } from "@/components/screens/link-detail";

export const metadata = { title: "Payment details" };

export default async function PaymentLinkDetailPage({ params }: PageProps<"/app/links/[id]">) {
  const { id } = await params;
  return <LinkDetailScreen id={id} />;
}
