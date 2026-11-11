export function AuthHeading({ title, summary }: { title: string; summary: string }) {
  return (
    <div className="space-y-1.5">
      <h1 className="m-0 font-display text-[28px] font-bold tracking-[-0.03em] text-ink">{title}</h1>
      <p className="m-0 text-[15px] leading-relaxed text-ink-soft">{summary}</p>
    </div>
  );
}
