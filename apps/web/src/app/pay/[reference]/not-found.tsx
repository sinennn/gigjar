import { Backdrop } from "@/components/backdrop";
import { JarMark } from "@/components/jar-mark";
import { ButtonLink } from "@/components/ui/button";
import { Glass } from "@/components/ui/glass";

export const metadata = { title: "Link not found", robots: { index: false } };

/** A payment link that does not exist: mistyped, cut off when it was pasted, or never made. */
export default function PaymentLinkNotFound() {
  return (
    <div className="relative isolate flex min-h-dvh items-center justify-center px-4">
      <Backdrop fixed />
      <Glass size="xl" className="flex w-full max-w-md flex-col gap-4 p-7">
        <JarMark className="h-9 w-9 text-ink" />
        <h1 className="m-0 font-display text-2xl font-bold tracking-[-0.02em]">We could not find that payment link</h1>
        <p className="m-0 text-[15px] leading-relaxed text-ink-soft">
          The link may have been cut off when it was copied, or typed wrongly. Please ask the person who sent it to send it again. Do not
          send any money until you have the right link.
        </p>
        <div>
          <ButtonLink href="/" variant="secondary">
            About GigJar
          </ButtonLink>
        </div>
      </Glass>
    </div>
  );
}
