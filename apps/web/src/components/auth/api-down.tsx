"use client";

import { useRouter } from "next/navigation";
import { useTransition } from "react";
import { Backdrop } from "@/components/backdrop";
import { Alert } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Glass } from "@/components/ui/glass";

/**
 * Shown when the API did not answer: it is asleep, restarting, or unreachable. This is never treated
 * as "signed out", so nobody is sent to the login page (or shown a raw error) for a server hiccup.
 */
export function ApiDownView({ onRetry, busy = false }: { onRetry: () => void; busy?: boolean }) {
  return (
    <div className="relative flex min-h-dvh items-center justify-center px-4">
      <Backdrop fixed />
      <Glass size="xl" className="flex w-full max-w-md flex-col gap-5 p-7">
        <h1 className="m-0 font-display text-2xl font-bold tracking-[-0.02em]">GigJar is waking up</h1>
        <Alert tone="warning">
          Our server rests when nobody is using it, and it has not answered yet. This can take up to a minute after a quiet spell. Your
          money is safe.
        </Alert>
        <Button onClick={onRetry} loading={busy}>
          Try again
        </Button>
      </Glass>
    </div>
  );
}

/** The version the app layout renders when the session check fails: a retry re-runs the server check. */
export function ApiDown() {
  const router = useRouter();
  const [busy, start] = useTransition();
  return <ApiDownView busy={busy} onRetry={() => start(() => router.refresh())} />;
}
