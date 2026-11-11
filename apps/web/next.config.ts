import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  turbopack: {
    rules: {
      "*.css": {
        loaders: ["@tailwindcss/turbopack"],
        as: "*.css",
      },
    },
  },
  // The signed-in app moved under /app. Old links (and the API's default Google
  // sign-in return_to of /dashboard) still land in the right place.
  async redirects() {
    return [
      { source: "/dashboard", destination: "/app", permanent: false },
      ...["jar", "links", "transactions", "settlements", "wallet", "settings"].map((p) => ({
        source: `/${p}/:rest*`,
        destination: `/app/${p}/:rest*`,
        permanent: false,
      })),
    ];
  },
};

export default nextConfig;
