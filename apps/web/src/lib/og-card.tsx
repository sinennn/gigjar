import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { ImageResponse } from "next/og";

/**
 * The link preview card shared on WhatsApp, X, Slack and the rest. 1200x630 is what every
 * platform expects; keep the result small (WhatsApp skips preview images over about 300 KB).
 */
export const OG_SIZE = { width: 1200, height: 630 };
export const OG_ALT = "GigJar. Small gigs add up: a glass jar of USDT coins that pays out to Cleva at $12.";

/**
 * Reads the jar render and fonts from disk. Only call it while drawing the card, never at module
 * load: request-time pages import this module to read OG_ALT and OG_SIZE for their metadata, and
 * on Cloudflare Workers there is no disk, so a top-level read broke /login and /app. The card itself
 * is prerendered at build time, where the files exist.
 */
async function loadAssets() {
  const assets = join(process.cwd(), "assets");
  const [jar, spaceGrotesk, interRegular, interSemiBold] = await Promise.all([
    readFile(join(assets, "og-jar.png"), "base64"),
    readFile(join(assets, "SpaceGrotesk-Bold.ttf")),
    readFile(join(assets, "Inter-Regular.ttf")),
    readFile(join(assets, "Inter-SemiBold.ttf")),
  ]);
  return { jar, spaceGrotesk, interRegular, interSemiBold };
}

const INK = "#13263a";
const INK_SOFT = "#3d4f63";
const CREAM = "#f6f3ec";
const GREEN = "#219e7a";
const ORANGE = "#e8922f";

export async function ogCard() {
  const { jar, spaceGrotesk, interRegular, interSemiBold } = await loadAssets();
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          position: "relative",
          backgroundColor: CREAM,
          backgroundImage: `radial-gradient(circle at 8% 92%, rgba(33,158,122,0.14), rgba(246,243,236,0) 38%), radial-gradient(circle at 96% 12%, rgba(232,146,47,0.13), rgba(246,243,236,0) 34%)`,
          fontFamily: "Inter",
          color: INK,
        }}
      >
        <div style={{ display: "flex", flexDirection: "column", padding: "64px 0 64px 76px", width: 720 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 14 }}>
            <svg width="40" height="45" viewBox="0 0 64 72">
              <rect x="18" y="2" width="28" height="8" rx="2" fill={ORANGE} />
              <rect x="6" y="14" width="52" height="54" rx="12" fill="none" stroke={INK} strokeWidth="5" />
              <circle cx="30" cy="42" r="7" fill={ORANGE} />
              <circle cx="44" cy="40" r="5.5" fill={GREEN} />
              <circle cx="20" cy="55" r="7" fill={GREEN} />
              <circle cx="36" cy="56" r="7" fill={GREEN} />
            </svg>
            <span style={{ fontFamily: "Space Grotesk", fontSize: 36, letterSpacing: -0.8 }}>GigJar</span>
          </div>

          <div
            style={{
              display: "flex",
              flexDirection: "column",
              marginTop: 54,
              fontFamily: "Space Grotesk",
              fontSize: 112,
              lineHeight: 0.95,
              letterSpacing: -4.5,
            }}
          >
            <span>Small gigs</span>
            <span style={{ color: GREEN }}>add up.</span>
          </div>

          <p style={{ margin: "34px 0 0", fontSize: 30, lineHeight: 1.4, color: INK_SOFT, maxWidth: 580 }}>
            Get paid in USDT for any gig, however small. Your jar pays out to Cleva at $12.
          </p>

          <div style={{ display: "flex", gap: 12, marginTop: "auto", fontSize: 22, fontWeight: 600 }}>
            {["$1 flat fee", "No minimum gig", "USDT on TRON"].map((label) => (
              <span
                key={label}
                style={{
                  display: "flex",
                  alignItems: "center",
                  gap: 10,
                  padding: "10px 20px",
                  borderRadius: 999,
                  backgroundColor: "rgba(255,255,255,0.7)",
                  border: "1px solid rgba(255,255,255,0.95)",
                  boxShadow: "0 8px 20px -10px rgba(19,38,58,0.25)",
                }}
              >
                <span style={{ width: 8, height: 8, borderRadius: 8, backgroundColor: GREEN }} />
                {label}
              </span>
            ))}
          </div>
        </div>

        {/* ImageResponse renders plain <img> only; next/image does not apply here. */}
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={`data:image/png;base64,${jar}`}
          width={377}
          height={560}
          alt=""
          style={{ position: "absolute", right: 96, top: 36 }}
        />

        <div
          style={{
            position: "absolute",
            right: 52,
            bottom: 70,
            width: 236,
            display: "flex",
            flexDirection: "column",
            gap: 10,
            padding: "18px 22px",
            borderRadius: 24,
            backgroundColor: "rgba(255,253,248,0.86)",
            border: "1px solid rgba(255,255,255,0.95)",
            boxShadow: "0 18px 40px -16px rgba(19,38,58,0.35)",
          }}
        >
          <span style={{ fontSize: 17, fontWeight: 600, color: INK_SOFT }}>In your jar</span>
          <span style={{ fontFamily: "Space Grotesk", fontSize: 44, lineHeight: 1, letterSpacing: -1.2 }}>$8.40</span>
          <div style={{ display: "flex", height: 8, borderRadius: 8, backgroundColor: "rgba(19,38,58,0.1)" }}>
            <div style={{ width: "70%", height: 8, borderRadius: 8, backgroundColor: GREEN }} />
          </div>
          <span style={{ display: "flex", alignItems: "center", gap: 8, fontSize: 16, color: INK_SOFT }}>
            <span style={{ width: 8, height: 8, borderRadius: 8, backgroundColor: ORANGE }} />
            Pays out at $12
          </span>
        </div>
      </div>
    ),
    {
      ...OG_SIZE,
      fonts: [
        { name: "Space Grotesk", data: spaceGrotesk, weight: 700, style: "normal" },
        { name: "Inter", data: interRegular, weight: 400, style: "normal" },
        { name: "Inter", data: interSemiBold, weight: 600, style: "normal" },
      ],
    },
  );
}
