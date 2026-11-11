/**
 * Liquid glass refraction. Builds an SVG displacement map shaped like the
 * element (a rounded rectangle with a curved bezel) and uses it as a
 * backdrop filter, so whatever sits behind the glass bends near its edges.
 *
 * Only Chromium supports SVG filters in backdrop-filter; everywhere else the
 * plain frosted `glass` style stays as it is.
 */

const SVG_NS = "http://www.w3.org/2000/svg";
let defs: SVGDefsElement | null = null;
let seq = 0;

export function supportsLiquidLens(): boolean {
  if (typeof navigator === "undefined") return false;
  const brands = (navigator as Navigator & { userAgentData?: { brands: { brand: string }[] } }).userAgentData
    ?.brands;
  return !!brands?.some((b) => /Chromium/i.test(b.brand));
}

type LensOptions = {
  /** Width of the curved edge in px. */
  bezel?: number;
  /** How far the edge bends the backdrop, in px. */
  strength?: number;
  /** Extra blur on top of the refraction, in px. */
  blur?: number;
};

/** Applies the lens to an element and keeps it in step with its size. Returns a cleanup. */
export function liquidLens(el: HTMLElement, options: LensOptions = {}): () => void {
  const id = `liquid-lens-${++seq}`;
  const filter = document.createElementNS(SVG_NS, "filter");
  filter.setAttribute("id", id);
  filter.setAttribute("filterUnits", "userSpaceOnUse");
  filter.setAttribute("primitiveUnits", "userSpaceOnUse");
  filter.setAttribute("color-interpolation-filters", "sRGB");
  const image = document.createElementNS(SVG_NS, "feImage");
  image.setAttribute("result", "map");
  image.setAttribute("preserveAspectRatio", "none");
  const displace = document.createElementNS(SVG_NS, "feDisplacementMap");
  displace.setAttribute("in", "SourceGraphic");
  displace.setAttribute("in2", "map");
  displace.setAttribute("xChannelSelector", "R");
  displace.setAttribute("yChannelSelector", "G");
  filter.append(image, displace);
  ensureDefs().append(filter);

  const previous = el.style.backdropFilter;
  let lastKey = "";

  const update = () => {
    const w = Math.round(el.offsetWidth);
    const h = Math.round(el.offsetHeight);
    if (!w || !h) return;
    const radius = Math.min(parseFloat(getComputedStyle(el).borderTopLeftRadius) || 0, w / 2, h / 2);
    const bezel = Math.min(options.bezel ?? 22, Math.min(w, h) / 2 - 1);
    const key = `${w}x${h}r${radius}b${bezel}`;
    if (key === lastKey) return;
    lastKey = key;

    for (const node of [filter, image]) {
      node.setAttribute("x", "0");
      node.setAttribute("y", "0");
      node.setAttribute("width", String(w));
      node.setAttribute("height", String(h));
    }
    image.setAttribute("href", displacementMap(w, h, radius, bezel));
    displace.setAttribute("scale", String(options.strength ?? 28));
    el.style.backdropFilter = `url(#${id}) blur(${options.blur ?? 2}px) saturate(185%) brightness(1.05)`;
  };

  const ro = new ResizeObserver(update);
  ro.observe(el);
  update();

  return () => {
    ro.disconnect();
    filter.remove();
    el.style.backdropFilter = previous;
  };
}

function ensureDefs(): SVGDefsElement {
  if (defs?.isConnected) return defs;
  const svg = document.createElementNS(SVG_NS, "svg");
  svg.setAttribute("aria-hidden", "true");
  svg.setAttribute("width", "0");
  svg.setAttribute("height", "0");
  svg.style.cssText = "position:absolute;width:0;height:0;overflow:hidden;pointer-events:none";
  defs = document.createElementNS(SVG_NS, "defs");
  svg.append(defs);
  document.body.append(svg);
  return defs;
}

/**
 * Red and green hold the x and y offset (128 = none). Inside the bezel each
 * pixel samples from further in, following the edge's inward normal, with
 * more pull right at the rim, like light through a rounded glass edge.
 */
function displacementMap(w: number, h: number, r: number, bezel: number): string {
  const canvas = document.createElement("canvas");
  canvas.width = w;
  canvas.height = h;
  const g = canvas.getContext("2d");
  if (!g) return "";
  const img = g.createImageData(w, h);
  const d = img.data;
  const hx = w / 2;
  const hy = h / 2;
  const ix = hx - r;
  const iy = hy - r;

  for (let y = 0; y < h; y++) {
    for (let x = 0; x < w; x++) {
      const px = x + 0.5 - hx;
      const py = y + 0.5 - hy;
      const qx = Math.abs(px) - ix;
      const qy = Math.abs(py) - iy;
      // Signed distance to the rounded rectangle (negative inside).
      let dist: number;
      let nx: number;
      let ny: number;
      if (qx > 0 && qy > 0) {
        const len = Math.hypot(qx, qy) || 1;
        dist = len - r;
        nx = (qx / len) * Math.sign(px);
        ny = (qy / len) * Math.sign(py);
      } else if (qx > qy) {
        dist = qx - r;
        nx = Math.sign(px);
        ny = 0;
      } else {
        dist = qy - r;
        nx = 0;
        ny = Math.sign(py);
      }
      const inside = -dist;
      let ox = 0;
      let oy = 0;
      if (inside >= 0 && inside < bezel) {
        const t = 1 - inside / bezel;
        const pull = t * t * (3 - 2 * t) * t;
        ox = -nx * pull;
        oy = -ny * pull;
      }
      const i = (y * w + x) * 4;
      d[i] = 128 + ox * 127;
      d[i + 1] = 128 + oy * 127;
      d[i + 2] = 128;
      d[i + 3] = 255;
    }
  }
  g.putImageData(img, 0, 0);
  return canvas.toDataURL();
}
