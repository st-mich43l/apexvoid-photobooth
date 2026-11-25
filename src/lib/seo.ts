import { SITE_URL } from "../config/site";
import { locales, localeNames, switchLocalePath, type Locale } from "../i18n/locales";

export interface Alternate {
  hreflang: string;
  href: string;
}

/** Absolute URL of a site path. */
export function absolute(path: string): string {
  return new URL(path, SITE_URL).toString();
}

/** hreflang alternates for a page: every locale plus x-default (English). */
export function alternatesFor(pathname: string): Alternate[] {
  const list: Alternate[] = locales.map((l) => ({ hreflang: localeNames[l].htmlLang, href: absolute(switchLocalePath(pathname, l)) }));
  list.push({ hreflang: "x-default", href: absolute(switchLocalePath(pathname, "en")) });
  return list;
}

/**
 * schema.org SoftwareApplication. Only facts that are true today: no price, rating, review, version or
 * download count (there is no release yet).
 */
export function softwareApplication(locale: Locale, description: string): Record<string, unknown> {
  return {
    "@context": "https://schema.org",
    "@type": "SoftwareApplication",
    name: "ApexVoid Photobooth",
    applicationCategory: "MultimediaApplication",
    operatingSystem: "Windows 10, Windows 11",
    processorRequirements: "x64",
    description,
    url: absolute(locale === "en" ? "/" : `/${locale}/`),
    image: absolute("/branding/og-image.png"),
    inLanguage: ["en", "vi"],
    sameAs: ["https://github.com/st-mich43l/apexvoid-photobooth"],
  };
}
