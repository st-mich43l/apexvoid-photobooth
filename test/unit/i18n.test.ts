import { describe, expect, it } from "vitest";
import { en } from "../../src/i18n/en";
import { vi } from "../../src/i18n/vi";
import { pathFor, routeKeys, splitLocale, switchLocalePath } from "../../src/i18n/locales";

type Json = string | number | boolean | null | Json[] | { [k: string]: Json };

/** Every leaf path of a dictionary, with array lengths, so a missing item is caught as well as a missing key. */
function shape(v: Json, at = ""): string[] {
  if (Array.isArray(v)) return [`${at}[${v.length}]`, ...v.flatMap((x, i) => shape(x, `${at}[${i}]`))];
  if (v && typeof v === "object")
    return Object.keys(v)
      .sort()
      .flatMap((k) => shape((v as Record<string, Json>)[k] as Json, `${at}.${k}`));
  return [at];
}
function leaves(v: Json): string[] {
  if (Array.isArray(v)) return v.flatMap(leaves);
  if (v && typeof v === "object") return Object.values(v).flatMap((x) => leaves(x as Json));
  return typeof v === "string" ? [v] : [];
}

describe("translations", () => {
  it("Vietnamese has exactly the English keys and list lengths", () => {
    expect(shape(vi as unknown as Json)).toEqual(shape(en as unknown as Json));
  });

  it("has no empty or placeholder strings", () => {
    for (const d of [en, vi]) {
      for (const s of leaves(d as unknown as Json)) {
        expect(s.trim().length).toBeGreaterThan(0);
        expect(s).not.toMatch(/lorem|todo|tbd|xxx|placeholder/i);
      }
    }
  });

  it("Vietnamese text is actually Vietnamese (diacritics present) where it differs from English", () => {
    const viText = leaves(vi as unknown as Json).join(" ");
    expect(viText).toMatch(/[ăâđêôơưạảấầẩẫậắằẳẵặẹẻẽếềểễệỉịọỏốồổỗộớờởỡợụủứừửữựỳỵỷỹ]/);
    // nav labels are translated, not copied
    expect(vi.nav.features).not.toBe(en.nav.features);
    expect(vi.nav.download).not.toBe(en.nav.download);
  });

  it("product identity and page structure stay consistent across languages", () => {
    expect(vi.meta.siteName).toBe(en.meta.siteName);
    expect(vi.common.platform.arch).toBe(en.common.platform.arch);
    expect(vi.home.hardware.rows).toHaveLength(en.home.hardware.rows.length);
    expect(vi.docs.guides.map((g) => g.id)).toEqual(en.docs.guides.map((g) => g.id));
  });
});

describe("routes", () => {
  it("English has no prefix, Vietnamese lives under /vi/, every path ends with a slash", () => {
    for (const k of routeKeys) {
      expect(pathFor(k, "en").startsWith("/vi")).toBe(false);
      expect(pathFor(k, "vi").startsWith("/vi/")).toBe(true);
      expect(pathFor(k, "en").endsWith("/")).toBe(true);
      expect(pathFor(k, "vi").endsWith("/")).toBe(true);
    }
    expect(pathFor("home", "vi")).toBe("/vi/");
  });

  it("the language switch keeps the reader on the same page", () => {
    expect(switchLocalePath("/features/", "vi")).toBe("/vi/features/");
    expect(switchLocalePath("/vi/features/", "en")).toBe("/features/");
    expect(switchLocalePath("/vi/", "en")).toBe("/");
    expect(switchLocalePath("/", "vi")).toBe("/vi/");
    expect(switchLocalePath("/download", "vi")).toBe("/vi/download/");
  });

  it("splits locale prefixes, treating unknown prefixes as English paths", () => {
    expect(splitLocale("/vi/docs/")).toEqual({ locale: "vi", rest: "/docs/" });
    expect(splitLocale("/docs/")).toEqual({ locale: "en", rest: "/docs/" });
    expect(splitLocale("/vi")).toEqual({ locale: "vi", rest: "/" });
    expect(splitLocale("/vietnam/")).toEqual({ locale: "en", rest: "/vietnam/" });
  });
});
