import type { components } from "./schema";

export type Schemas = components["schemas"];
export type ApiErrorBody = Schemas["ErrorBody"];

/**
 * Words for a failure that came without the API's own error body (a proxy or host error page).
 * Nobody should read "request failed with status 502".
 */
function plainMessage(status: number): string {
  if (status === 502 || status === 503 || status === 504) {
    return "GigJar's server is waking up. Give it a few seconds and try again.";
  }
  if (status >= 500) {
    return "Something went wrong on our side. Please try again in a moment.";
  }
  if (status === 429) {
    return "Too many requests. Please wait a moment and try again.";
  }
  return "That did not work. Please try again.";
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string | undefined;
  readonly fields: Record<string, string>;

  constructor(status: number, body: ApiErrorBody | null) {
    super(body?.error.message ?? plainMessage(status));
    this.name = "ApiError";
    this.status = status;
    this.code = body?.error.code ?? (status >= 500 ? "api_unavailable" : "unknown_error");
    this.requestId = body?.error.request_id;
    this.fields = body?.error.fields ?? {};
  }
}

export const API_BASE_PATH = "/api";

type RequestOptions = {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  idempotencyKey?: string;
  signal?: AbortSignal;
};

export async function apiRequest<TResponse>(path: string, options: RequestOptions = {}): Promise<TResponse> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (options.idempotencyKey) {
    headers["Idempotency-Key"] = options.idempotencyKey;
  }

  const response = await fetch(`${API_BASE_PATH}${path}`, {
    method: options.method ?? "GET",
    headers,
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    credentials: "same-origin",
    cache: "no-store",
    signal: options.signal,
  });

  if (response.status === 204) {
    return undefined as TResponse;
  }
  const payload: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    throw new ApiError(response.status, isErrorBody(payload) ? payload : null);
  }
  return payload as TResponse;
}

export type FormError = { message: string; fields: Record<string, string> };

/** Turns anything a failed request can throw into text a form can show. */
export function describeError(error: unknown): FormError {
  if (error instanceof ApiError) {
    return { message: error.message, fields: error.fields };
  }
  return { message: "We couldn't reach the server. Check your connection and try again.", fields: {} };
}

function isErrorBody(value: unknown): value is ApiErrorBody {
  return (
    typeof value === "object" &&
    value !== null &&
    "error" in value &&
    typeof (value as { error: unknown }).error === "object"
  );
}
