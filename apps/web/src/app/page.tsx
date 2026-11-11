import type { Metadata } from "next";
import Link from "next/link";
import { JarMark } from "@/components/jar-mark";
import { JarScene } from "@/components/landing/jar-scene";
import { LandingMotion } from "@/components/landing/landing-motion";

export const metadata: Metadata = {
  title: { absolute: "GigJar" },
  description:
    "Get paid in USDT for any gig, however small. Your jar collects each payment and sends it to your Cleva wallet once it reaches $12.",
};

const STEPS = [
  { n: "1", title: "Make a link", body: "Name the gig, set the price, send the link to your client." },
  {
    n: "2",
    title: "Get paid in USDT",
    body: "Your client pays on TRON. It shows in your jar once the network confirms it.",
  },
  { n: "3", title: "Payout at $12", body: "When your jar reaches $12, it goes to your Cleva wallet on its own." },
];

const POINTS = ["$1 flat fee per payout", "No minimum gig size", "USDT on TRON"];

const focusRing = "focus-visible:outline-2 focus-visible:outline-offset-4 focus-visible:outline-ink";
const primaryCta = `inline-flex min-h-[52px] items-center justify-center rounded-full bg-ink px-7 text-base font-semibold text-paper no-underline shadow-[0_10px_24px_-10px_rgb(19_38_58/0.6)] transition-colors hover:bg-ink-soft ${focusRing}`;

export default function LandingPage() {
  return (
    <LandingMotion>
      <noscript>
        <style>{".landing [data-intro]{visibility:visible}"}</style>
      </noscript>
      <div className="relative isolate min-h-dvh overflow-x-clip bg-cream text-ink antialiased selection:bg-jar-green-soft">
        <div className="mx-auto max-w-[1160px] px-4 sm:px-6">
          <Nav />

          <header
            id="top"
            className="grid items-center gap-6 pt-10 pb-12 sm:pt-14 lg:min-h-[min(calc(100dvh-88px),760px)] lg:grid-cols-[1.05fr_1fr] lg:gap-10 lg:py-10"
          >
            <div className="flex min-w-0 flex-col gap-6 sm:gap-7">
              <h1
                data-a="h1"
                data-intro
                className="m-0 font-display text-[clamp(3.25rem,8.6vw,6rem)] leading-[0.95] font-bold tracking-[-0.04em]"
              >
                <Line>Small gigs</Line>
                <Line className="text-jar-green">add up.</Line>
              </h1>
              <p data-a="fade" data-intro className="m-0 max-w-[480px] text-[17px] leading-[1.6] text-ink-soft sm:text-lg">
                Get paid in USDT for any gig, however small. Your jar collects each payment and sends it to your
                Cleva wallet once it reaches $12.
              </p>
              <div data-a="fade" data-intro className="flex flex-col gap-3 sm:flex-row">
                <Link href="/register" className={primaryCta}>
                  Start your jar
                </Link>
                <a
                  href="#how"
                  data-lens="18"
                  className={`glass relative inline-flex min-h-[52px] items-center justify-center rounded-full px-7 text-base font-medium text-ink no-underline ${focusRing}`}
                >
                  How it works
                </a>
              </div>
              <ul data-a="fade" data-intro className="m-0 flex list-none flex-wrap gap-x-6 gap-y-2 p-0 text-sm text-muted">
                {POINTS.map((p) => (
                  <li key={p} className="inline-flex items-center gap-2">
                    <span aria-hidden="true" className="size-1.5 rounded-full bg-jar-green" />
                    {p}
                  </li>
                ))}
              </ul>
            </div>

            <Stage />
          </header>
        </div>

        <div className="relative">
          <Backdrop />
          <div className="mx-auto max-w-[1160px] px-4 sm:px-6">
            <section id="how" className="flex scroll-mt-28 flex-col gap-7 pt-6">
              <h2 data-a="reveal" className="m-0 font-display text-3xl font-bold tracking-[-0.03em] sm:text-4xl">
                How it works
              </h2>
              <ol className="m-0 grid list-none gap-4 p-0 md:grid-cols-3 md:gap-5">
                {STEPS.map((s) => (
                  <li
                    key={s.n}
                    data-a="reveal"
                    data-lens="22"
                    className="glass relative flex gap-5 rounded-[28px] p-6 md:flex-col md:gap-4 md:p-7"
                  >
                    <span
                      aria-hidden="true"
                      className="flex size-9 flex-none items-center justify-center rounded-full bg-ink font-display text-[15px] font-bold text-paper"
                    >
                      {s.n}
                    </span>
                    <div className="flex flex-col gap-1.5">
                      <h3 className="m-0 font-display text-xl font-bold tracking-[-0.02em]">{s.title}</h3>
                      <p className="m-0 text-[15px] leading-[1.6] text-ink-soft">{s.body}</p>
                    </div>
                  </li>
                ))}
              </ol>
            </section>

            <section
              id="fees"
              data-a="reveal"
              data-lens="26"
              className="glass relative mt-14 flex scroll-mt-28 flex-col gap-6 rounded-[32px] px-6 py-8 sm:mt-16 md:flex-row md:items-center md:justify-between md:gap-10 md:rounded-[36px] md:px-11 md:py-10"
            >
              <div className="flex min-w-0 flex-col gap-4 overflow-hidden sm:flex-row sm:items-center sm:gap-8">
                <div
                  data-a="fee"
                  className="font-display text-[88px] leading-[0.9] font-bold tracking-[-0.05em] text-ink sm:text-[104px]"
                >
                  $1
                </div>
                <div className="flex max-w-[420px] flex-col gap-2">
                  <h2 className="m-0 font-display text-[26px] font-bold tracking-[-0.02em] sm:text-[28px]">
                    One flat fee
                  </h2>
                  <p className="m-0 text-[15px] leading-[1.6] text-ink-soft sm:text-base">
                    GigJar takes $1 from each payout. That is the whole price list.
                  </p>
                </div>
              </div>
              <Link href="/register" className={primaryCta}>
                Start your jar
              </Link>
            </section>

            <footer
              data-a="reveal"
              className="flex flex-col gap-1 px-1 pt-9 pb-10 text-sm text-muted sm:flex-row sm:flex-wrap sm:items-center sm:justify-between sm:gap-6 sm:pt-10 sm:pb-11"
            >
              <span>GigJar. USDT on TRON, payouts to Cleva.</span>
              <nav aria-label="Footer" className="-ml-2.5 flex gap-1 sm:ml-0 sm:gap-2">
                <FooterLink href="/login">Log in</FooterLink>
                <FooterLink href="#how">How it works</FooterLink>
                <FooterLink href="#fees">Fees</FooterLink>
              </nav>
            </footer>
          </div>
        </div>
      </div>
    </LandingMotion>
  );
}

function Line({ children, className = "" }: { children: string; className?: string }) {
  return (
    <span className="block overflow-hidden pb-[0.08em]">
      <span data-a="line" className={`block ${className}`}>
        {children}
      </span>
    </span>
  );
}

function Nav() {
  return (
    <div data-a="nav" data-intro className="sticky top-3 z-50 pt-3 sm:top-4 sm:pt-5">
      <nav
        aria-label="Main"
        data-lens="16"
        className="glass relative flex items-center justify-between gap-2 rounded-full py-1.5 pr-1.5 pl-4 transition-[background-color] duration-500 sm:gap-4 sm:py-2 sm:pr-2 sm:pl-5"
      >
        <a
          href="#top"
          className="inline-flex min-h-11 items-center gap-2.5 font-display text-lg font-bold tracking-[-0.02em] text-ink no-underline"
        >
          <JarMark className="h-7 w-7 text-ink" />
          GigJar
        </a>
        <div className="hidden items-center gap-1 md:flex">
          <NavLink href="#how">How it works</NavLink>
          <NavLink href="#fees">Fees</NavLink>
        </div>
        <div className="flex items-center gap-0.5 sm:gap-1">
          <Link
            href="/login"
            className="inline-flex min-h-11 items-center rounded-full px-3 text-[15px] font-medium text-ink no-underline hover:text-ink-soft sm:px-4"
          >
            Log in
          </Link>
          <Link
            href="/register"
            className="inline-flex min-h-11 items-center justify-center rounded-full bg-ink px-[18px] text-[15px] font-semibold text-paper no-underline transition-colors hover:bg-ink-soft sm:px-5"
          >
            <span className="sm:hidden">Start</span>
            <span className="hidden sm:inline">Start your jar</span>
          </Link>
        </div>
      </nav>
    </div>
  );
}

function NavLink({ href, children }: { href: string; children: string }) {
  return (
    <a
      href={href}
      className="inline-flex min-h-11 items-center rounded-full px-4 text-[15px] font-medium text-ink-soft no-underline transition-colors hover:text-ink"
    >
      {children}
    </a>
  );
}

function FooterLink({ href, children }: { href: string; children: string }) {
  const className = "inline-flex min-h-11 items-center px-2.5 text-ink-soft no-underline hover:text-ink";
  return href.startsWith("#") ? (
    <a href={href} className={className}>
      {children}
    </a>
  ) : (
    <Link href={href} className={className}>
      {children}
    </Link>
  );
}

/** Soft colour behind the lower sections, so the glass has something to bend. */
function Backdrop() {
  const shapes = [
    { top: "-6%", left: "-14%", size: "min(70vw, 760px)", color: "rgb(33 158 122 / 0.16)" },
    { top: "28%", right: "-16%", size: "min(64vw, 680px)", color: "rgb(232 146 47 / 0.16)" },
    { bottom: "-10%", left: "18%", size: "min(56vw, 600px)", color: "rgb(33 158 122 / 0.1)" },
  ];
  return (
    <div aria-hidden="true" className="pointer-events-none absolute inset-0 -z-10 overflow-hidden">
      {shapes.map(({ size, color, ...pos }, i) => (
        <div
          key={i}
          className="absolute rounded-full"
          style={{ ...pos, width: size, height: size, background: `radial-gradient(closest-side, ${color}, transparent)` }}
        />
      ))}
    </div>
  );
}

function Stage() {
  return (
    <div data-a="stage" data-intro className="relative mx-auto w-full max-w-[480px]">
      <div
        role="img"
        aria-label="A glass jar of coins, filled to $8.40 of the $12 payout line"
        className="relative aspect-[44/52] w-full"
      >
        <JarScene />

        <div
          data-a="chip"
          data-intro
          data-lens="14"
          className="glass glass-dense absolute top-[24%] left-0 flex items-center gap-3 rounded-[20px] py-2.5 pr-4 pl-2.5 sm:left-[-2%]"
        >
          <span
            data-a="pay-icon"
            className="group/pay flex size-9 items-center justify-center rounded-[12px] bg-jar-green text-paper transition-colors duration-500 data-[out]:bg-ink"
          >
            <svg
              width="18"
              height="18"
              viewBox="0 0 24 24"
              aria-hidden="true"
              fill="none"
              stroke="currentColor"
              strokeWidth="2.2"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
              <path className="group-data-[out]/pay:hidden" d="M17 7 7 17m0 0h7.5M7 17V9.5" />
              <path className="hidden group-data-[out]/pay:inline" d="M7 17 17 7m0 0H9.5M17 7v7.5" />
            </svg>
          </span>
          <span className="flex flex-col">
            <span data-a="pay-label" className="text-[13px] font-medium text-ink">
              Flyer design
            </span>
            <span data-a="pay-amount" className="text-[13px] font-semibold text-jar-green tabular-nums">
              +$3.00 USDT
            </span>
          </span>
        </div>

        <div
          data-a="chip"
          data-intro
          data-lens="16"
          className="glass glass-dense absolute right-0 bottom-[7%] flex w-[min(220px,58%)] flex-col gap-2 rounded-[22px] px-4 py-3.5 sm:right-[-2%]"
        >
          <span className="text-xs font-medium text-ink-soft">In your jar</span>
          <span
            data-a="count"
            data-value="8.40"
            className="font-display text-[28px] leading-none font-bold tracking-[-0.03em] text-ink tabular-nums"
          >
            $8.40
          </span>
          <span className="relative block h-1.5 overflow-hidden rounded-full bg-ink/10">
            <span
              data-a="progress"
              className="absolute inset-y-0 left-0 w-full origin-left rounded-full bg-jar-green"
              style={{ transform: "scaleX(0.7)" }}
            />
          </span>
          <span className="flex items-center gap-1.5 text-xs text-ink-soft">
            <span aria-hidden="true" className="size-1.5 rounded-full bg-lid-orange" />
            Pays out at $12
          </span>
        </div>
      </div>
    </div>
  );
}
