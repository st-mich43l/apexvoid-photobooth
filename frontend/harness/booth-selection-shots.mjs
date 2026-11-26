// Booth Mode capture + photo-selection workflow for BOTH a 2×6 strip (4 photos) and a 4×6 sheet (6 photos), out of 8 captures.
// Starts the real Go service headless (simulated camera, real Go compositor) and drives the real UI.
//   cd frontend && npx vite build -c vite.harness.config.ts
//   go build -tags uxharness -o /tmp/uxh ./cmd/uxharness           (from the repo root)
//   NODE_PATH=$(npm root -g) UXH=/tmp/uxh node harness/booth-selection-shots.mjs ../docs/img/booth01
import { createRequire } from "node:module";
import { spawn } from "node:child_process";
import { mkdirSync } from "node:fs";
const { chromium } = createRequire(import.meta.url)("playwright");

const OUT = process.argv[2] ?? "../docs/img/booth01";
const UXH = process.env.UXH ?? "/tmp/uxh";
const ONLY = process.argv[3]; // optional: "strip" | "sheet"
mkdirSync(OUT, { recursive: true });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const LAYOUTS = [{ key: "2x6", frame: "plain.strip4", need: 4, pick: [2, 4, 1, 7] }, { key: "4x6", frame: "plain.sheet6", need: 6, pick: [2, 4, 1, 7, 5, 8] }];
const VIEWPORTS = [{ key: "landscape", width: 1920, height: 1080 }, { key: "compact", width: 1366, height: 768 }, { key: "portrait", width: 1080, height: 1920 }];
const T = { en: { take: /Take photos/, cont: /^Continue$/, start: /Start shooting/ }, vi: { take: /Chụp ảnh/, cont: /^Tiếp tục$/, start: /Bắt đầu chụp/ } };

const server = spawn(UXH, ["-addr", "127.0.0.1:5311", "-dist", "dist-harness", "-shots", "8", "-tick", "300ms"], { stdio: "ignore" });
const BASE = "http://127.0.0.1:5311";
for (let i = 0; i < 60; i++) { try { await fetch(`${BASE}/rpc/BoothReadiness`, { method: "POST", body: "[]" }); break; } catch { await sleep(250); } }
const rpc = (m, ...a) => fetch(`${BASE}/rpc/${m}`, { method: "POST", body: JSON.stringify(a) });

async function settle(page) {
  await page.waitForFunction(() => Array.from(document.images).filter((i) => { const r = i.getBoundingClientRect(); return r.width > 0 && r.bottom > 0 && r.top < innerHeight; }).every((i) => i.complete && i.naturalWidth > 0), null, { timeout: 15000 }).catch(() => {});
  await sleep(300);
}
async function overflow(page, label) {
  const o = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth);
  if (o) console.log(`  !! horizontal overflow: ${label}`);
}

const browser = await chromium.launch();
async function run(layout, vp, lang) {
  const tag = `${layout.key}-${vp.key}${lang === "vi" ? "-vi" : ""}`;
  const t = T[lang];
  await rpc("PatchSettings", { guestLanguage: lang });
  const ctx = await browser.newContext({ viewport: { width: vp.width, height: vp.height }, deviceScaleFactor: 1, hasTouch: true });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => console.log("  pageerror:", e.message));
  const shot = async (name) => { await settle(page); await overflow(page, `${tag} ${name}`); await page.screenshot({ path: `${OUT}/${tag}-${name}.png` }); };
  await page.goto(`${BASE}/#booth`);
  await page.getByRole("button", { name: t.take }).click();
  await page.locator(`[data-frame="${layout.frame}"]`).click();
  await page.getByRole("button", { name: t.cont }).click();
  await page.getByRole("button", { name: t.start }).waitFor({ timeout: 20000 });
  await page.waitForFunction(() => !document.querySelector(".g-start")?.disabled);
  await sleep(1200);
  await shot("1-preview");
  await page.getByRole("button", { name: t.start }).click();
  // capture 7 of 8: the index that used to lose the framing guide on frames with fewer than seven photos
  await page.locator(".steps span", { hasText: /7/ }).first().waitFor({ timeout: 60000 });
  await sleep(200);
  await shot("2-capture-7-of-8");
  await page.locator(".g-pick-grid").waitFor({ timeout: 90000 });
  await shot("3-review-empty");
  const tap = async (n) => { await page.locator(`.g-pick-tile[data-slot="${n - 1}"]`).click(); await sleep(380); };
  for (const n of layout.pick.slice(0, Math.ceil(layout.need / 2))) await tap(n);
  await shot("4-review-partial");
  for (const n of layout.pick.slice(Math.ceil(layout.need / 2))) await tap(n);
  await shot("5-review-complete");
  await page.getByRole("button", { name: t.cont }).click();
  await page.locator(".g-result").waitFor({ timeout: 60000 });
  await sleep(1400);
  await shot("6-result");
  await ctx.close();
}
try {
  for (const layout of LAYOUTS) {
    if (ONLY && !layout.key.includes(ONLY === "strip" ? "2x6" : ONLY === "sheet" ? "4x6" : ONLY)) continue;
    for (const vp of VIEWPORTS) { console.log(layout.key, vp.key); await run(layout, vp, "en"); }
    console.log(layout.key, "landscape vi"); await run(layout, VIEWPORTS[0], "vi");
  }
} finally { await rpc("PatchSettings", { guestLanguage: "en" }).catch(() => {}); server.kill(); await browser.close(); }
console.log("done");
