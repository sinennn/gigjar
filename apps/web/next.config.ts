import type { NextConfig } from "next";

const apiOrigin = process.env.GIGJAR_API_ORIGIN ?? "http://localhost:8090";

const nextConfig: NextConfig = {
  cacheComponents: true,
  partialPrefetching: true,
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
  async redirects() {
    return [{ source: "/", destination: "/dashboard", permanent: false }];
  },
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${apiOrigin}/:path*` }];
  },
};

export default nextConfig;
