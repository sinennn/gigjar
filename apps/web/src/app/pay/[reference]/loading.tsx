import { Backdrop } from "@/components/backdrop";
import { JarMark } from "@/components/jar-mark";

/** Shown while the payment details load, which can take a while if the server was asleep. */
export default function LoadingPayment() {
  return (
    <div className="relative isolate flex min-h-dvh flex-col items-center justify-center gap-4 px-4 text-center">
      <Backdrop fixed />
      <JarMark className="h-14 w-14 text-ink motion-safe:animate-pulse" />
      <p role="status" className="m-0 text-ink-soft">
        Getting the payment details ready…
      </p>
    </div>
  );
}
