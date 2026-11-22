// Drives the REAL guest flow (simulated camera, Go frame renderer, fake printer) in headless Chromium and writes screenshots.
//   cd frontend && npx vite build -c vite.harness.config.ts
//   go run -tags uxharness ./cmd/uxharness -addr 127.0.0.1:5199 -dist frontend/dist-harness &
//   NODE_PATH=$(npm root -g) node harness/shots.mjs ../docs/img/ux2
import { createRequire } from "node:module";
import { mkdirSync } from "node:fs";
const { chromium } = createRequire(import.meta.url)("playwright");

const BASE = process.env.BASE ?? "http://127.0.0.1:5199";
const OUT = process.argv[2] ?? "../docs/img/ux2";
const ONLY = process.argv[3]; // optional: one viewport name
mkdirSync(OUT, { recursive: true });

const VIEWPORTS = {
  landscape: { width: 1920, height: 1080 },
  portrait: { width: 1080, height: 1920 },
  "small-landscape": { width: 1366, height: 768 },
  "compact-portrait": { width: 768, height: 1024 },
  "scaled-125": { width: 1536, height: 864 },
  "scaled-150": { width: 1280, height: 720 },
};
const MAIN = new Set(["landscape", "portrait"]);
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const unplug = (present) => fetch(`${BASE}/dev/camera?present=${present ? 1 : 0}`);

async function settled(page) {
  // only images in view: lazy ones below the fold are deliberately not loaded yet
  await page.waitForFunction(() => Array.from(document.images).filter((i) => { const r = i.getBoundingClientRect(); return r.width > 0 && r.bottom > 0 && r.top < innerHeight; }).every((i) => i.complete && i.naturalWidth > 0), null, { timeout: 15000 });
  await sleep(250);
}
async function noOverflow(page, label) {
  const o = await page.evaluate(() => ({ x: document.documentElement.scrollWidth > innerWidth, b: document.body.scrollWidth > innerWidth }));
  if (o.x || o.b) console.log(`  !! horizontal overflow at ${label}`);
  const clipped = await page.evaluate(() => Array.from(document.querySelectorAll(".booth-root button:not(.g-scroll-area button)")).filter((b) => {
    const r = b.getBoundingClientRect(); return r.width > 0 && (r.right > innerWidth + 1 || r.bottom > innerHeight + 1 || r.left < -1 || r.top < -1);
  }).map((b) => b.textContent.trim().slice(0, 24)));
  if (clipped.length) console.log(`  !! clipped buttons at ${label}: ${clipped.join(", ")}`);
}

const browser = await chromium.launch();
for (const [name, vp] of Object.entries(VIEWPORTS)) {
  if (ONLY && ONLY !== name) continue;
  const main = MAIN.has(name);
  const tag = (s) => `${OUT}/${s}-${name}.png`;
  const ctx = await browser.newContext({ viewport: vp, deviceScaleFactor: 1, hasTouch: true });
  const page = await ctx.newPage();
  page.on("pageerror", (e) => console.log("  pageerror:", e.message));
  console.log(name, vp.width + "x" + vp.height);
  await page.goto(`${BASE}/#booth`);
  const take = page.getByRole("button", { name: /Take photos/ });
  await take.waitFor();
  await page.waitForFunction(() => !document.querySelector(".g-start")?.disabled);
  await sleep(1100); // entrance animation done
  await page.screenshot({ path: tag("01-welcome") }); await noOverflow(page, "welcome");

  await take.click();
  await page.getByRole("radio").first().waitFor();
  await settled(page);
  await page.screenshot({ path: tag("02-frames") }); await noOverflow(page, "frames");
  // choose a frame with colour variants and capture the selection state
  const cards = page.getByRole("radio", { name: /frame/ });
  if ((await cards.count()) > 2) { await cards.nth(2).click(); await sleep(300); await settled(page); if (main) await page.screenshot({ path: tag("02b-frames-selected") }); }
  await page.getByRole("button", { name: /Continue|Use the first/ }).click();

  const start = page.getByRole("button", { name: /Start shooting/ });
  await start.waitFor({ timeout: 20000 });
  await page.waitForFunction(() => !document.querySelector(".g-start")?.disabled);
  await sleep(1200);
  await page.screenshot({ path: tag("03-preview") }); await noOverflow(page, "preview");

  if (main) { // camera loss while the live view is up: the calm reconnecting state
    await unplug(false); await page.getByText(/Reconnecting the camera/).waitFor({ timeout: 8000 }); await sleep(400);
    await page.screenshot({ path: tag("07-reconnecting") }); await noOverflow(page, "reconnecting");
    await unplug(true);
    await page.waitForFunction(() => !document.querySelector(".g-start")?.disabled, null, { timeout: 30000 }).catch(() => {});
    await sleep(800);
  }
  await start.click();
  await page.locator(".g-count span").first().waitFor({ timeout: 8000 });
  await sleep(250);
  await page.screenshot({ path: tag("04-countdown") }); await noOverflow(page, "countdown");

  await page.getByText("How do they look?").waitFor({ timeout: 60000 });
  await settled(page); await sleep(300);
  await page.screenshot({ path: tag("05-review") }); await noOverflow(page, "review");

  await page.getByRole("button", { name: /Looks good/ }).click();
  await page.getByText("All done!").waitFor({ timeout: 30000 });
  await settled(page); await sleep(1400);
  await page.screenshot({ path: tag("06-result") }); await noOverflow(page, "result");

  await page.getByRole("button", { name: "Finish" }).click();
  await take.waitFor();
  await ctx.close();
}
await browser.close();
console.log("done");
