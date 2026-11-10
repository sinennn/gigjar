type JarMarkProps = {
  className?: string;
};

export function JarMark({ className }: JarMarkProps) {
  return (
    <svg viewBox="0 0 64 72" className={className} role="img" aria-label="GigJar">
      <rect x="18" y="2" width="28" height="8" rx="2" className="fill-lid-orange" />
      <rect x="6" y="14" width="52" height="54" rx="12" fill="none" strokeWidth="4" className="stroke-current" />
      <circle cx="30" cy="42" r="7" className="fill-lid-orange" />
      <circle cx="44" cy="40" r="5.5" className="fill-jar-green" />
      <circle cx="20" cy="55" r="7" className="fill-jar-green" />
      <circle cx="36" cy="56" r="7" className="fill-jar-green" />
    </svg>
  );
}
