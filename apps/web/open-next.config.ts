import { defineCloudflareConfig } from "@opennextjs/cloudflare";
import staticAssetsIncrementalCache from "@opennextjs/cloudflare/overrides/incremental-cache/static-assets-incremental-cache";

// Pages Next prerenders at build time (/, /register, /forgot-password, the link preview images)
// are read from the Worker's static assets. Without an incremental cache they had nowhere to
// come from and returned 500. `opennextjs-cloudflare deploy` copies them into the assets.
// The site has no revalidated pages, which is what this read-only cache is for.
export default defineCloudflareConfig({
  incrementalCache: staticAssetsIncrementalCache,
});
