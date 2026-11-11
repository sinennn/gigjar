# GigJar web design guide

This is the agreed look for the whole web app. The landing page at `/` (`src/app/page.tsx`) is the reference build: when in doubt, match it.

## Direction

Warm, minimal, approachable. Not a crypto exchange, not neon, nothing that reads as "AI made this". Light Liquid Glass in the spirit of Apple's: clear, slightly milky panes with a bright rim, soft lift and edge refraction, over the warm cream page.

## Colour (tokens in `src/app/globals.css`)

| Token | Hex | Use |
| --- | --- | --- |
| `cream` | `#f6f3ec` | Page background everywhere |
| `paper` | `#fffdf8` | Text on dark buttons, solid surfaces |
| `ink` | `#13263a` | Headings, body text, primary buttons |
| `ink-soft` | `#3d4f63` | Secondary text |
| `muted` | `#6b7684` | Hints, footers, captions |
| `line` | `#e3ddcf` | Hairlines on solid surfaces |
| `jar-green` / `-soft` | `#219e7a` / `#d7efe6` | Money that has arrived, success, progress |
| `lid-orange` / `-soft` | `#e8922f` / `#fbe3c7` | Thresholds (the $12 line) and warnings |
| `danger` / `-soft` | `#b4432f` / `#f7ddd6` | Errors |

No other hues. No dark theme for now. No gradients as decoration except the very soft green/orange radial shapes behind glass (`Backdrop` in `page.tsx`) so the glass has something to bend.

## Type

Space Grotesk (`font-display`, bold, tight tracking around `-0.03em`) for headings and big numbers; Inter (`font-sans`) for everything else. Money uses `tabular-nums`. Both fonts are loaded in `src/app/layout.tsx`.

## Glass

- `glass` utility: the pane. The element must be `relative` or `absolute` (the rim is a `::before`).
- `glass-dense`: add on glass that carries text over busy content (cards over the jar, over lists).
- `data-lens="<px>"` on a glass element turns on edge refraction (`src/lib/liquid-glass.ts`, Chromium only, desktop with a mouse only; others keep plain frosted glass). Use 14 to 18 for pills and chips, 22 to 26 for cards. `LandingMotion` applies it to every `[data-lens]` inside it; app pages need the same hook (see Next steps).
- Use glass for floating things: nav, cards, chips, panels, dialogs, the secondary button. Do not put glass on glass more than one level deep.
- Shapes are round: pills (`rounded-full`) for buttons and nav, `rounded-[20px]` to `rounded-[36px]` for cards.

## Buttons and controls

- Primary: ink pill, paper text (`bg-ink text-paper hover:bg-ink-soft`), min height 52px (44px in the nav).
- Secondary: glass pill with ink text.
- Every touch target is at least 44px. Focus is visible (`outline-2 outline-offset-4 outline-ink`).

## Motion (GSAP 3.15 + `@gsap/react`)

- Headlines rise in line by line; supporting text fades up with a short stagger; sections reveal on scroll once (`ScrollTrigger.batch`).
- Easing `expo.out` for entrances, `back.out` only for small pops (chips, badges).
- Nothing follows the pointer: no magnetic buttons, no tilt.
- Always honour `prefers-reduced-motion`: everything visible and still. Elements hidden before their intro use `data-intro` (hidden only when motion is allowed).

## The jar (`src/components/landing/jar-scene.tsx`)

three.js (0.186.1) plus Rapier physics (`@dimforge/rapier3d-compat` 0.21.0), both loaded lazily on the client.

- Real glass (transmission), gold and green coins with real physics, an orange lid with a coin slot, a printed $12 line.
- Always turning slowly. Only a drag (mouse or finger) changes the direction; it keeps the momentum and eases back to a steady turn.
- Story loop: pour to $8.40, lid on, gig payments drop through the slot up to $12, then every coin streams back out through the slot ("Sent to Cleva $11.00", the $1 fee taken) and it fills from $0 again.
- Coins entering from above fade in (masked top edge), never clipped.
- Events for the UI: `jar:pour`, `jar:amount`, `jar:payment`, `jar:payout`.

## Copy

- Plain, short, human. No em dashes anywhere (UI text, metadata, docs written for users). No hype words.
- Payout is at $12 with a flat $1 fee (backend default `GIGJAR_SETTLEMENT_THRESHOLD=12`, `GIGJAR_SETTLEMENT_FEE=1`). Read real values from the API on app screens.
- The two product rules still apply: a persistent "Simulated" badge whenever the API says `network: "simulated"`, and "USDT on TRON (TRC20) only" wherever a pay-to address appears.

## Layout and responsiveness

- Content width `max-w-[1160px]`, side padding 16px (`px-4`), 24px from `sm`.
- Must work from 320px up with no sideways scroll. Stack on phones; wide tables scroll inside their own box.
- Test at 390px and 1440px.

## Next steps

- Move the signed-in app under `/app` (the user wants the app there; the landing owns `/`).
- Restyle the app shell and every screen in this language: glass sidebar or top nav (a glass tab bar on phones), glass cards, ink primary buttons, jar-green money, lid-orange threshold and warnings.
- Lift the shared pieces out of the landing into components (`Glass` card, buttons, the lens hook as a small client wrapper for app pages).
- Auth screens (`/login`, `/register`, `/forgot-password`, `/reset-password`) in the same style.
