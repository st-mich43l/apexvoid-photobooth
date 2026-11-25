import { mkdir } from "node:fs/promises";
import { test, expect } from "@playwright/test";

test.describe("footer screenshots", () => {
  test.skip(({ isMobile }) => isMobile, "viewport scenarios are set explicitly");
  const cases = [
    { name: "mobile-en-dark-390", path: "/download/", width: 390, height: 844, scheme: "dark" as const },
    { name: "mobile-vi-dark-390", path: "/vi/download/", width: 390, height: 844, scheme: "dark" as const },
    { name: "mobile-en-light-390", path: "/download/", width: 390, height: 844, scheme: "light" as const },
    { name: "tablet-en-light-768", path: "/download/", width: 768, height: 1024, scheme: "light" as const },
    { name: "desktop-en-light-1440", path: "/download/", width: 1440, height: 900, scheme: "light" as const },
  ];

  for (const scenario of cases) {
    test(`after: ${scenario.name}`, async ({ page }) => {
      await mkdir("artifacts/footer-shots/after", { recursive: true });
      await page.setViewportSize({ width: scenario.width, height: scenario.height });
      await page.emulateMedia({ colorScheme: scenario.scheme });
      await page.goto(scenario.path);
      const footer = page.locator("footer");
      await expect(footer).toBeVisible();
      await footer.screenshot({ path: `artifacts/footer-shots/after/${scenario.name}.png` });
    });
  }
});

test.describe("footer behavior", () => {
  test.skip(({ isMobile }) => isMobile, "viewport scenarios are set explicitly");

  test("keeps all localized destinations visible without overflow at requested widths", async ({ page }) => {
    for (const width of [320, 360, 390, 430, 768, 1024, 1440, 1920]) {
      await page.setViewportSize({ width, height: 900 });
      await page.goto("/download/");
      const footer = page.locator("footer");
      const visibleLinkHrefs = await footer.locator("a:visible").evaluateAll((links) =>
        links.map((link) => (link as HTMLAnchorElement).pathname),
      );
      for (const href of ["/features/", "/download/", "/releases/", "/docs/", "/community/", "/privacy/", "/licenses/"]) {
        expect(visibleLinkHrefs.filter((value) => value === href), `${href} visible at ${width}px`).toHaveLength(1);
      }
      expect(await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth), `overflow at ${width}px`).toBeLessThanOrEqual(0);
      const clipped = await footer.locator("a:visible").evaluateAll((links) =>
        links.filter((link) => {
          const rect = link.getBoundingClientRect();
          return rect.left < -1 || rect.right > window.innerWidth + 1;
        }).length,
      );
      expect(clipped, `clipped footer links at ${width}px`).toBe(0);
    }
  });

  test("footer language switch preserves the current localized route", async ({ page }) => {
    await page.goto("/download/");
    const footer = page.locator("footer");
    await footer.locator('[data-locale-link="vi"]').click();
    await expect(page).toHaveURL(new RegExp("/vi/download/$"));
    await expect(page.locator("html")).toHaveAttribute("lang", "vi");
    await page.locator("footer [data-locale-link=\"en\"]").click();
    await expect(page).toHaveURL(new RegExp("/download/$"));
    await expect(page.locator("html")).toHaveAttribute("lang", "en");
  });
});
