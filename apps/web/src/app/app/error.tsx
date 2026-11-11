"use client";

import { Backdrop } from "@/components/backdrop";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Glass } from "@/components/ui/glass";

/** The session check or a screen threw (API down, timed out). Never mistaken for "signed out". */
export default function AppError({ reset }: { error: Error; reset: () => void }) {
  return (
    <div className="relative flex min-h-dvh items-center justify-center px-4">
      <Backdrop fixed />
      <Glass size="xl" className="w-full max-w-md space-y-5 p-7">
        <h1 className="m-0 font-display text-2xl font-bold tracking-[-0.02em]">We can&apos;t reach your jar right now</h1>
        <Alert tone="warning">The server did not answer. Your money is safe. Please try again in a moment.</Alert>
        <Button onClick={reset}>Try again</Button>
      </Glass>
    </div>
  );
}
