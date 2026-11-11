"use client";

import { useEffect, useState } from "react";
import { GoogleSignInButton } from "@/components/google-sign-in";
import { apiRequest } from "@/lib/api/client";

// GET /v1/auth/providers (the contract defines this response inline, so it has no named schema).
type AuthProviders = { google: boolean; ai: boolean };

/**
 * Shows "Continue with Google" only when the API reports it is configured. It loads in the
 * browser so the form renders at once even while a sleeping API instance wakes up.
 *
 * The answer arrives after the page has drawn, so the button opens smoothly (height and fade)
 * instead of popping in and shoving the form down. It also owns its bottom gap
 * (`mb-0` overrides the parent's `space-y`), so nothing is left behind while it is closed.
 */
export function GoogleGate({ divider }: { divider: string }) {
  const [enabled, setEnabled] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    apiRequest<AuthProviders>("/v1/auth/providers", { signal: controller.signal })
      .then((providers) => setEnabled(providers.google))
      .catch(() => setEnabled(false));
    return () => controller.abort();
  }, []);

  return (
    <div
      inert={!enabled}
      aria-hidden={!enabled || undefined}
      className={`mb-0 grid transition-[grid-template-rows,opacity] duration-500 ease-out motion-reduce:transition-none ${
        enabled ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0"
      }`}
    >
      {/* Room around the button so the clip never cuts its shadow, without narrowing it. */}
      <div className="-mx-3 -mt-3 min-h-0 overflow-hidden px-3 pt-3">
        <div className="space-y-4 pb-6">
          <GoogleSignInButton />
          <div className="flex items-center gap-3 text-xs text-muted">
            <span className="h-px flex-1 bg-line" />
            {divider}
            <span className="h-px flex-1 bg-line" />
          </div>
        </div>
      </div>
    </div>
  );
}
