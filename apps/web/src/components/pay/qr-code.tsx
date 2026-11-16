import { encode } from "uqr";

/**
 * A QR code drawn as one SVG path (pure, so it renders the same on the server and in the browser). Dark squares on a white tile with the 4-module quiet zone scanners
 * need, whatever the page colour behind it. Error correction M: the code stays readable with a bit of
 * glare or a scuffed screen.
 */
export function QrCode({ value, label, className = "" }: { value: string; label: string; className?: string }) {
  const { data, size } = encode(value, { ecc: "M", border: 0 });
  const quiet = 4;
  const total = size + quiet * 2;

  // One rectangle per horizontal run of dark modules keeps the path short.
  let path = "";
  for (let y = 0; y < size; y++) {
    let x = 0;
    while (x < size) {
      if (!data[y][x]) {
        x += 1;
        continue;
      }
      const start = x;
      while (x < size && data[y][x]) {
        x += 1;
      }
      path += `M${start + quiet} ${y + quiet}h${x - start}v1h-${x - start}z`;
    }
  }

  return (
    <svg
      role="img"
      aria-label={label}
      viewBox={`0 0 ${total} ${total}`}
      shapeRendering="crispEdges"
      className={`block aspect-square rounded-[18px] bg-white ${className}`}
    >
      <path d={path} fill="#13263a" />
    </svg>
  );
}
