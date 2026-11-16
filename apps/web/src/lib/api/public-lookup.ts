import type { Schemas } from "./client";
import { isGatewayFailure } from "./upstream";

export type PublicPayment = Schemas["PublicPaymentInstructions"];

/** Every way looking up a payment link can end, so the page has one branch for each. */
export type PublicLookup =
  | { kind: "ok"; payment: PublicPayment }
  | { kind: "not-found" }
  | { kind: "not-ready" }
  | { kind: "down" };

/**
 * Turns the API's answer to `GET /v1/public/pay/{reference}` into one of the four outcomes.
 * A host error page (a sleeping or restarting API) counts as "down", never as "not found": telling
 * someone their link does not exist because our server was asleep would be wrong and alarming.
 */
export async function interpretPublicResponse(response: Response): Promise<PublicLookup> {
  if (response.ok) {
    return { kind: "ok", payment: (await response.json()) as PublicPayment };
  }
  if (isGatewayFailure(response)) {
    await response.body?.cancel();
    return { kind: "down" };
  }

  const body = (await response.json().catch(() => null)) as { error?: { code?: string } } | null;
  if (response.status === 503 && body?.error?.code === "jar_not_ready") {
    return { kind: "not-ready" };
  }
  if (response.status === 404 || response.status === 400 || response.status === 422) {
    return { kind: "not-found" };
  }
  return { kind: "down" };
}
