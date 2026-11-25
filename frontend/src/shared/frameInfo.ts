import { formatTranslation, photoCount, translate, type TranslationKey } from "./i18n";
import type { Frame, FrameChoice, Variant } from "./types";

export const photosLabel = (n: number) => `${n} ${n === 1 ? "photo" : "photos"}`;

const builtinFrameKeys: Record<string, TranslationKey> = {
  'white-minimal': 'builtinFrameWhiteMinimal',
  'black-minimal': 'builtinFrameBlackMinimal',
  'clean-border': 'builtinFrameCleanBorder',
  'soft-gray': 'builtinFrameSoftGray',
  'rose-pink': 'builtinFrameRosePink',
  'lavender': 'builtinFrameLavender',
  'cream-beige': 'builtinFrameCreamBeige',
  'sky-blue': 'builtinFrameSkyBlue',
  'film-strip': 'builtinFrameFilmStrip',
  'vintage-camera': 'builtinFrameVintageCamera',
  'polaroid': 'builtinFramePolaroid',
  'monochrome-film': 'builtinFrameMonochromeFilm',
  'floral': 'builtinFrameFloral',
  'birthday': 'builtinFrameBirthday',
  'couple': 'builtinFrameCouple',
  'seasonal': 'builtinFrameSeasonal',
  'plain': 'builtinFramePlain',
};
const builtinColorKeys: Record<string, TranslationKey> = {
  'Amber': 'builtinColorAmber',
  'Autumn': 'builtinColorAutumn',
  'Baby Blue': 'builtinColorBabyBlue',
  'Black': 'builtinColorBlack',
  'Blossom': 'builtinColorBlossom',
  'Blue': 'builtinColorBlue',
  'Blue Night': 'builtinColorBlueNight',
  'Blue Party': 'builtinColorBlueParty',
  'Blush': 'builtinColorBlush',
  'Charcoal': 'builtinColorCharcoal',
  'Classic': 'builtinColorClassic',
  'Cool Gray': 'builtinColorCoolGray',
  'Coral': 'builtinColorCoral',
  'Cream': 'builtinColorCream',
  'Graphite': 'builtinColorGraphite',
  'Soft Gray': 'builtinColorSoftGray',
  'Ink': 'builtinColorInk',
  'Ivory': 'builtinColorIvory',
  'Kraft': 'builtinColorKraft',
  'Latte': 'builtinColorLatte',
  'Lavender': 'builtinColorLavender',
  'Lilac': 'builtinColorLilac',
  'Lilac Bloom': 'builtinColorLilacBloom',
  'Mint': 'builtinColorMint',
  'Mint Party': 'builtinColorMintParty',
  'Mist': 'builtinColorMist',
  'Navy': 'builtinColorNavy',
  'Night': 'builtinColorNight',
  'Oat': 'builtinColorOat',
  'Ocean': 'builtinColorOcean',
  'Olive': 'builtinColorOlive',
  'Paper': 'builtinColorPaper',
  'Party': 'builtinColorParty',
  'Peach': 'builtinColorPeach',
  'Rose': 'builtinColorRose',
  'Sepia': 'builtinColorSepia',
  'Sky': 'builtinColorSky',
  'Spring': 'builtinColorSpring',
  'Stone': 'builtinColorStone',
  'Summer': 'builtinColorSummer',
  'Sunflower': 'builtinColorSunflower',
  'Violet': 'builtinColorViolet',
  'Warm Gray': 'builtinColorWarmGray',
  'White': 'builtinColorWhite',
  'Wine': 'builtinColorWine',
  'Winter': 'builtinColorWinter',
};
/** Translate only catalog-owned names; custom frame and variant names stay untouched. */
export function frameDisplayName(frame: { id: string; name: string; builtin?: boolean }, language?: string): string {
  const key = language === "vi" && frame.builtin ? builtinFrameKeys[frame.id.split(".")[0]] : undefined;
  return key ? translate(language, key) : frame.name;
}
export function variantDisplayName(frame: { builtin?: boolean }, variant: Variant, language?: string): string {
  const key = language === "vi" && frame.builtin ? builtinColorKeys[variant.name] : undefined;
  return key ? translate(language, key) : variant.name;
}
export function layoutLabel(format: string, language?: string): string {
  if (language !== "vi") return format;
  return format.replace(/ strip$/, ` ${translate(language, "layoutStrip")}`).replace(/ sheet$/, ` ${translate(language, "layoutSheet")}`);
}

/** Colour shown for a variant button. */
export const swatch = (v: Variant, f: Frame): string => v.background || v.accent || f.background;

export interface Filters { category: string; photos: number | 0 }

export function filterFrames(frames: Frame[], f: Filters): Frame[] {
  return frames.filter((x) => (f.category === "All" || x.category === f.category) && (f.photos === 0 || x.photos === f.photos));
}

/** The photo counts present in a list of frames, ascending. */
export const photoCounts = (frames: Frame[]): number[] => [...new Set(frames.map((f) => f.photos))].sort((a, b) => a - b);

/**
 * How a frame relates to photographs that were already taken:
 *  ok       – same number, nothing to decide
 *  fewer    – the frame uses fewer photos than were taken (the first N can be used)
 *  more     – the frame needs more photos than were taken (retake required)
 * With `pool` the photos are candidates the guest is still choosing from, so any frame that needs no more photos than
 * were taken is fine ("ok"): the choice, not the order they were shot in, decides which ones are used.
 */
export type Fit = { kind: "ok" } | { kind: "fewer"; need: number; have: number } | { kind: "more"; need: number; have: number };
export function fitFor(frame: Frame, taken: number | undefined, pool = false): Fit {
  if (!taken || frame.photos === taken) return { kind: "ok" };
  if (pool && frame.photos < taken) return { kind: "ok" }; // candidates still being reviewed: the guest chooses which ones fill the frame
  return frame.photos < taken ? { kind: "fewer", need: frame.photos, have: taken } : { kind: "more", need: frame.photos, have: taken };
}

export function categoryLabel(category: string, language?: string): string {
  const keys: Record<string, TranslationKey> = { Classic: "categoryClassic", "Korean Pastel": "categoryPastel", Retro: "categoryRetro", Special: "categorySpecial", Plain: "categoryPlain", Custom: "categoryCustom" };
  return keys[category] ? translate(language, keys[category]) : category;
}

export function explainFit(f: Fit, name: string, language?: string): string {
  if (language === "vi" && f.kind !== "ok") return formatTranslation(language, f.kind === "fewer" ? "fitFewer" : "fitMore", { name, need: photoCount(language, f.need), have: photoCount(language, f.have), first: photoCount(language, f.need), havePhotos: photoCount(language, f.have) });
  switch (f.kind) {
    case "fewer": return `“${name}” uses ${photosLabel(f.need)}, but you took ${f.have}. You can use the first ${f.need}, or pick a design for ${f.have} photos.`;
    case "more": return `“${name}” needs ${photosLabel(f.need)}, but you took ${f.have}. Pick a design for ${f.have} photos, or start over to take ${f.need}.`;
    default: return "";
  }
}

export const choiceOf = (f: Frame, variantId: string, caption: string): FrameChoice =>
  ({ id: f.id, name: f.name, builtin: f.builtin, variantId, caption: f.captionEnabled ? caption.trim() : "", photos: f.photos, format: f.format,
    photoSlots: f.slots.map(({ photo, w, h, focusX, focusY }) => ({ photo, w, h, focusX, focusY })), dpi: f.dpi, borderIn: f.borderIn });
