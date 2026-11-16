import "server-only";
import { cache } from "react";
import { interpretPublicResponse, type PublicLookup } from "./public-lookup";
import { apiOrigin } from "./server";
import { fetchWaitingForApi } from "./upstream";

export type { PublicLookup, PublicPayment } from "./public-lookup";

/**
 * `GET /v1/public/pay/{reference}`: what a client sees. No login and no private data. It is read on
 * the server so the page arrives complete, and cached per request so the page and its metadata share
 * one call. A sleeping API is waited out first, and only then reported as "down".
 */
export const getPublicPayment = cache(async (reference: string): Promise<PublicLookup> => {
  try {
    const response = await fetchWaitingForApi(
      `${apiOrigin()}/v1/public/pay/${encodeURIComponent(reference)}`,
      { headers: { Accept: "application/json" }, cache: "no-store" },
      { deadlineMs: 30_000 },
    );
    return await interpretPublicResponse(response);
  } catch {
    return { kind: "down" };
  }
});
