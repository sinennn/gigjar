import type { Metadata } from "next";
import { Suspense } from "react";
import { AppShell } from "@/components/app/app-shell";
import { ApiDown } from "@/components/auth/api-down";
import { ToLogin } from "@/components/auth/to-login";
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
  let user: Awaited<ReturnType<typeof getCurrentUser>>;
  try {
    user = await getCurrentUser();
  } catch {
    // The API is asleep or unreachable. Say so, and let the visitor retry. Never treat it as signed out.
    return <ApiDown />;
  }
  if (!user) {
    return <ToLogin />;
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
