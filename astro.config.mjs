// @ts-check
import { defineConfig } from "astro/config";
import sitemap from "@astrojs/sitemap";
import tailwindcss from "@tailwindcss/vite";

export const SITE = "https://photobooth.apexvoid.net";

// Static output only: every route is pre-rendered to dist/<route>/index.html so a plain
// Nginx `try_files $uri $uri/ =404` serves it. No server runtime, no adapter.
export default defineConfig({
  site: SITE,
  output: "static",
  trailingSlash: "always",
  build: { format: "directory" },
  i18n: {
    defaultLocale: "en",
    locales: ["en", "vi"],
    routing: { prefixDefaultLocale: false, redirectToDefaultLocale: false },
  },
  integrations: [
    sitemap({
      i18n: { defaultLocale: "en", locales: { en: "en", vi: "vi" } },
      filter: (page) => !page.includes("/404"),
    }),
  ],
  image: { responsiveStyles: true },
  vite: { plugins: [tailwindcss()] },
  devToolbar: { enabled: false },
});
