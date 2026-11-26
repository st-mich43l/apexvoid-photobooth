import type { BadgeKind } from "../../shared/ui";
import type { TranslationKey } from "../../shared/i18n";
import type { Media } from "../../shared/types";

/** One page of session media split into the groups the session-detail workspace presents. */
export interface SessionMediaGroups { originals: Media[]; compositions: Media[]; videos: Media[] }

/** Classifies media by kind; "strip" and "print" are both finished compositions, never ordinary photos. */
export function groupSessionMedia(items: Media[]): SessionMediaGroups {
  const groups: SessionMediaGroups = { originals: [], compositions: [], videos: [] };
  for (const m of items) {
    if (m.kind === "photo") groups.originals.push(m);
    else if (m.kind === "video") groups.videos.push(m);
    else groups.compositions.push(m);
  }
  return groups;
}

/** Width ÷ height when both are known; null otherwise (never 0, Infinity or NaN). */
export function aspectOf(width: number, height: number): number | null {
  return width > 0 && height > 0 && Number.isFinite(width / height) ? width / height : null;
}

const printSizes: Array<{ size: string; ratio: number; kind: "strip" | "sheet" }> = [
  { size: "2×6", ratio: 2 / 6, kind: "strip" },
  { size: "4×6", ratio: 4 / 6, kind: "sheet" },
  { size: "5×7", ratio: 5 / 7, kind: "sheet" },
];

export interface CompositionFormat { size: string; kind: "strip" | "sheet" | null; orientation: "portrait" | "landscape" | "square" }

/** Recognises the print size of a composition from its pixel dimensions (either orientation, 2 % tolerance). */
export function compositionFormat(width: number, height: number): CompositionFormat | null {
  const ar = aspectOf(width, height);
  if (ar === null) return null;
  const orientation = ar < 0.99 ? "portrait" : ar > 1.01 ? "landscape" : "square";
  const portraitRatio = Math.min(ar, 1 / ar);
  const match = printSizes.find((p) => Math.abs(portraitRatio - p.ratio) / p.ratio < 0.02);
  return { size: match?.size ?? "", kind: match?.kind ?? null, orientation };
}

/** The newest composition is the one the guest last chose; media pages list compositions oldest first. */
export function defaultComposition(compositions: Media[], preferredId?: string): Media | null {
  return compositions.find((m) => m.id === preferredId) ?? compositions.at(-1) ?? null;
}

/** Column band for the original-photo grid; the CSS turns it into 2–4 columns from the space actually available. */
export function photoGridBand(count: number): "one" | "two" | "three" | "four" | "many" {
  if (count <= 1) return "one";
  if (count === 2) return "two";
  if (count === 3) return "three";
  if (count === 4) return "four";
  return "many";
}

/** Mostly portrait photos need more columns to stay compact; landscape is the default. */
export function dominantOrientation(items: Media[]): "portrait" | "landscape" {
  const portrait = items.filter((m) => m.height > m.width).length;
  return portrait > items.length / 2 ? "portrait" : "landscape";
}

export const sessionStatusBadge: Record<string, { key: TranslationKey; kind: BadgeKind }> = {
  active: { key: "sdStatusActive", kind: "info" },
  completed: { key: "sdStatusCompleted", kind: "ok" },
  cancelled: { key: "sdStatusCancelled", kind: "warn" },
  failed: { key: "sdStatusFailed", kind: "err" },
  interrupted: { key: "sdStatusInterrupted", kind: "warn" },
  trashing: { key: "sdStatusTrashing", kind: "err" },
  trashed: { key: "sdStatusTrashed", kind: "err" },
};

/** The rules the previous header enforced, kept in one place. */
export const sessionActions = (status: string) => ({
  canExport: status !== "active" && status !== "trashing" && status !== "trashed",
  canRestore: status === "trashing" || status === "trashed",
  canTrash: status !== "active" && status !== "trashing" && status !== "trashed",
});
