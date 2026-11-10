import { JarMark } from "@/components/jar-mark";

export default function AuthLayout({ children }: LayoutProps<"/">) {
  return (
    <div className="flex min-h-screen items-center justify-center px-4 py-10">
      <div className="w-full max-w-md space-y-8">
        <div className="flex flex-col items-center gap-3 text-center">
          <JarMark className="h-14 w-14 text-ink" />
          <p className="text-ink-soft">Get paid in USDT for any gig, however small.</p>
        </div>
        <div className="rounded-2xl border border-line bg-paper p-6 shadow-sm">{children}</div>
      </div>
    </div>
  );
}
