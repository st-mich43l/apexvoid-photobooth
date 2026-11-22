// Screenshots of the OPERATOR (Studio) screens. Starts the real Go service headless (cmd/uxharness) once per scenario.
//   cd frontend && npx vite build -c vite.harness.config.ts
//   go build -tags uxharness -o /tmp/uxh ./cmd/uxharness      (from the repo root)
//   NODE_PATH=$(npm root -g) UXH=/tmp/uxh node harness/operator-shots.mjs ../docs/img/ux3
// The "webcam" scenarios use a harness stand-in (the mock camera presented as a USB webcam): they show how the screens look
// for a non-simulated UVC camera, not that a real webcam works.
import { createRequire } from "node:module";
import { spawn } from "node:child_process";
import { mkdirSync } from "node:fs";
const { chromium } = createRequire(import.meta.url)("playwright");

const OUT = process.argv[2] ?? "../docs/img/ux3";
const UXH = process.env.UXH ?? "/tmp/uxh";
mkdirSync(OUT, { recursive: true });
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const LAND = { width: 1440, height: 900 }, PORT = { width: 834, height: 1112 };

async function withServer(port, args, fn) {
  const p = spawn(UXH, ["-addr", `127.0.0.1:${port}`, "-dist", "dist-harness", "-few-frames", ...args], { stdio: "ignore", cwd: process.cwd() });
  const base = `http://127.0.0.1:${port}`;
  for (let i = 0; i < 60; i++) { try { await fetch(`${base}/rpc/BoothReadiness`, { method: "POST", body: "[]" }); break; } catch { await sleep(250); } }
  try { await fn(base); } finally { p.kill(); }
}

async function shot(browser, base, hash, vp, name, prep) {
  const ctx = await browser.newContext({ viewport: vp, deviceScaleFactor: 1 });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => console.log("  pageerror:", e.message));
  await page.goto(`${base}/#${hash}`);
  await page.locator("nav[aria-label='Studio navigation']").waitFor();
  await sleep(1500);
  if (prep) await prep(page);
  await page.waitForFunction(() => Array.from(document.images).filter((i) => { const r = i.getBoundingClientRect(); return r.width > 0 && r.top < innerHeight; }).every((i) => i.complete && i.naturalWidth > 0), null, { timeout: 15000 }).catch(() => {});
  await sleep(400);
  const o = await page.evaluate(() => document.documentElement.scrollWidth > innerWidth);
  if (o) console.log(`  !! horizontal overflow: ${name}`);
  await page.screenshot({ path: `${OUT}/${name}.png` });
  await ctx.close();
}

const browser = await chromium.launch();
// 1. webcam + printer: everything ready
await withServer(5211, ["-webcam"], async (b) => {
  await shot(browser, b, "prepare", LAND, "prepare-ready-landscape");
  await shot(browser, b, "camera", LAND, "camera-setup-landscape");
  await shot(browser, b, "printer", LAND, "printer-setup-landscape");
  await shot(browser, b, "dashboard", LAND, "dashboard-landscape");
  await shot(browser, b, "prepare", PORT, "prepare-ready-portrait");
  await shot(browser, b, "dashboard", PORT, "studio-navigation-portrait");
  await shot(browser, b, "camera", PORT, "camera-setup-portrait");
});
// 2. webcam, digital only (no printer configured)
await withServer(5212, ["-webcam", "-printer=false"], async (b) => { await shot(browser, b, "prepare", LAND, "prepare-webcam-digital-only-landscape"); });
// 3. nothing connected: setup required
await withServer(5213, ["-no-camera", "-printer=false"], async (b) => {
  await shot(browser, b, "prepare", LAND, "prepare-setup-required-landscape");
  await shot(browser, b, "prepare", PORT, "prepare-setup-required-portrait");
  await shot(browser, b, "prepare", LAND, "open-booth-blocked-landscape", async (p) => { await p.getByRole("button", { name: /Open Booth Mode/ }).first().click(); await p.getByRole("dialog").waitFor(); });
});
// 4. simulated camera: demo / test mode
await withServer(5214, [], async (b) => {
  await shot(browser, b, "prepare", LAND, "prepare-demo-landscape");
  await shot(browser, b, "camera", LAND, "camera-setup-demo-landscape");
});
await browser.close();
console.log("done");
