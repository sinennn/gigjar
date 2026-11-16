# Frontend build plan

The web app lives in `apps/web` (Next.js 16, Tailwind v4, GSAP, three.js). This plan breaks the work into phases. Tick a box when the work is merged to `main`, and add a short note under a task if it is only partly done.

The design rules for every phase are in `apps/web/DESIGN.md`. The API shape is `packages/contracts/openapi.yaml`; every operation in it is implemented, so build against the real API and run `pnpm contracts` after pulling to refresh the types.

Last updated 2026-11-15.

## Where things stand

| Phase | What | State |
| --- | --- | --- |
| 0 | Foundations | Done |
| 1 | Design system and landing page | Done |
| 2 | Authentication | Done, checked against a real API |
| 3 | App shell and the nine signed-in screens | Done, checked with real data |
| 4 | Public payment page | Done, checked with real data (per-link share image still open) |
| 5 | End-to-end check on the simulated network | Not started |
| 6 | AI features | Not started (API is ready) |
| 7 | Polish: loading, empty and error states, accessibility | Not started |
| 8 | Automated tests | Not started |
| 9 | Release and operations | Partly done |

Do phases 4 and 5 next. A student can already make a link, but a client cannot yet pay it, and nothing signed in has been seen working with real data.

## Phase 0: Foundations

- [x] Next.js 16 app with TypeScript, Tailwind v4 and ESLint
- [x] API types generated from the contract (`pnpm contracts`)
- [x] `apiRequest` client and `ApiError`, with field errors from the error envelope
- [x] Same-origin `/api` proxy so the session cookie stays first-party
- [x] Money helpers: USDT amounts are strings with six decimals, never `number`
- [x] Cloudflare Workers build through Open Next, served at https://gigjar.pages.dev
- [x] Prerendered pages served from the static-assets cache (fixed the 500s)
- [x] A sleeping or restarting API is waited out instead of showing "request failed with status 502": the proxy and the session check retry gateway errors, messages are plain words, forms say "still working", and the app shows a "GigJar is waking up" screen

## Phase 1: Design system and landing page

- [x] Original palette and type (cream, ink navy, jar green, lid orange; Space Grotesk and Inter)
- [x] Light Liquid Glass: `glass` utility, edge refraction in Chromium, plain frosted glass elsewhere
- [x] Landing page at `/`: hero, how it works, the $1 fee, footer
- [x] 3D coin jar (three.js and Rapier physics) with the pour, payments, payout and refill loop
- [x] Jar always turns; only a drag changes direction
- [x] GSAP intro and scroll reveals, with reduced motion honoured
- [x] Page icon (SVG, `.ico`, Apple touch icon)
- [x] Link preview card (1200x630, under 300 KB for WhatsApp) with Open Graph and Twitter text
- [x] `DESIGN.md` written and linked from the web README
- [ ] Check the landing page on Safari and Firefox (no edge refraction there, so confirm the plain glass still looks right)
- [ ] Check the jar on a low-end Android phone and cut the pixel ratio or coin count if it drops frames

## Phase 2: Authentication

- [x] Log in, register, forgot password and reset password forms on the API
- [x] Field-level errors and a general error message
- [x] "Continue with Google", shown only when `GET /v1/auth/providers` says it is on, opening without a jump
- [x] Failed Google sign-in message from `?error=`
- [x] Signed-out visitors to `/app` go to `/login`
- [x] Log in, log out and reset a password against a running API and confirm the session cookie works
- [x] After login, honour a `return_to` so a visitor lands back where they were going (only in-app paths are accepted)
- [x] Rate limit and lockout messages (429 `too_many_attempts` on login, forgot-password limit)
- [x] Reset link with a bad or expired token (`reset_token_invalid`) shows a clear next step
- [x] Redirect signed-in visitors away from `/login` and `/register`

## Phase 3: App shell and signed-in screens

All routes are under `/app`. Old paths such as `/dashboard` and `/links` redirect there.

- [x] Glass sidebar on desktop, glass tab bar on phones, session gate
- [x] Overview (`/app`)
- [x] Jar (`/app/jar`)
- [x] Payment links list, new link and link detail
- [x] Transactions
- [x] Settlements (payouts)
- [x] Cleva wallet (save and clear the deposit address)
- [x] Settings (profile, log out)
- [x] Open every screen with a real account and fix what looks wrong, at 390px and 1440px
- [x] Overview shows the jar balance, pending, remaining to $12, recent payments and last payout from the live API
- [x] Prompt to add a Cleva address when none is set, since no payout happens without one
- [x] "Simulated" badge visible on every screen whenever `network` is `simulated`
- [x] "USDT on TRON (TRC20) only" shown everywhere a pay-to address appears in the app (the public page is phase 4)
- [x] Read the threshold and fee from the API (`settlement_threshold`, `settlement_fee`), never hard-code them
- [x] Money always shown through `formatUsdt`; no screen converts an amount to `number`
- [x] New link: warn that a client must send the exact amount. The API attributes a transfer to the oldest open link with that exact amount, so two open links with the same amount are ambiguous
- [x] Link detail: share actions (copy the pay URL, share sheet on phones), close a link, and its transfers
- [x] Transactions and incoming transfers: paging through `next_cursor`, with pending, confirmed and failed states
- [x] Refresh the data while something is pending (poll, pausing when the tab is hidden)

## Phase 4: Public payment page

The page a client opens from a shared link: `/pay/[reference]`, no login. It is still the placeholder `ScreenScaffold`. Fields come from `GET /v1/public/pay/{reference}`.

- [x] Fetch on the server; unknown link (404) shows a friendly not-found page
- [x] Jar not ready (503 `jar_not_ready`) shows "not ready to receive payments"
- [x] Gig title, description, amount, and the recipient's display name
- [x] Pay-to address with a copy button and a QR code
- [x] Prominent "USDT on TRON (TRC20) only; other networks are lost" next to the address
- [x] "Send exactly this amount" note, with the amount copyable
- [x] USDT contract address for verification (hidden on the simulated network, where it is null)
- [x] Live status: poll while `pending_payment_observed`, then show confirmed and paid states
- [x] Paid and closed links show a clear finished state instead of payment details
- [x] "Simulated" badge when the network is simulated
- [x] Designed for a phone first, since clients open this from a chat
- [x] Per-link share text (gig title, amount and recipient in the title and description)
- [ ] Per-link share image. The generic card is used for now. A per-link image has to bundle its fonts and artwork into the Worker instead of reading files from disk, because Cloudflare Workers have no disk

## Phase 5: End-to-end check on the simulated network

Follow "Testing payments locally" in `apps/web/README.md`. The simulator route is for testing only; no UI may depend on it.

- [ ] Run `make up` (Postgres, API, watcher on the simulated network) and the web app together
- [ ] Register, log in, add a Cleva address
- [ ] Create a link and open its public page in a second browser
- [ ] Pay it with the simulator: see pending, then confirmed, on the jar, the link, the transactions and the public page
- [ ] Reach $12 available and watch the sweep go queued, broadcast, confirmed; check the $1 fee and the settled total
- [ ] Failed transfer (`outcome: reverted`) shows as failed and does not change the balance
- [ ] A payment that matches no link shows as unattributed
- [ ] Write the steps down as a short checklist someone else can repeat

## Phase 6: AI features

The API is ready (`/v1/ai/*`); the web app does not use it yet. Show these only when `GET /v1/auth/providers` says `ai` is true, and handle 429 and 503 gracefully.

- [ ] Link draft: describe a gig on the new link form and fill in title, description and a suggested price (`POST /v1/ai/link-draft`), editable before saving
- [ ] Earnings insights: a short plain summary on the overview (`GET /v1/ai/insights`), cached by the API for ten minutes
- [ ] Assistant: ask about pending payments, sweeps and fees (`POST /v1/ai/assistant`); it only answers and can never move money
- [ ] Clear wording that AI text is generated, and that the numbers come from the ledger

## Phase 7: Polish

- [ ] Loading skeletons for every screen (`loading.tsx` or in-page), so nothing is blank while the API wakes up
- [ ] Empty states: no links yet, no payments yet, no payouts yet, each with a next step
- [ ] Error states with a retry, and the request id shown for support
- [ ] A branded site-wide 404 page (done so far: the payment link 404, a root error page, and the "waking up" screens)
- [ ] Keyboard and screen reader pass on every screen: focus order, labels, live regions for status changes
- [ ] Contrast pass on glass over the coloured shapes behind it
- [ ] Reduced motion pass: nothing important depends on animation
- [ ] Responsive pass at 320, 390, 768 and 1440px with no sideways scroll
- [ ] Copy pass: short, plain, no em dashes
- [ ] Performance pass: the three.js and Rapier code stays lazy and off the signed-in screens

## Phase 8: Automated tests

- [ ] Unit tests for money and format helpers (decimal strings, rounding, no float maths)
- [ ] Component tests for forms: field errors, busy state, success path
- [ ] A scripted end-to-end smoke test over Chrome DevTools Protocol against the simulated network (log in, create a link, pay it, see it confirmed). Do not use Playwright
- [ ] Run typecheck, lint, unit tests and a production build on every pull request

## Phase 9: Release and operations

- [x] Deploys by hand with `pnpm cf:deploy` in `apps/web`
- [ ] Redeploy so the 500 fixes reach https://gigjar.pages.dev (needs a machine that can run the Open Next build; this Windows PC cannot)
- [ ] Continuous integration: typecheck, lint and build on pull requests
- [ ] Automated deploy from CI on Linux, so a release no longer depends on one person's machine
- [ ] After each deploy, a smoke check that `/`, `/login`, `/register`, `/app` (redirect), `/opengraph-image` and a public pay page all respond
- [ ] Set `NEXT_PUBLIC_SITE_URL` if the public address changes from gigjar.pages.dev
- [ ] Error monitoring for the browser and the Worker
- [ ] Refresh stale docs: the table in `apps/web/README.md` still lists the old `(app)/dashboard` routes, and `docs/architecture.md` says the payout point is about $11 (it is $12 with a $1 fee)

## Not planned for now

- Dark theme (the design guide says none for now)
- Other languages
- Anything that moves money from the web app beyond what the contract offers
