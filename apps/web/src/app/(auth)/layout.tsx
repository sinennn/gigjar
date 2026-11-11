import Link from "next/link";
import { Backdrop } from "@/components/backdrop";
import { JarMark } from "@/components/jar-mark";
import { LensRoot } from "@/components/lens-root";
import { Glass } from "@/components/ui/glass";

export default function AuthLayout({ children }: LayoutProps<"/">) {
  return (
    <LensRoot className="relative isolate flex min-h-dvh items-center justify-center px-4 py-10">
      <Backdrop fixed />
      <div className="page-in w-full max-w-[420px] space-y-7">
        <Link
          href="/"
          className="mx-auto flex w-fit items-center gap-2.5 font-display text-xl font-bold tracking-[-0.02em] text-ink no-underline"
        >
          <JarMark className="h-9 w-9 text-ink" />
          GigJar
        </Link>
        <Glass size="xl" className="p-6 sm:p-8">
          {children}
        </Glass>
      </div>
    </LensRoot>
  );
}
