/** Shown under a button while a request is taking long, typically because the server is waking up. */
export function SlowNote({ show }: { show: boolean }) {
  if (!show) {
    return null;
  }
  return (
    <p role="status" className="m-0 text-center text-sm leading-relaxed text-ink-soft">
      Still working. The first request after a quiet spell can take up to a minute while our server wakes up.
    </p>
  );
}
