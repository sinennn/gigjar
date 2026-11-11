# GigJar web

Next.js 16 (App Router, Cache Components), TypeScript and Tailwind v4. Read `AGENTS.md`: this Next version differs from older docs, and the bundled guides are in `node_modules/next/dist/docs/`.

## Running

```bash
pnpm install
pnpm dev            # http://localhost:3000
```

The browser only ever calls `/api/*`. `next.config.ts` rewrites that path to the Go API (`GIGJAR_API_ORIGIN`, default `http://localhost:8090`). Requests are then same-origin, so the session cookie (`gigjar_session`, httpOnly) works without CORS.

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

Every operation in `openapi.yaml` is `implemented`, including `GET /v1/settlements` and `settlement_fee` on the jar, so build against the real API. Run `pnpm contracts` after pulling to refresh the generated types.

## Testing payments locally

`make up` runs Postgres, the API and the watcher on the **simulated** network. Every jar gets a fake address at sign-up, and no real funds exist. To make a "client" pay a link:

```bash
curl -X POST localhost:8090/v1/simulator/transfers -H 'Content-Type: application/json' \
  -d '{"reference":"<link reference>","amount":"4","confirmation_delay_seconds":20}'
```

- For the first ~20 seconds the jar shows it under `balances.pending`, `GET /v1/incoming-transfers` lists it as `pending`, and the public pay page returns `pending_payment_observed: true`.
- Then the watcher confirms it. It moves to `balances.available`, the link becomes `paid` with `amount_received`, and a `deposit` appears in `GET /v1/transactions`.
- Use `"to_address"` instead of `"reference"` for a payment that matches no link. Use `"outcome":"reverted"` to see a `failed` transfer.
- Once a student with a Cleva address reaches $12 available, the sweeper starts a sweep. `balances.settling` shows the amount, and `GET /v1/settlements` shows it go `queued` → `broadcast` → `confirmed` (about 10s on the simulated network). After that, `settled` grows by the amount minus `settlement_fee`.

The simulator route exists only on the simulated network. Never build UI that depends on it.

## Design direction

Warm, minimal and approachable, not a crypto exchange. The palette and type come from the pitch deck: ink navy, cream background, jar green for money that has arrived, lid orange for thresholds and warnings, and Space Grotesk headings over Inter. The tokens are in `src/app/globals.css`, and the full guide (Liquid Glass, motion, the 3D jar, copy rules) is in [DESIGN.md](DESIGN.md). Two rules:

- **Simulated network.** Whenever the API returns `network: "simulated"`, show a persistent, unmistakable "Simulated" badge.
- **TRC20 warning.** Every place a pay-to address appears must say "USDT on TRON (TRC20) only".
