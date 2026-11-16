import { fetchWaitingForApi, isGatewayFailure, unavailableResponse } from "@/lib/api/upstream";

const forwardedRequestHeaders = [
  "accept",
  "accept-language",
  "content-type",
  "cookie",
  "idempotency-key",
  "origin",
  "referer",
  "sec-fetch-site",
  "user-agent",
  "x-request-id",
];

const droppedResponseHeaders = new Set(["connection", "content-encoding", "content-length", "keep-alive", "set-cookie", "transfer-encoding"]);

function apiOrigin(): string {
  return (process.env.GIGJAR_API_ORIGIN ?? "http://localhost:8090").replace(/\/+$/, "");
}

async function proxy(request: Request, context: RouteContext<"/api/[...path]">): Promise<Response> {
  const { path } = await context.params;
  const incoming = new URL(request.url);
  const target = `${apiOrigin()}/${path.map(encodeURIComponent).join("/")}${incoming.search}`;

  const headers = new Headers();
  for (const name of forwardedRequestHeaders) {
    const value = request.headers.get(name);
    if (value) {
      headers.set(name, value);
    }
  }
  headers.set("x-forwarded-host", incoming.host);
  headers.set("x-forwarded-proto", incoming.protocol.replace(":", ""));

  const hasBody = request.method !== "GET" && request.method !== "HEAD";
  const repeatable = request.method === "GET" || request.method === "HEAD" || request.method === "OPTIONS";
  let upstream: Response;
  try {
    // The API sleeps when idle and is replaced on deploys, so its host's edge answers a gateway error
    // for a while. Wait that out (up to 45 s) rather than showing "request failed with status 502".
    // A dropped connection is only retried for requests that are safe to repeat.
    upstream = await fetchWaitingForApi(
      target,
      {
        method: request.method,
        headers,
        body: hasBody ? await request.arrayBuffer() : undefined,
        redirect: "manual",
        cache: "no-store",
      },
      { retryConnectionErrors: repeatable },
    );
  } catch {
    return unavailableResponse();
  }
  if (isGatewayFailure(upstream)) {
    await upstream.body?.cancel();
    return unavailableResponse();
  }

  const responseHeaders = new Headers();
  upstream.headers.forEach((value, name) => {
    if (!droppedResponseHeaders.has(name.toLowerCase())) {
      responseHeaders.set(name, value);
    }
  });
  for (const cookie of upstream.headers.getSetCookie()) {
    responseHeaders.append("set-cookie", cookie);
  }
  const bodyless = request.method === "HEAD" || [101, 204, 205, 304].includes(upstream.status);
  return new Response(bodyless ? null : upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers: responseHeaders,
  });
}

export const dynamic = "force-dynamic";

export { proxy as DELETE, proxy as GET, proxy as HEAD, proxy as OPTIONS, proxy as PATCH, proxy as POST, proxy as PUT };
