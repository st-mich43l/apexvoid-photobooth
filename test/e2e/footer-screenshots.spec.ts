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
    test(`before: ${scenario.name}`, async ({ page }) => {
      await mkdir("artifacts/footer-shots/before", { recursive: true });
      await page.setViewportSize({ width: scenario.width, height: scenario.height });
      await page.emulateMedia({ colorScheme: scenario.scheme });
      await page.goto(scenario.path);
      const footer = page.locator("footer");
      await expect(footer).toBeVisible();
      await footer.screenshot({ path: `artifacts/footer-shots/before/${scenario.name}.png` });
    });
  }
});
