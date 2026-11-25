// Verifies the built site: every expected route exists in both languages, and every internal link,
// image and asset referenced from any page resolves to a file in the build (and #fragments to an id).
// Usage: node scripts/check-links.mjs dist
import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const DIST = process.argv[2] ?? "dist";
const ROUTES = ["/", "/features/", "/download/", "/releases/", "/docs/", "/community/", "/privacy/", "/licenses/"];
const LOCALES = ["", "/vi"];
const errors = [];

function walk(dir) {
  return readdirSync(dir).flatMap((f) => {
    const p = join(dir, f);
    return statSync(p).isDirectory() ? walk(p) : [p];
  });
}
const pageFile = (url) => {
  const clean = decodeURI(url.split("#")[0].split("?")[0]);
  if (clean.endsWith("/")) return join(DIST, clean, "index.html");
  return join(DIST, clean);
};

// 1. generated routes
for (const l of LOCALES)
  for (const r of ROUTES) {
    const url = l + r;
    if (!existsSync(pageFile(url))) errors.push(`missing route ${url}`);
  }
if (!existsSync(join(DIST, "404.html"))) errors.push("missing 404.html");
for (const f of ["sitemap-index.xml", "robots.txt", "favicon.svg", "branding/og-image.png"])
  if (!existsSync(join(DIST, f))) errors.push(`missing ${f}`);

// 2. links and assets on every page
const pages = walk(DIST).filter((f) => f.endsWith(".html"));
const ids = new Map();
const idsOf = (file) => {
  if (!ids.has(file)) ids.set(file, new Set([...readFileSync(file, "utf8").matchAll(/\sid="([^"]+)"/g)].map((m) => m[1])));
  return ids.get(file);
};
let checked = 0;
for (const page of pages) {
  const html = readFileSync(page, "utf8");
  const self = "/" + relative(DIST, page).replace(/index\.html$/, "");
  const refs = [...html.matchAll(/\s(?:href|src)="([^"]+)"/g), ...html.matchAll(/\ssrcset="([^"]+)"/g)].flatMap((m) =>
    m[0].includes("srcset") ? m[1].split(",").map((s) => s.trim().split(/\s+/)[0]) : [m[1]],
  );
  for (const ref of refs) {
    if (!ref || /^(https?:|mailto:|data:|tel:)/.test(ref)) continue;
    // release storage is served from the VPS bind mount, not from dist/; only the manifest may be linked statically
    if (ref.startsWith("/downloads/")) {
      if (ref !== "/downloads/index.json") errors.push(`${self}: static link into release storage ${ref}`);
      continue;
    }
    checked++;
    if (ref.startsWith("#")) {
      if (ref.length > 1 && !idsOf(page).has(ref.slice(1))) errors.push(`${self}: missing anchor ${ref}`);
      continue;
    }
    if (!ref.startsWith("/")) {
      errors.push(`${self}: relative link ${ref} (use site-absolute paths)`);
      continue;
    }
    const target = pageFile(ref);
    if (!existsSync(target)) {
      errors.push(`${self}: broken link ${ref}`);
      continue;
    }
    const frag = ref.split("#")[1];
    if (frag && target.endsWith(".html") && !idsOf(target).has(frag)) errors.push(`${self}: missing anchor ${ref}`);
    if (!ref.split("#")[0].endsWith("/") && target.endsWith("index.html") === false && !/\.[a-z0-9]+$/i.test(ref.split("#")[0]))
      errors.push(`${self}: link without trailing slash ${ref}`);
  }
  if (/localhost|127\.0\.0\.1/.test(html)) errors.push(`${self}: contains a localhost URL`);
}

if (errors.length) {
  console.error(`link check failed (${errors.length}):\n  ` + errors.join("\n  "));
  process.exit(1);
}
console.log(`link check passed: ${pages.length} pages, ${checked} internal references, ${ROUTES.length * LOCALES.length} routes`);
