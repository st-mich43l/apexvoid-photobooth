export const locales = ["en", "vi"] as const;
export type Locale = (typeof locales)[number];
export const defaultLocale: Locale = "en";

export const localeNames: Record<Locale, { native: string; short: string; htmlLang: string; og: string }> = {
  en: { native: "English", short: "EN", htmlLang: "en", og: "en_US" },
  vi: { native: "Tiếng Việt", short: "VI", htmlLang: "vi", og: "vi_VN" },
};

export function isLocale(v: string | undefined): v is Locale {
  return v !== undefined && (locales as readonly string[]).includes(v);
}

/** The page routes of the site. Paths are locale-neutral and always end with a slash. */
export const routes = {
  home: "/",
  features: "/features/",
  download: "/download/",
  releases: "/releases/",
  docs: "/docs/",
  community: "/community/",
  privacy: "/privacy/",
  licenses: "/licenses/",
} as const;
export type RouteKey = keyof typeof routes;
export const routeKeys = Object.keys(routes) as RouteKey[];

/** Path of a route in a locale: English has no prefix, Vietnamese lives under /vi/. */
export function pathFor(route: RouteKey, locale: Locale): string {
  const p = routes[route];
  return locale === defaultLocale ? p : `/${locale}${p}`;
}

/** Splits a site path into its locale and the locale-neutral rest. Unknown prefixes count as English. */
export function splitLocale(pathname: string): { locale: Locale; rest: string } {
  const clean = pathname.startsWith("/") ? pathname : `/${pathname}`;
  const m = /^\/([a-z]{2})(\/.*)?$/.exec(clean);
  if (m && isLocale(m[1]) && m[1] !== defaultLocale) return { locale: m[1], rest: m[2] ?? "/" };
  return { locale: defaultLocale, rest: clean };
}

/** The same page in another language (the language switcher keeps the reader where they are). */
export function switchLocalePath(pathname: string, target: Locale): string {
  const { rest } = splitLocale(pathname);
  const withSlash = rest.endsWith("/") ? rest : `${rest}/`;
  return target === defaultLocale ? withSlash : `/${target}${withSlash}`;
}
