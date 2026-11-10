import Link from "next/link";
import { AppNav } from "@/components/app-nav";
import { JarMark } from "@/components/jar-mark";

export default function AppLayout({ children }: LayoutProps<"/">) {
  return (
    <div className="mx-auto flex min-h-screen max-w-6xl flex-col gap-6 px-4 py-6 md:flex-row md:gap-10 md:px-8">
      <aside className="space-y-6 md:w-56 md:shrink-0">
        <Link href="/dashboard" className="flex items-center gap-2">
          <JarMark className="h-8 w-8 text-ink" />
          <span className="font-display text-xl font-bold">GigJar</span>
        </Link>
        <AppNav />
      </aside>
      <main className="min-w-0 flex-1">{children}</main>
    </div>
  );
}
