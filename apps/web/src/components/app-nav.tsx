import Link from "next/link";

type NavItem = {
  href: "/dashboard" | "/jar" | "/links" | "/transactions" | "/settlements" | "/wallet" | "/settings";
  label: string;
};

const NAV_ITEMS: NavItem[] = [
  { href: "/dashboard", label: "Overview" },
  { href: "/jar", label: "Jar" },
  { href: "/links", label: "Payment links" },
  { href: "/transactions", label: "Transactions" },
  { href: "/settlements", label: "Settlements" },
  { href: "/wallet", label: "Cleva wallet" },
  { href: "/settings", label: "Settings" },
];

export function AppNav() {
  return (
    <nav aria-label="Main" className="flex gap-1 overflow-x-auto md:flex-col md:overflow-visible">
      {NAV_ITEMS.map((item) => (
        <Link
          key={item.href}
          href={item.href}
          className="whitespace-nowrap rounded-lg px-3 py-2 text-sm font-medium text-ink-soft hover:bg-paper hover:text-ink"
        >
          {item.label}
        </Link>
      ))}
    </nav>
  );
}
