import "server-only";
import { cache } from "react";
import { cookies } from "next/headers";
import type { Schemas } from "./client";

export function apiOrigin(): string {
  return (process.env.GIGJAR_API_ORIGIN ?? "http://localhost:8090").replace(/\/+$/, "");
}

/**
 * Calls the Go API from a server component, forwarding the visitor's session cookie.
 * Returns the response untouched so callers decide what a 401 means.
 */
export async function apiFetchServer(path: string, init: RequestInit = {}): Promise<Response> {
  const cookieHeader = (await cookies()).toString();
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (cookieHeader) {
    headers.set("Cookie", cookieHeader);
  }
  return fetch(`${apiOrigin()}${path}`, { ...init, headers, cache: "no-store", redirect: "manual" });
}

/**
 * The signed-in user, or null when there is no valid session. Any other failure (API down,
 * cold start timing out) throws, so an outage is never mistaken for "signed out".
 */
export const getCurrentUser = cache(async (): Promise<Schemas["User"] | null> => {
  const response = await apiFetchServer("/v1/me");
  if (response.status === 401) {
    return null;
  }
  if (!response.ok) {
    throw new Error(`could not load your session (API status ${response.status})`);
  }
  return (await response.json()) as Schemas["User"];
});
