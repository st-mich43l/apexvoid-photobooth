// Captures the built website (served by `npm run preview`) for visual review:
//   npm run build && (npm run preview -- --port 4330 &) && node scripts/site-screenshots.mjs out-dir [paths...]
// Writes <out>/<name>-<viewport>-<theme>.png, full page, and reports horizontal overflow.
import { chromium } from "@playwright/test";
import { mkdirSync } from "node:fs";

const BASE = process.env.BASE ?? "http://127.0.0.1:4330";
const OUT = process.argv[2] ?? "site-shots";
const PATHS = process.argv.slice(3).length ? process.argv.slice(3) : ["/", "/vi/"];
const VIEWPORTS = (process.env.VIEWPORTS ?? "desktop:1440x900,mobile:390x844").split(",").map((v) => {
  const [name, size] = v.split(":");
  const [width, height] = size.split("x").map(Number);
  return { name, width, height };
});
const THEMES = (process.env.THEMES ?? "light,dark").split(",");
mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch();
for (const vp of VIEWPORTS) {
  for (const theme of THEMES) {
    const ctx = await browser.newContext({
      viewport: { width: vp.width, height: vp.height },
      deviceScaleFactor: 1,
      colorScheme: theme === "dark" ? "dark" : "light",
      reducedMotion: "reduce",
    });
    const page = await ctx.newPage();
    page.on("console", (m) => m.type() === "error" && console.log("console error", m.text()));
    for (const p of PATHS) {
      await page.goto(BASE + p, { waitUntil: "networkidle" });
      // load lazy images before a full-page capture
      await page.evaluate(async () => {
        for (let y = 0; y < document.body.scrollHeight; y += 600) {
          window.scrollTo(0, y);
          await new Promise((r) => setTimeout(r, 60));
        }
        window.scrollTo(0, 0);
      });
      await page.waitForTimeout(300);
      const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
      if (overflow > 0) console.log(`!! horizontal overflow ${overflow}px on ${p} at ${vp.name}`);
      const name = p === "/" ? "home" : p.replace(/^\/|\/$/g, "").replace(/\//g, "-");
      await page.screenshot({ path: `${OUT}/${name}-${vp.name}-${theme}.png`, fullPage: true });
    }
    await ctx.close();
  }
}
await browser.close();
console.log("screenshots written to", OUT);
