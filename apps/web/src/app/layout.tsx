import type { Metadata } from "next";
import { Inter, Space_Grotesk } from "next/font/google";
import "./globals.css";

const inter = Inter({ variable: "--font-inter", subsets: ["latin"] });
const spaceGrotesk = Space_Grotesk({ variable: "--font-space-grotesk", subsets: ["latin"] });

const SHARE_TITLE = "GigJar: small gigs add up";
const SHARE_TEXT =
  "Get paid in USDT for any gig, however small. Your jar collects every payment and sends it to your Cleva wallet at $12, for one flat $1 fee.";

export const metadata: Metadata = {
  // Link previews need absolute URLs. Set NEXT_PUBLIC_SITE_URL if the site moves.
  metadataBase: new URL(process.env.NEXT_PUBLIC_SITE_URL ?? "https://gigjar.pages.dev"),
  title: { default: "GigJar", template: "%s · GigJar" },
  description: "Get paid in USDT for any gig, however small. It lands safely in your Cleva wallet.",
  openGraph: {
    type: "website",
    siteName: "GigJar",
    url: "/",
    locale: "en",
    title: SHARE_TITLE,
    description: SHARE_TEXT,
  },
  twitter: {
    card: "summary_large_image",
    title: SHARE_TITLE,
    description: SHARE_TEXT,
  },
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="en" className={`${inter.variable} ${spaceGrotesk.variable} h-full antialiased`}>
      <body className="min-h-full font-sans">{children}</body>
    </html>
  );
}
