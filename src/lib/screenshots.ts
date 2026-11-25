import type { ImageMetadata } from "astro";
import type { Locale } from "../i18n/locales";

/** Application screenshots, keyed "<name>.<locale>". A Vietnamese page uses the Vietnamese capture when one
 *  exists and falls back to the English one otherwise. All are real renders of the desktop UI (see README). */
const files = import.meta.glob<{ default: ImageMetadata }>("../assets/screenshots/*.png", { eager: true });

export const shotNames = [
  "booth-welcome",
  "booth-welcome-portrait",
  "booth-frames",
  "booth-countdown",
  "booth-review",
  "booth-result",
  "booth-reconnecting",
  "studio-dashboard",
  "studio-frames",
  "studio-gallery",
  "studio-printer",
  "studio-camera",
] as const;
export type ShotName = (typeof shotNames)[number];

export function shotFor(name: ShotName, locale: Locale): ImageMetadata {
  const pick = (l: string) => files[`../assets/screenshots/${name}.${l}.png`]?.default;
  const img = pick(locale) ?? pick("en");
  if (!img) throw new Error(`missing screenshot ${name}`);
  return img;
}

/** True when the screenshot shown on a page is in that page's language. */
export function shotIsLocalized(name: ShotName, locale: Locale): boolean {
  return Boolean(files[`../assets/screenshots/${name}.${locale}.png`]);
}
