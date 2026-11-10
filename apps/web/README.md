# GigJar web

Next.js 16 (App Router, Cache Components), TypeScript and Tailwind v4. Read `AGENTS.md`: this Next version differs from older docs, and the bundled guides are in `node_modules/next/dist/docs/`.

## Running

```bash
pnpm install
pnpm dev            # http://localhost:3000
```

The browser only ever calls `/api/*`. `next.config.ts` rewrites that path to the Go API (`GIGJAR_API_ORIGIN`, default `http://localhost:8090`). Requests are then same-origin, so the session cookie planned for Phase 2 (`gigjar_session`, httpOnly) works without CORS.

## What is here

| Path | Purpose |
|---|---|
| `src/app/(auth)/login`, `register` | Auth screens |
| `src/app/(app)/dashboard` … `settings` | Signed-in screens, sharing the sidebar layout in `(app)/layout.tsx` |
| `src/app/pay/[reference]` | Public payment page clients open from a shared link |
| `src/components/screen-scaffold.tsx` | Temporary panel on every screen listing what it must show and which contract operations feed it. Replace it screen by screen. |
| `src/lib/api/client.ts` | `apiRequest` wrapper and `ApiError` (maps the `{ error: { code, message, request_id, fields } }` envelope) |
| `src/lib/api/schema.d.ts` | Generated from `packages/contracts/openapi.yaml` with `pnpm contracts`. Do not edit by hand. |
| `src/lib/money.ts` | USDT amounts are decimal strings with six fractional digits. Format them with `formatUsdt` and never convert to `number`. |

## Contract status

Every operation in `openapi.yaml` has `x-status`. Only `/healthz` and `/readyz` are `implemented` today. The rest are the agreed shapes, so you can build against them with mocks now.

## Design direction

Warm, minimal and approachable, not a crypto exchange. The palette and type come from the pitch deck: ink navy, cream background, jar green for money that has arrived, lid orange for thresholds and warnings, and Space Grotesk headings over Inter. The tokens are in `src/app/globals.css`. Two rules:

- **Simulated network.** Whenever the API returns `network: "simulated"`, show a persistent, unmistakable "Simulated" badge.
- **TRC20 warning.** Every place a pay-to address appears must say "USDT on TRON (TRC20) only".
