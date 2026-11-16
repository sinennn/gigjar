"use client";

import Link from "next/link";
import { Backdrop } from "@/components/backdrop";
import { Alert } from "@/components/ui/alert";
import { Button, ButtonLink } from "@/components/ui/button";
import { Glass } from "@/components/ui/glass";

/** Anything outside the signed-in app (landing, log in, register, payment pages) that throws ends here. */
export default function RootError({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <div className="relative flex min-h-dvh items-center justify-center bg-cream px-4">
      <Backdrop fixed />
      <Glass size="xl" className="flex w-full max-w-md flex-col gap-5 p-7">
        <Link href="/" className="font-display text-lg font-bold tracking-[-0.02em] text-ink no-underline">
          GigJar
        </Link>
        <h1 className="m-0 font-display text-2xl font-bold tracking-[-0.02em]">That page did not load</h1>
        <Alert tone="warning">Something went wrong on our side, or our server is still waking up. Please try again in a moment.</Alert>
        <div className="flex flex-wrap gap-3">
          <Button onClick={reset}>Try again</Button>
          <ButtonLink href="/" variant="secondary">
            Go home
          </ButtonLink>
        </div>
      </Glass>
    </div>
  );
}
