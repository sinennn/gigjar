import type { ReactNode } from "react";

const TONES = {
  error: "bg-danger-soft text-danger",
  warning: "bg-lid-orange-soft text-ink",
  success: "bg-jar-green-soft text-ink",
};

export function Alert({ tone = "error", children }: { tone?: keyof typeof TONES; children: ReactNode }) {
  return (
    <p role={tone === "error" ? "alert" : "status"} className={`rounded-xl px-4 py-3 text-sm ${TONES[tone]}`}>
      {children}
    </p>
  );
}
