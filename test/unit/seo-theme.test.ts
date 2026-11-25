import { describe, expect, it } from "vitest";
import { alternatesFor, absolute, softwareApplication } from "../../src/lib/seo";
import { parsePref, resolveTheme, THEME_BOOTSTRAP } from "../../src/lib/client/theme";

describe("SEO helpers", () => {
  it("builds absolute canonical URLs on the production domain", () => {
    expect(absolute("/vi/features/")).toBe("https://photobooth.apexvoid.net/vi/features/");
  });

  it("lists every language plus x-default for a page", () => {
    expect(alternatesFor("/vi/download/")).toEqual([
      { hreflang: "en", href: "https://photobooth.apexvoid.net/download/" },
      { hreflang: "vi", href: "https://photobooth.apexvoid.net/vi/download/" },
      { hreflang: "x-default", href: "https://photobooth.apexvoid.net/download/" },
    ]);
  });

  it("structured data states only true facts: no price, rating, review, version or download count", () => {
    const ld = softwareApplication("en", "desc");
    expect(ld["@type"]).toBe("SoftwareApplication");
    expect(ld.operatingSystem).toBe("Windows 10, Windows 11");
    for (const k of ["offers", "aggregateRating", "review", "softwareVersion", "downloadUrl", "interactionStatistic"])
      expect(ld).not.toHaveProperty(k);
  });
});

describe("theme preference", () => {
  it("defaults anything unknown to system and resolves it from the OS", () => {
    expect(parsePref(null)).toBe("system");
    expect(parsePref("purple")).toBe("system");
    expect(parsePref("dark")).toBe("dark");
    expect(resolveTheme("system", true)).toBe("dark");
    expect(resolveTheme("system", false)).toBe("light");
    expect(resolveTheme("light", true)).toBe("light");
  });

  it("the pre-paint script sets the theme before anything renders and survives blocked storage", () => {
    const attrs: Record<string, string> = {};
    const root = { dataset: attrs, style: { colorScheme: "" } };
    const run = new Function("document", "localStorage", "window", "matchMedia", THEME_BOOTSTRAP);
    const mm = (dark: boolean) => () => ({ matches: dark });
    run({ documentElement: root }, { getItem: () => "light" }, { matchMedia: mm(true) }, mm(true));
    expect(attrs.theme).toBe("light");
    run(
      { documentElement: root },
      {
        getItem: () => {
          throw new Error("blocked");
        },
      },
      { matchMedia: mm(true) },
      mm(true),
    );
    expect(attrs.theme).toBe("dark");
    expect(attrs.themePref).toBe("system");
  });
});
