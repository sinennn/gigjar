import type { ReactNode } from "react";

const TONES = {
  error: "bg-danger-soft text-danger",
  warning: "bg-lid-orange-soft text-ink",
  success: "bg-jar-green-soft text-ink",
  info: "bg-ink/5 text-ink-soft",
};

export function Alert({ tone = "error", children }: { tone?: keyof typeof TONES; children: ReactNode }) {
  return (
    <div role={tone === "error" ? "alert" : "status"} className={`rounded-2xl px-4 py-3 text-sm leading-relaxed ${TONES[tone]}`}>
      {children}
    </div>
  );
}
