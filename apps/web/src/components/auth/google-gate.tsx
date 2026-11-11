"use client";

import { useEffect, useState } from "react";
import { GoogleSignInButton } from "@/components/google-sign-in";
import { apiRequest } from "@/lib/api/client";

// GET /v1/auth/providers (the contract defines this response inline, so it has no named schema).
type AuthProviders = { google: boolean; ai: boolean };

/**
 * Shows "Continue with Google" only when the API reports it is configured. It loads in the
 * browser so the form renders at once even while a sleeping API instance wakes up.
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

  if (!enabled) {
    return null;
  }
  return (
    <div className="space-y-4">
      <GoogleSignInButton />
      <div className="flex items-center gap-3 text-xs text-muted">
        <span className="h-px flex-1 bg-line" />
        {divider}
        <span className="h-px flex-1 bg-line" />
      </div>
    </div>
  );
}
