import { expect, test, type Page } from "@playwright/test";
import AxeBuilder from "@axe-core/playwright";

const ROUTES = ["/", "/features/", "/download/", "/releases/", "/docs/", "/community/", "/privacy/", "/licenses/"];
const ALL = [...ROUTES, ...ROUTES.map((r) => `/vi${r}`)];
const WIDTHS = [320, 375, 390, 430, 768, 1024, 1440, 1920];

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
  for (const path of ["/download/", "/vi/download/", "/releases/"]) {
    test(`${path} shows the honest empty state and no invented release facts`, async ({ page }) => {
      await page.goto(path);
      const main = page.locator("main");
      await expect(main.locator('[data-download-state="empty"], [data-releases-state="empty"]')).toHaveCount(1);
      const text = (await main.textContent()) ?? "";
      expect(text).not.toMatch(/\bv\d+\.\d+\.\d+\b/);
      expect(text).not.toMatch(/\b[a-f0-9]{64}\b/);
      expect(text).not.toMatch(/\d+(\.\d+)?\s?(MB|GB)\b/);
    });
  }
  test("the hero download action leads to the download page", async ({ page }) => {
    await page.goto("/");
    await page.getByRole("link", { name: "Download for Windows" }).click();
    await expect(page).toHaveURL(/\/download\/$/);
    await expect(page.getByText("Official public releases are not available yet.")).toBeVisible();
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
