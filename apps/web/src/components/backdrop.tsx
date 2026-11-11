/**
 * Soft green and orange shapes behind the glass, so it has something to bend.
 * `fixed` keeps them in place on long app pages; the landing uses the absolute variant.
 */
export function Backdrop({ fixed = false }: { fixed?: boolean }) {
  const shapes = [
    { top: "-6%", left: "-14%", size: "min(70vw, 760px)", color: "rgb(33 158 122 / 0.16)" },
    { top: "28%", right: "-16%", size: "min(64vw, 680px)", color: "rgb(232 146 47 / 0.16)" },
    { bottom: "-10%", left: "18%", size: "min(56vw, 600px)", color: "rgb(33 158 122 / 0.1)" },
  ];
  return (
    <div
      aria-hidden="true"
      className={`pointer-events-none inset-0 -z-10 overflow-hidden ${fixed ? "fixed" : "absolute"}`}
    >
      {shapes.map(({ size, color, ...pos }, i) => (
        <div
          key={i}
          className="absolute rounded-full"
          style={{ ...pos, width: size, height: size, background: `radial-gradient(closest-side, ${color}, transparent)` }}
        />
      ))}
    </div>
  );
}
