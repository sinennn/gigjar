"use client";

import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect } from "react";
import { Backdrop } from "@/components/backdrop";
import { JarMark } from "@/components/jar-mark";
import { loginUrlFor } from "@/lib/return-to";

/**
 * Shown instead of the app when there is no session. The layout runs on the server and cannot see
 * which page was asked for, so this knows the path and sends the visitor to the log in page with
 * `return_to`, which brings them back here afterwards. No app content is rendered for them.
 */
export function ToLogin() {
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams().toString();
  const target = loginUrlFor(search ? `${pathname}?${search}` : pathname);

  useEffect(() => {
    router.replace(target);
  }, [router, target]);

  return (
    <div className="relative flex min-h-dvh flex-col items-center justify-center gap-4 px-4 text-center">
      <Backdrop fixed />
      <JarMark className="h-14 w-14 text-ink" />
      <p role="status" className="m-0 text-ink-soft">
        Taking you to log in…
      </p>
      <Link href={target} className="text-sm font-medium text-ink underline underline-offset-4">
        Log in
      </Link>
    </div>
  );
}
