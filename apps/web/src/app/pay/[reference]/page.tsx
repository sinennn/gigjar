import type { Metadata } from "next";
import { notFound } from "next/navigation";
import { ApiDown } from "@/components/auth/api-down";
import { Backdrop } from "@/components/backdrop";
import { PayView } from "@/components/pay/pay-view";
import { Glass } from "@/components/ui/glass";
import { JarMark } from "@/components/jar-mark";
import { getPublicPayment } from "@/lib/api/public";
import { formatUsdtNumber } from "@/lib/money";

// A payment page is for the person holding the link, not for search engines.
const robots = { index: false, follow: false };

export async function generateMetadata({ params }: PageProps<"/pay/[reference]">): Promise<Metadata> {
  const { reference } = await params;
  const lookup = await getPublicPayment(reference);
  if (lookup.kind !== "ok") {
    return { title: "Pay for a gig", robots };
  }
  const { title, amount, recipient_display_name: recipient } = lookup.payment;
  const description = `Pay ${formatUsdtNumber(amount)} USDT to ${recipient} for "${title}". USDT on TRON (TRC20) only.`;
  return {
    title: `Pay ${recipient}: ${title}`,
    description,
    robots,
    openGraph: { title: `Pay ${recipient}: ${title}`, description },
    twitter: { title: `Pay ${recipient}: ${title}`, description },
  };
}

export default async function PublicPaymentPage({ params }: PageProps<"/pay/[reference]">) {
  const { reference } = await params;
  const lookup = await getPublicPayment(reference);

  switch (lookup.kind) {
    case "ok":
      return <PayView initial={lookup.payment} />;
    case "not-found":
      notFound();
    case "not-ready":
      return (
        <Shell>
          <h1 className="m-0 font-display text-2xl font-bold tracking-[-0.02em]">Not ready for payments yet</h1>
          <p className="m-0 text-[15px] leading-relaxed text-ink-soft">
            Whoever sent you this link is still setting up their account, so it cannot take a payment right now. Please check back a little
            later, or ask them to confirm.
          </p>
        </Shell>
      );
    case "down":
      return <ApiDown />;
  }
}

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <div className="relative isolate flex min-h-dvh items-center justify-center px-4">
      <Backdrop fixed />
      <Glass size="xl" className="flex w-full max-w-md flex-col gap-4 p-7">
        <JarMark className="h-9 w-9 text-ink" />
        {children}
      </Glass>
    </div>
  );
}
