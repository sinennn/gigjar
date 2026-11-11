"use client";

import { createContext, useContext, useMemo, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { useApi } from "@/lib/api/hooks";

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
  return (
    <UserContext value={user}>
      <JarContext value={value}>{children}</JarContext>
    </UserContext>
  );
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
