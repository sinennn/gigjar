import type { Metadata } from "next";
import { redirect } from "next/navigation";
import { Suspense } from "react";
import { AppShell } from "@/components/app/app-shell";
import { Backdrop } from "@/components/backdrop";
import { JarMark } from "@/components/jar-mark";
import { getCurrentUser } from "@/lib/api/server";

export const metadata: Metadata = { robots: { index: false } };

export default function AppLayout({ children }: LayoutProps<"/app">) {
  return (
    <Suspense fallback={<Waking />}>
      <Gate>{children}</Gate>
    </Suspense>
  );
}

/** Signed-in screens only. No valid session sends the visitor to the login page. */
async function Gate({ children }: { children: React.ReactNode }) {
  const user = await getCurrentUser();
  if (!user) {
    redirect("/login");
  }
  return <AppShell user={user}>{children}</AppShell>;
}

/** Shown while the session check runs, which can take a moment if the API is waking up. */
function Waking() {
  return (
    <div className="relative flex min-h-dvh flex-col items-center justify-center gap-4 px-4 text-center">
      <Backdrop fixed />
      <JarMark className="h-14 w-14 text-ink motion-safe:animate-pulse" />
      <p role="status" className="m-0 text-ink-soft">
        Opening your jar…
      </p>
    </div>
  );
}
