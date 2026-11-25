import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";
import { readFileSync } from "node:fs";
import { join } from "node:path";

const ROUTES = ["/", "/features/", "/download/", "/releases/", "/docs/", "/community/", "/privacy/", "/licenses/"];
const ALL = [...ROUTES, ...ROUTES.map((r) => `/vi${r}`)];
const WIDTHS = [320, 375, 390, 430, 768, 1024, 1440, 1920];

/** Release manifests shared with the publisher (ansible-library fixtures). */
const manifest = (name: string): string => readFileSync(join(import.meta.dirname, "../fixtures/manifest", name), "utf8");

/** The download pages read /downloads/index.json at runtime. By default every test sees the empty manifest
 *  (production before the first release); download tests override it. Later routes win in Playwright. */
async function serveManifest(page: Page, body: string, status = 200): Promise<void> {
  await page.route("**/downloads/index.json", (r) => r.fulfill({ status, contentType: "application/json", body }));
}
test.beforeEach(async ({ page }) => serveManifest(page, manifest("manifest-empty.json")));

/** Fails the test on any console error or uncaught page error. */
function watchConsole(page: Page): string[] {
  const errors: string[] = [];
  page.on("console", (m) => m.type() === "error" && errors.push(m.text()));
  page.on("pageerror", (e) => errors.push(e.message));
  return errors;
}

test.describe("routes", () => {
  for (const path of ALL) {
    test(`${path} renders with metadata and no console errors`, async ({ page }) => {
      const errors = watchConsole(page);
      const res = await page.goto(path);
      expect(res?.status()).toBe(200);
      const vi = path.startsWith("/vi/");
      await expect(page.locator("html")).toHaveAttribute("lang", vi ? "vi" : "en");
      expect((await page.title()).length).toBeGreaterThan(10);
      await expect(page.locator("h1")).toHaveCount(1);
      await expect(page.locator('meta[name="description"]')).toHaveAttribute("content", /.{40,}/);
      await expect(page.locator('link[rel="canonical"]')).toHaveAttribute("href", `https://photobooth.apexvoid.net${path}`);
      await expect(page.locator('link[rel="alternate"][hreflang]')).toHaveCount(3);
      await expect(page.locator('meta[property="og:image"]')).toHaveAttribute(
        "content",
        "https://photobooth.apexvoid.net/branding/og-image.png",
      );
      // every image has explicit dimensions (no layout shift)
      const sizeless = await page.locator("img:not([width]), img:not([height])").count();
      expect(sizeless).toBe(0);
      expect(errors).toEqual([]);
    });
  }

  test("titles are unique per page and language", async ({ page }) => {
    const titles = new Set<string>();
    for (const p of ALL) {
      await page.goto(p);
      titles.add(await page.title());
    }
    expect(titles.size).toBe(ALL.length);
  });

  test("an unknown address gets the 404 page", async ({ page }) => {
    const res = await page.goto("/no-such-page/");
    expect(res?.status()).toBe(404);
    await expect(page.getByRole("heading", { level: 1 })).toContainText("isn't in the frame");
  });

  test("the home page carries truthful SoftwareApplication data", async ({ page }) => {
    await page.goto("/");
    const ld = JSON.parse((await page.locator('script[type="application/ld+json"]').textContent()) ?? "{}");
    expect(ld["@type"]).toBe("SoftwareApplication");
    expect(ld).not.toHaveProperty("offers");
    expect(ld).not.toHaveProperty("aggregateRating");
    expect(ld).not.toHaveProperty("softwareVersion");
  });
});

test.describe("language", () => {
  test("the switcher keeps the current page and remembers the choice", async ({ page, isMobile }) => {
    await page.goto("/features/");
    if (isMobile) await page.getByRole("button", { name: "Open menu" }).click();
    await page.locator('[data-locale-link="vi"]').locator("visible=true").first().click();
    await expect(page).toHaveURL(/\/vi\/features\/$/);
    await expect(page.locator("html")).toHaveAttribute("lang", "vi");
    expect(await page.evaluate(() => localStorage.getItem("apx-locale"))).toBe("vi");
    if (isMobile) await page.getByRole("button", { name: "Mở menu" }).click();
    await page.locator('[data-locale-link="en"]').locator("visible=true").first().click();
    await expect(page).toHaveURL(/\/features\/$/);
    await expect(page).not.toHaveURL(/\/vi\//);
  });

  test("never redirects on the browser language", async ({ browser }) => {
    const ctx = await browser.newContext({ locale: "vi-VN" });
    const page = await ctx.newPage();
    await page.goto("/");
    await expect(page).toHaveURL(/\/$/);
    await expect(page).not.toHaveURL(/\/vi\//);
    await ctx.close();
  });

  test("navigation is fully translated", async ({ page }) => {
    await page.goto("/vi/");
    const header = page.locator("header.site-header");
    for (const word of ["Features", "Download", "Documentation", "Community"]) await expect(header).not.toContainText(word);
    await expect(page.locator("footer")).toContainText("Quyền riêng tư");
  });
});

test.describe("theme", () => {
  test("first visit follows the operating system", async ({ browser }) => {
    for (const scheme of ["dark", "light"] as const) {
      const ctx = await browser.newContext({ colorScheme: scheme });
      const page = await ctx.newPage();
      await page.goto("/", { waitUntil: "domcontentloaded" });
      await expect(page.locator("html")).toHaveAttribute("data-theme", scheme);
      await ctx.close();
    }
  });

  test("a chosen theme persists and is applied before the page renders", async ({ page, isMobile }) => {
    await page.emulateMedia({ colorScheme: "light" });
    await page.goto("/download/");
    if (isMobile) await page.getByRole("button", { name: "Open menu" }).click();
    const dark = page.getByRole("button", { name: "Dark" }).locator("visible=true").first();
    await dark.click();
    await expect(dark).toHaveAttribute("aria-pressed", "true");
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
    // a reload: the inline head script must have applied it before any module script runs
    await page.goto("/features/", { waitUntil: "commit" });
    await page.waitForSelector("body", { state: "attached" });
    expect(await page.evaluate(() => document.documentElement.dataset.theme)).toBe("dark");
    // back to system
    if (isMobile) await page.getByRole("button", { name: "Open menu" }).click();
    await page.getByRole("button", { name: "System" }).locator("visible=true").first().click();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
    await page.emulateMedia({ colorScheme: "dark" });
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  });
});

test.describe("mobile navigation", () => {
  test.skip(({ isMobile }) => !isMobile, "phone layout only");

  test("opens, closes with Escape and returns focus, and navigates", async ({ page }) => {
    await page.goto("/");
    const toggle = page.getByRole("button", { name: "Open menu" });
    await expect(page.locator("#mobile-nav")).toBeHidden();
    await toggle.click();
    await expect(page.locator("#mobile-nav")).toBeVisible();
    await expect(page.getByRole("button", { name: "Close menu" })).toHaveAttribute("aria-expanded", "true");
    await page.keyboard.press("Escape");
    await expect(page.locator("#mobile-nav")).toBeHidden();
    await expect(page.getByRole("button", { name: "Open menu" })).toBeFocused();
    await page.getByRole("button", { name: "Open menu" }).click();
    await page.locator("#mobile-nav").getByRole("link", { name: "Documentation" }).click();
    await expect(page).toHaveURL(/\/docs\/$/);
  });

  test("touch targets in the menu are at least 44 px tall", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("button", { name: "Open menu" }).click();
    const heights = await page
      .locator("#mobile-nav a, #mobile-nav button, #mobile-nav [aria-current]")
      .evaluateAll((els) => els.map((e) => e.getBoundingClientRect().height));
    expect(Math.min(...heights)).toBeGreaterThanOrEqual(44);
  });
});

test.describe("download", () => {
  const panel = (page: Page) => page.locator("[data-download-panel]");
  const list = (page: Page) => page.locator("[data-release-list]");
  const visibleStates = (page: Page) => page.locator("main [data-when]:visible");

  for (const path of ["/download/", "/vi/download/", "/releases/", "/vi/releases/"]) {
    test(`${path} with the empty manifest shows the honest empty state and no invented release facts`, async ({ page }) => {
      const errors = watchConsole(page);
      await page.goto(path);
      await expect(page.locator("main [data-state]")).toHaveAttribute("data-state", "empty");
      await expect(visibleStates(page)).toHaveCount(1);
      const text = (await page.locator("main").innerText()) ?? "";
      expect(text).not.toMatch(/\bv?\d+\.\d+\.\d+\b/);
      expect(text).not.toMatch(/\b[a-f0-9]{64}\b/);
      expect(text).not.toMatch(/\d+(\.\d+)?\s?(MB|GB)\b/);
      await expect(page.locator('main a[href*="/downloads/releases/"]')).toHaveCount(0);
      expect(errors).toEqual([]);
    });
  }

  test("a published stable release is offered with its facts, checksum and unsigned notice", async ({ page }) => {
    await serveManifest(page, manifest("manifest-valid.json"));
    await page.goto("/download/");
    const p = panel(page);
    await expect(p).toHaveAttribute("data-state", "available");
    await expect(visibleStates(page)).toHaveCount(1);
    await expect(p.getByRole("heading", { level: 2 })).toHaveText("ApexVoid Photobooth 0.2.1");
    await expect(p).toContainText("v0.2.1");
    await expect(p).toContainText("3e23e8160039594a33894f6564e1b1348bbd7a0088d42c4acb73eeaed59c009d");
    await expect(p).toContainText("976.6 KB");
    await expect(p.locator("time")).toHaveAttribute("datetime", "2027-01-20T09:00:00Z");
    const dl = p.getByRole("link", { name: "Download for Windows" });
    await expect(dl).toHaveAttribute("href", "/downloads/releases/v0.2.1/ApexVoidPhotobooth-v0.2.1-windows-x64.exe");
    await expect(p.getByRole("link", { name: "SHA256SUMS" })).toHaveAttribute("href", "/downloads/releases/v0.2.1/SHA256SUMS");
    await expect(p.getByRole("link", { name: "Release notes" })).toHaveAttribute("href", "/downloads/releases/v0.2.1/RELEASE-NOTES.md");
    await expect(p.getByRole("link", { name: "Third-party notices" })).toHaveAttribute(
      "href",
      "/downloads/releases/v0.2.1/THIRD-PARTY-NOTICES.txt",
    );
    await expect(p.getByText("Not code-signed.")).toBeVisible();
    await expect(p.getByText("Authenticode")).toBeHidden();
    await expect(p.getByText("Does not include the Canon EDSDK.", { exact: false })).toBeVisible();
    await expect(p.locator("code")).toContainText("Get-FileHash .\\ApexVoidPhotobooth-v0.2.1-windows-x64.exe -Algorithm SHA256");
    // the newer preview is never offered as the current download
    await expect(p).not.toContainText("0.3.0-rc.1");
  });

  test("the release history lists every release newest first with current and preview labels", async ({ page }) => {
    await serveManifest(page, manifest("manifest-valid.json"));
    await page.goto("/releases/");
    await expect(list(page)).toHaveAttribute("data-state", "list");
    const items = list(page).locator("li");
    await expect(items).toHaveCount(3);
    await expect(items.locator("h2")).toHaveText([/0\.3\.0-rc\.1\s*Preview/, /0\.2\.1\s*Current/, /0\.2\.0$/]);
    await expect(items.nth(2).getByRole("link", { name: "Download" })).toHaveAttribute(
      "href",
      "/downloads/releases/v0.2.0/ApexVoidPhotobooth-v0.2.0-windows-x64.exe",
    );
  });

  test("with only a preview, it is offered clearly labelled as a preview", async ({ page }) => {
    const m = JSON.parse(manifest("manifest-valid.json"));
    m.releases = m.releases.filter((r: { channel: string }) => r.channel === "preview");
    m.latest.stable = null;
    await serveManifest(page, JSON.stringify(m));
    await page.goto("/download/");
    await expect(panel(page)).toHaveAttribute("data-state", "preview-only");
    await expect(panel(page).getByText("Preview build")).toBeVisible();
    await expect(panel(page).getByText("Latest release")).toBeHidden();
    await expect(panel(page).getByText(/No stable release has been published yet/)).toBeVisible();
  });

  test("an unreachable manifest offers nothing and can be retried", async ({ page }) => {
    await serveManifest(page, "", 503);
    await page.goto("/download/");
    await expect(panel(page)).toHaveAttribute("data-state", "unavailable");
    await expect(page.locator('main a[href*="/downloads/releases/"]')).toHaveCount(0);
    await serveManifest(page, manifest("manifest-valid.json"));
    await panel(page).getByRole("button", { name: "Try again" }).click();
    await expect(panel(page)).toHaveAttribute("data-state", "available");
  });

  for (const [label, body] of [
    ["a manifest that fails the shared schema", manifest("manifest-invalid-absolute-url.json")],
    ["a manifest that links outside the release directory", manifest("manifest-invalid-traversal.json")],
    ["a body that is not JSON", "<html>oops</html>"],
  ] as const) {
    test(`${label} is shown as invalid, with nothing from it on the page`, async ({ page }) => {
      await serveManifest(page, body);
      for (const path of ["/download/", "/releases/"]) {
        await page.goto(path);
        await expect(page.locator("main [data-state]")).toHaveAttribute("data-state", "invalid");
        await expect(page.locator('main a[href*="/downloads/releases/"], main a[href*="evil"]')).toHaveCount(0);
        expect(await page.locator("main").innerText()).not.toMatch(/\b[a-f0-9]{64}\b/);
      }
    });
  }

  test("Vietnamese pages show the release in Vietnamese", async ({ page }) => {
    await serveManifest(page, manifest("manifest-valid.json"));
    await page.goto("/vi/download/");
    await expect(panel(page)).toHaveAttribute("data-state", "available");
    await expect(panel(page).getByText("Chưa ký số.")).toBeVisible();
    await expect(panel(page).locator("time")).toHaveText(/2027/);
  });

  test("the hero download action leads to the download page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Download for Windows" }).click();
    await expect(page).toHaveURL(/\/download\/$/);
    await expect(page.getByText("No official release has been published yet.")).toBeVisible();
  });

  test.describe("accessibility with a published release", () => {
    test.skip(({ isMobile }) => isMobile, "run once, both themes");
    for (const path of ["/download/", "/releases/"]) {
      for (const scheme of ["light", "dark"] as const) {
        test(`axe: ${path} (${scheme})`, async ({ page }) => {
          await serveManifest(page, manifest("manifest-valid.json"));
          await page.emulateMedia({ colorScheme: scheme, reducedMotion: "reduce" });
          await page.goto(path);
          await expect(page.locator("main [data-state]")).not.toHaveAttribute("data-state", "loading");
          const r = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"]).analyze();
          expect(r.violations.map((v) => `${v.id}: ${v.nodes[0]?.target.join(" ")} — ${v.help}`)).toEqual([]);
        });
      }
    }
  });
});

test.describe("layout", () => {
  test.skip(({ isMobile }) => isMobile, "widths are set explicitly");
  for (const width of WIDTHS) {
    test(`no horizontal overflow at ${width}px`, async ({ page }) => {
      for (const height of [800, Math.round(width * 0.6)]) {
        await page.setViewportSize({ width, height: Math.max(height, 480) });
        for (const path of ["/", "/features/", "/download/", "/vi/", "/vi/community/", "/licenses/"]) {
          await page.goto(path);
          const over = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
          expect(over, `${path} at ${width}px`).toBeLessThanOrEqual(0);
        }
      }
    });
  }

  test("a published download release fits phone widths without clipped content", async ({ page }) => {
    await serveManifest(page, manifest("manifest-valid.json"));
    for (const width of [320, 360, 375, 390, 430]) {
      await page.setViewportSize({ width, height: 840 });
      await page.goto("/download/");
      await expect(panel(page)).toHaveAttribute("data-state", "available");
      const overflow = await page.evaluate(() => {
        const viewport = document.documentElement.clientWidth;
        return Array.from(document.querySelectorAll("main *"))
          .filter((element) => {
            if (element.closest("pre")) return false;
            const rect = element.getBoundingClientRect();
            const style = getComputedStyle(element);
            return rect.width > 0 && style.display !== "none" && style.visibility !== "hidden" &&
              element.getClientRects().length > 0 && (rect.left < -1 || rect.right > viewport + 1);
          })
          .map((element) => ({
            tag: element.tagName.toLowerCase(),
            className: typeof element.className === "string" ? element.className : "",
            right: Math.round(element.getBoundingClientRect().right),
          }))
          .slice(0, 8);
      });
      expect(overflow, `clipped elements at ${width}px`).toEqual([]);
    }
  });
});

test.describe("accessibility", () => {
  test.skip(({ isMobile }) => isMobile, "run once, both themes");
  for (const path of ["/", "/features/", "/download/", "/docs/", "/community/", "/privacy/", "/licenses/", "/releases/", "/vi/"]) {
    for (const scheme of ["light", "dark"] as const) {
      test(`axe: ${path} (${scheme})`, async ({ page }) => {
        await page.emulateMedia({ colorScheme: scheme, reducedMotion: "reduce" });
        await page.goto(path);
        const r = await new AxeBuilder({ page }).withTags(["wcag2a", "wcag2aa", "wcag21a", "wcag21aa", "wcag22aa"]).analyze();
        const issues = r.violations.map((v) => `${v.id}: ${v.nodes.length} × ${v.nodes[0]?.target.join(" ")} — ${v.help}`);
        expect(issues).toEqual([]);
      });
    }
  }

  test("keyboard: the skip link is first and moves to the content", async ({ page }) => {
    await page.goto("/");
    await page.keyboard.press("Tab");
    const skip = page.getByRole("link", { name: "Skip to content" });
    await expect(skip).toBeFocused();
    await expect(skip).toBeVisible();
    await page.keyboard.press("Enter");
    await expect(page).toHaveURL(/#main$/);
  });
});
