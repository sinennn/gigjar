/**
 * Talking to the Go API from the website's server side (the /api proxy and server components).
 *
 * The API runs on a host that sleeps when idle and is replaced during deploys. While it wakes or
 * rolls over, the host's own edge answers with an HTML 502, 503 or 504 (for example
 * `x-render-routing: no-deploy`) and the app never sees the request. Passing that straight to the
 * browser shows people "request failed with status 502" on a perfectly healthy account, so these
 * answers are waited out and retried instead.
 */

/** What the browser sees when the API cannot be reached after waiting. Matches the API's own error body. */
export const UNAVAILABLE_BODY = {
  error: {
    code: "api_unavailable",
    message: "GigJar's server is waking up. Give it a few seconds and try again.",
  },
} as const;

export function unavailableResponse(): Response {
  return Response.json(UNAVAILABLE_BODY, {
    status: 503,
    headers: { "Retry-After": "10", "Cache-Control": "no-store" },
  });
}

/**
 * True for a gateway error that came from the host's edge, not from the API. The API answers its own
 * errors as JSON (including a deliberate 503 such as `jar_not_ready`), so a JSON body means "the API
 * spoke" and must never be retried or hidden.
 */
export function isGatewayFailure(response: Response): boolean {
  if (response.status !== 502 && response.status !== 503 && response.status !== 504) {
    return false;
  }
  return !(response.headers.get("content-type") ?? "").toLowerCase().includes("json");
}

const BACKOFF_MS = [1000, 2000, 3000, 4000, 5000];
const MAX_THROWN_ATTEMPTS = 3;

const sleep = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));

type Options = {
  /** Total time to keep trying while the host answers with a gateway error. */
  deadlineMs?: number;
  /**
   * Retry when the connection itself fails (no response at all). Only safe for requests that can be
   * repeated: a dropped connection may have reached the API after all. A gateway error response is
   * different, since the API provably never saw it, so those are retried for every method.
   */
  retryConnectionErrors?: boolean;
};

/**
 * Fetches `url`, waiting out a sleeping or restarting API. Resolves with the first response that is
 * not a gateway failure. If the deadline passes it resolves with the last gateway response, or
 * rejects with the last connection error when there was never a response. The caller turns either
 * into something a person can read.
 */
export async function fetchWaitingForApi(url: string, init: RequestInit, options: Options = {}): Promise<Response> {
  const { deadlineMs = 45_000, retryConnectionErrors = true } = options;
  const started = Date.now();
  let attempt = 0;
  let connectionErrors = 0;
  let lastResponse: Response | null = null;
  let lastError: unknown = null;

  for (;;) {
    const remaining = deadlineMs - (Date.now() - started);
    try {
      const response = await fetch(url, { ...init, signal: AbortSignal.timeout(Math.max(remaining, 3000)) });
      if (!isGatewayFailure(response)) {
        return response;
      }
      await lastResponse?.body?.cancel();
      lastResponse = response;
      lastError = null;
    } catch (error) {
      connectionErrors += 1;
      lastError = error;
      if (!retryConnectionErrors || connectionErrors >= MAX_THROWN_ATTEMPTS) {
        throw error;
      }
    }

    // Wait a little longer each time, but never past the deadline: the last wait is shortened so
    // there is still one final try right at the end of the budget.
    const left = deadlineMs - (Date.now() - started);
    if (left <= 1000) {
      if (lastResponse) {
        return lastResponse;
      }
      throw lastError;
    }
    const wait = Math.min(BACKOFF_MS[Math.min(attempt, BACKOFF_MS.length - 1)], left - 500);
    attempt += 1;
    await sleep(wait);
  }
}
