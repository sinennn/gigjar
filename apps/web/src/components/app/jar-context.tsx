"use client";

import { createContext, useContext, useMemo, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { useApi, type Page } from "@/lib/api/hooks";

export type Jar = Schemas["Jar"];
export type User = Schemas["User"];

type JarValue = {
  jar: Jar | null;
  error: Error | null;
  loading: boolean;
  reload: () => void;
};

const JarContext = createContext<JarValue | null>(null);
const UserContext = createContext<User | null>(null);

/**
 * One shared, polled copy of `GET /v1/jar` for the whole signed-in app. The shell reads the
 * network from it (for the persistent Simulated badge) and the overview, jar and wallet screens
 * read the balances and the settlement threshold and fee, so nothing hard-codes them.
 */
export function AppDataProvider({ user, children }: { user: User; children: ReactNode }) {
  const { data, error, loading, reload } = useApi<Jar>("/v1/jar", { pollMs: 10_000 });
  const value = useMemo(() => ({ jar: data, error, loading, reload }), [data, error, loading, reload]);

  // Payments only carry the id of the link they were matched to. A shared, slowly refreshed
  // lookup lets every list say "Flyer design" instead of a wallet address, without each screen
  // fetching its own copy. Newest 100 links and transfers are enough for the rows people look at.
  const links = useApi<Page<Schemas["PaymentLink"]>>("/v1/payment-links?limit=100", { pollMs: 30_000 });
  const transfers = useApi<Page<Schemas["IncomingTransfer"]>>("/v1/incoming-transfers?limit=100", { pollMs: 30_000 });
  const gigs = useMemo<GigLookup>(() => {
    const titles = new Map((links.data?.items ?? []).map((link) => [link.id, link.title]));
    const transferLink = new Map((transfers.data?.items ?? []).map((transfer) => [transfer.id, transfer.payment_link_id]));
    return {
      titleOf: (linkId) => (linkId ? (titles.get(linkId) ?? null) : null),
      titleOfTransfer: (transferId) => {
        const linkId = transferId ? transferLink.get(transferId) : null;
        return linkId ? (titles.get(linkId) ?? null) : null;
      },
    };
  }, [links.data, transfers.data]);

  return (
    <UserContext value={user}>
      <JarContext value={value}>
        <GigContext value={gigs}>{children}</GigContext>
      </JarContext>
    </UserContext>
  );
}

type GigLookup = {
  /** Title of a payment link, or null when it is unknown (or not loaded yet). */
  titleOf: (linkId: string | null | undefined) => string | null;
  /** Title of the gig an incoming transfer was matched to, or null. */
  titleOfTransfer: (transferId: string | null | undefined) => string | null;
};

const GigContext = createContext<GigLookup>({ titleOf: () => null, titleOfTransfer: () => null });

export function useGigs(): GigLookup {
  return useContext(GigContext);
}

export function useJar(): JarValue {
  const value = useContext(JarContext);
  if (!value) {
    throw new Error("useJar must be used inside AppDataProvider");
  }
  return value;
}

export function useUser(): User {
  const value = useContext(UserContext);
  if (!value) {
    throw new Error("useUser must be used inside AppDataProvider");
  }
  return value;
}
