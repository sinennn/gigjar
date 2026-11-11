"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useEffect, useRef, useState, type ReactNode } from "react";
import { Backdrop } from "@/components/backdrop";
import { JarMark } from "@/components/jar-mark";
import { LensRoot } from "@/components/lens-root";
import { SimulatedBadge } from "@/components/ui/badge";
import { Glass } from "@/components/ui/glass";
import { Icon, type IconName } from "@/components/ui/icons";
import { AppDataProvider, useJar, type User } from "./jar-context";

type NavItem = { href: string; label: string; short?: string; icon: IconName };

const OVERVIEW: NavItem = { href: "/app", label: "Overview", icon: "home" };
const JAR: NavItem = { href: "/app/jar", label: "Jar", icon: "jar" };
const LINKS: NavItem = { href: "/app/links", label: "Payment links", short: "Links", icon: "link" };
const TRANSACTIONS: NavItem = { href: "/app/transactions", label: "Transactions", short: "Activity", icon: "activity" };
const SETTLEMENTS: NavItem = { href: "/app/settlements", label: "Settlements", icon: "send" };
const WALLET: NavItem = { href: "/app/wallet", label: "Cleva wallet", icon: "wallet" };
const SETTINGS: NavItem = { href: "/app/settings", label: "Settings", icon: "settings" };

const SIDEBAR = [OVERVIEW, JAR, LINKS, TRANSACTIONS, SETTLEMENTS, WALLET, SETTINGS];
const TABS = [OVERVIEW, LINKS, JAR, TRANSACTIONS];
const MORE = [SETTLEMENTS, WALLET, SETTINGS];

function isActive(pathname: string, href: string): boolean {
  return href === "/app" ? pathname === "/app" : pathname === href || pathname.startsWith(`${href}/`);
}

export function AppShell({ user, children }: { user: User; children: ReactNode }) {
  return (
    <AppDataProvider user={user}>
      <LensRoot className="relative min-h-dvh">
        <Backdrop fixed />
        <a
          href="#main"
          className="sr-only z-50 rounded-full bg-ink px-4 py-2 text-paper focus:not-sr-only focus:fixed focus:top-3 focus:left-3"
        >
          Skip to content
        </a>
        <div className="mx-auto flex max-w-[1160px] flex-col px-4 sm:px-6 md:flex-row md:gap-8 md:py-4">
          <TopBar />
          <Sidebar user={user} />
          <main id="main" className="min-w-0 flex-1 pt-6 pb-32 md:pt-8 md:pb-12">
            {children}
          </main>
        </div>
        <TabBar />
      </LensRoot>
    </AppDataProvider>
  );
}

function useSimulated(): boolean {
  return useJar().jar?.network === "simulated";
}

function Logo() {
  return (
    <Link
      href="/app"
      className="inline-flex min-h-11 items-center gap-2.5 font-display text-lg font-bold tracking-[-0.02em] text-ink no-underline"
    >
      <JarMark className="h-7 w-7 text-ink" />
      GigJar
    </Link>
  );
}

/** Phones: a slim glass bar with the logo and, when it applies, the Simulated badge. */
function TopBar() {
  const simulated = useSimulated();
  return (
    <header className="sticky top-3 z-40 pt-3 md:hidden">
      <Glass size="pill" className="flex items-center justify-between gap-2 py-1.5 pr-3 pl-4">
        <Logo />
        {simulated ? <SimulatedBadge /> : null}
      </Glass>
    </header>
  );
}

function Sidebar({ user }: { user: User }) {
  const pathname = usePathname();
  const simulated = useSimulated();
  return (
    <aside className="hidden w-64 shrink-0 md:block">
      <div className="sticky top-4 h-[calc(100dvh-2rem)]">
        <Glass size="xl" className="flex h-full flex-col gap-6 p-4">
          <div className="flex flex-col items-start gap-2 px-2 pt-1">
            <Logo />
            {simulated ? <SimulatedBadge /> : null}
          </div>
          <nav aria-label="Main" className="flex flex-col gap-1">
            {SIDEBAR.map((item) => {
              const active = isActive(pathname, item.href);
              return (
                <Link
                  key={item.href}
                  href={item.href}
                  aria-current={active ? "page" : undefined}
                  className={`flex min-h-11 items-center gap-3 rounded-full px-4 text-[15px] font-medium no-underline transition-colors ${
                    active ? "bg-ink text-paper" : "text-ink-soft hover:bg-ink/5 hover:text-ink"
                  }`}
                >
                  <Icon name={item.icon} className="size-[18px]" />
                  {item.label}
                </Link>
              );
            })}
          </nav>
          <Link
            href="/app/settings"
            className="mt-auto flex min-h-11 flex-col justify-center rounded-2xl px-3 py-2 no-underline hover:bg-ink/5"
          >
            <span className="truncate text-sm font-semibold text-ink">{user.display_name}</span>
            <span className="truncate text-xs text-muted">{user.email}</span>
          </Link>
        </Glass>
      </div>
    </aside>
  );
}

/** Phones: a floating glass tab bar. Less-used screens sit behind "More". */
function TabBar() {
  const pathname = usePathname();
  const [open, setOpen] = useState(false);
  const [openFor, setOpenFor] = useState(pathname);
  const bar = useRef<HTMLDivElement>(null);
  const toggle = useRef<HTMLButtonElement>(null);

  // Close the sheet when the route changes (state reset during render, not in an effect).
  if (openFor !== pathname) {
    setOpenFor(pathname);
    setOpen(false);
  }

  useEffect(() => {
    if (!open) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
        toggle.current?.focus();
      }
    };
    const onPointer = (event: PointerEvent) => {
      if (bar.current && !bar.current.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("pointerdown", onPointer);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("pointerdown", onPointer);
    };
  }, [open]);

  const moreActive = MORE.some((item) => isActive(pathname, item.href));
  const tab = (active: boolean) =>
    `flex min-h-14 flex-1 flex-col items-center justify-center gap-0.5 rounded-full px-1 text-[11px] font-semibold no-underline transition-colors ${
      active ? "bg-ink text-paper" : "text-ink-soft"
    }`;

  return (
    <div ref={bar} className="fixed inset-x-3 bottom-[max(0.75rem,env(safe-area-inset-bottom))] z-40 md:hidden">
      {open ? (
        <Glass
          id="more-menu"
          size="lg"
          lens={false}
          className="absolute right-0 bottom-full left-0 mb-2 flex flex-col gap-1 p-2 shadow-[0_20px_50px_-20px_rgb(19_38_58/0.35)]"
        >
          {MORE.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              aria-current={isActive(pathname, item.href) ? "page" : undefined}
              className="flex min-h-12 items-center gap-3 rounded-full px-4 text-[15px] font-medium text-ink no-underline hover:bg-ink/5 aria-[current=page]:bg-ink aria-[current=page]:text-paper"
            >
              <Icon name={item.icon} className="size-[18px]" />
              {item.label}
            </Link>
          ))}
        </Glass>
      ) : null}
      <Glass as="nav" size="pill" lens={false} aria-label="Main" className="flex items-center gap-1 p-1.5">
        {TABS.map((item) => {
          const active = isActive(pathname, item.href);
          return (
            <Link key={item.href} href={item.href} aria-current={active ? "page" : undefined} className={tab(active)}>
              <Icon name={item.icon} className="size-5" />
              {item.short ?? item.label}
            </Link>
          );
        })}
        <button
          ref={toggle}
          type="button"
          aria-expanded={open}
          aria-controls="more-menu"
          onClick={() => setOpen((value) => !value)}
          className={tab(moreActive)}
        >
          <Icon name="more" className="size-5" />
          More
        </button>
      </Glass>
    </div>
  );
}
