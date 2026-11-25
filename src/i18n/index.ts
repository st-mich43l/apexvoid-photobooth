import { en, type Dict } from "./en";
import { vi } from "./vi";
import type { Locale } from "./locales";

export * from "./locales";
export type { Dict };

const dicts: Record<Locale, Dict> = { en, vi };

/** The dictionary of a locale. The only way components get text. */
export function useDict(locale: Locale): Dict {
  return dicts[locale];
}

/** Dates are formatted for the reader's language, in UTC so the static build is deterministic. */
export function formatDate(iso: string, locale: Locale): string {
  return new Intl.DateTimeFormat(locale === "vi" ? "vi-VN" : "en-GB", { dateStyle: "long", timeZone: "UTC" }).format(new Date(iso));
}
