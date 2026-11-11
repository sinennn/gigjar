import type { components } from "./schema";

export type Schemas = components["schemas"];
export type ApiErrorBody = Schemas["ErrorBody"];

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string | undefined;
  readonly fields: Record<string, string>;

  constructor(status: number, body: ApiErrorBody | null) {
    super(body?.error.message ?? `request failed with status ${status}`);
    this.name = "ApiError";
    this.status = status;
    this.code = body?.error.code ?? "unknown_error";
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
