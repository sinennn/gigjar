/**
 * Where to send someone after they log in. A visitor bounced from /app/links to the login page
 * comes back to /app/links. The value arrives in a query string, so it is untrusted: only paths
 * inside the signed-in app are accepted, which rules out other sites ("//evil.example",
 * "https://evil.example") and anything that is not the app.
 */
export const DEFAULT_RETURN = "/app";

export function safeReturnTo(raw: string | string[] | null | undefined): string {
  const value = Array.isArray(raw) ? raw[0] : raw;
  if (!value || value.length > 300) {
    return DEFAULT_RETURN;
  }
  const insideApp = value === "/app" || /^\/app[/?#]/.test(value);
  const sneaky = value.includes("\\") || value.includes("//") || /[\u0000-\u001f]/.test(value);
  return insideApp && !sneaky ? value : DEFAULT_RETURN;
}

/** The login page address that brings someone back to `path` afterwards. */
export function loginUrlFor(path: string): string {
  const back = safeReturnTo(path);
  return back === DEFAULT_RETURN ? "/login" : `/login?return_to=${encodeURIComponent(back)}`;
}
