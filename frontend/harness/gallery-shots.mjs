// Screenshots of the Gallery session-detail screen against the REAL Go service (cmd/uxharness -seed-gallery): real frame
// renderer, fake printer, generated gradient "photos" (never guests). Run once per frontend build to compare before/after.
//   cd frontend && npx vite build -c vite.harness.config.ts
//   go build -tags uxharness -o /tmp/uxh ./cmd/uxharness            (from the repo root)
//   NODE_PATH=<dir with playwright> UXH=/tmp/uxh DIST=dist-harness node harness/gallery-shots.mjs ../docs/img/ux8-gallery after
import { createRequire } from "node:module";
import { spawn } from "node:child_process";
import { mkdirSync } from "node:fs";
const { chromium } = createRequire(import.meta.url)("playwright");

const OUT = process.argv[2] ?? "../docs/img/ux8-gallery";
const PREFIX = process.argv[3] ?? "after";
const UXH = process.env.UXH ?? "/tmp/uxh";
const DIST = process.env.DIST ?? "dist-harness";
const PORT = Number(process.env.PORT ?? 5231);
mkdirSync(OUT, { recursive: true });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

const VP = {
  "1920x1080": { width: 1920, height: 1080 },
  "1600x900": { width: 1600, height: 900 },
  "1366x768": { width: 1366, height: 768 },
  "1280x720": { width: 1280, height: 720 },
  "1080x1920": { width: 1080, height: 1920 },
  "win125-1920x1080": { width: 1536, height: 864, scale: 1.25 },
  "win150-1920x1080": { width: 1280, height: 720, scale: 1.5 },
};
// seeded oldest -> newest in cmd/uxharness/gallery_seed.go; QuerySessions lists newest first
const SCENARIOS = ["no-composition", "multiple-compositions", "video-only", "eight-photos", "three-photo-2x6", "four-photo-4x6"];

const server = spawn(UXH, ["-addr", `127.0.0.1:${PORT}`, "-dist", DIST, "-webcam", "-seed-gallery"], { stdio: "ignore" });
const base = `http://127.0.0.1:${PORT}`;
const rpc = async (name, ...args) => (await fetch(`${base}/rpc/${name}`, { method: "POST", body: JSON.stringify(args) })).json();
for (let i = 0; i < 120; i++) { try { await rpc("BoothReadiness"); break; } catch { await sleep(250); } }
const page1 = await rpc("QuerySessions", { search: "", mode: "", status: "", simulated: "all", from: "", to: "", cursor: "", limit: 40 });
const ids = Object.fromEntries(SCENARIOS.map((s, i) => [s, page1.items[i].id]));

const browser = await chromium.launch();
const problems = [];
async function shot(scenario, vpName, { lang = "en", name, prep, scroll = false } = {}) {
  const vp = VP[vpName];
  const ctx = await browser.newContext({ viewport: { width: vp.width, height: vp.height }, deviceScaleFactor: vp.scale ?? 1 });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => console.log("  pageerror:", e.message));
  await page.goto(`${base}/#gallery`);
  await page.locator("nav[aria-label='Studio navigation']").waitFor();
  if (lang !== "en") { await page.locator(".language-switcher select").selectOption(lang); await sleep(600); }
  await page.locator(".session-card", { hasText: ids[scenario] }).click();
  await sleep(900);
  await page.waitForFunction(() => Array.from(document.images).filter((i) => { const r = i.getBoundingClientRect(); return r.width > 0 && r.bottom > 0 && r.top < innerHeight; }).every((i) => i.complete && i.naturalWidth > 0) && !document.querySelector(".media-image[data-state='loading'], .media-image[data-state='retrying']"), null, { timeout: 15000 }).catch(() => console.log("  images not settled", scenario, vpName));
  if (prep) await prep(page);
  if (scroll) await page.evaluate(() => { const m = document.querySelector(".studio-shell main"); if (m) m.scrollTop = m.scrollHeight; });
  await sleep(350);
  const file = `${PREFIX}-${name ?? scenario}-${lang}-${vpName}${scroll ? "-scrolled" : ""}.jpg`;
  const check = await page.evaluate(() => {
    const overflow = document.documentElement.scrollWidth > innerWidth || (document.querySelector(".studio-shell main")?.scrollWidth ?? 0) > innerWidth;
    const clipped = Array.from(document.querySelectorAll(".session-detail button")).filter((b) => b.scrollWidth > b.clientWidth + 1).map((b) => b.textContent.trim().slice(0, 30));
    return { overflow, clipped };
  });
  if (check.overflow) problems.push(`${file}: horizontal overflow`);
  if (check.clipped.length) problems.push(`${file}: clipped ${check.clipped.join(" | ")}`);
  await page.screenshot({ path: `${OUT}/${file}`, type: "jpeg", quality: 80 });
  console.log("  wrote", file);
  if (lang !== "en") { await page.locator(".language-switcher select").selectOption("en"); await sleep(400); }
  await ctx.close();
}

try {
  for (const s of SCENARIOS) await shot(s, "1920x1080");
  await shot("four-photo-4x6", "1920x1080", { scroll: true });
  for (const vp of ["1600x900", "1366x768", "1280x720", "1080x1920", "win125-1920x1080", "win150-1920x1080"]) await shot("four-photo-4x6", vp);
  await shot("eight-photos", "1366x768");
  await shot("multiple-compositions", "1280x720");
  await shot("three-photo-2x6", "1080x1920");
  await shot("four-photo-4x6", "1920x1080", { lang: "vi" });
  await shot("multiple-compositions", "1366x768", { lang: "vi" });
  await shot("video-only", "1366x768", { lang: "vi" });
  await shot("three-photo-2x6", "1080x1920", { lang: "vi" });
  if (PREFIX === "after") {
    await shot("four-photo-4x6", "1366x768", { name: "overflow-menu", prep: async (p) => { await p.getByRole("button", { name: "More session actions" }).click(); } });
    await shot("four-photo-4x6", "1366x768", { name: "export-dialog", prep: async (p) => { await p.getByRole("button", { name: "Export session" }).click(); await p.getByRole("dialog").waitFor(); } });
    await shot("four-photo-4x6", "1366x768", { lang: "vi", name: "overflow-menu", prep: async (p) => { await p.getByRole("button", { name: "Thao tác khác cho phiên" }).click(); } });
  }
} finally {
  await browser.close();
  server.kill();
}
console.log(problems.length ? `PROBLEMS:\n${problems.join("\n")}` : "no overflow or clipped buttons detected");
