type ScreenScaffoldProps = {
  title: string;
  summary: string;
  mustShow: string[];
  endpoints: string[];
};

export function ScreenScaffold({ title, summary, mustShow, endpoints }: ScreenScaffoldProps) {
  return (
    <section className="space-y-6">
      <header className="space-y-1">
        <h1 className="font-display text-2xl font-bold sm:text-3xl">{title}</h1>
        <p className="text-ink-soft">{summary}</p>
      </header>
      <div className="rounded-2xl border border-dashed border-lid-orange bg-lid-orange-soft/40 p-5 text-sm">
        <p className="font-semibold">Scaffold: not wired to the API yet</p>
        <div className="mt-3 grid gap-4 sm:grid-cols-2">
          <div>
            <p className="font-medium text-ink-soft">This screen must show</p>
            <ul className="mt-1 list-disc space-y-1 pl-5">
              {mustShow.map((item) => (
                <li key={item}>{item}</li>
              ))}
            </ul>
          </div>
          <div>
            <p className="font-medium text-ink-soft">Contract operations</p>
            <ul className="mt-1 space-y-1 font-mono text-xs">
              {endpoints.map((endpoint) => (
                <li key={endpoint}>{endpoint}</li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </section>
  );
}
