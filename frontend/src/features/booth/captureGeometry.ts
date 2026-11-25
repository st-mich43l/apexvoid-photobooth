import type { CSSProperties } from "react";
import type { FrameChoice } from "../../shared/types";

/**
 * Capture geometry: what part of the camera picture ends up in the frame's photo windows, so the guide the guest
 * looks at while posing is the picture they will get.
 *
 * Source of truth is the Go compositor (internal/templates/render.go): a window is the slot rounded to pixels and
 * inset by the frame's photo border; the photograph is scaled to COVER the window and the excess is cut around the
 * focus point (0 = centre). `coverCrop` repeats that integer arithmetic exactly and is tested against crops dumped
 * by the compositor (internal/templates/testdata/slotcrops.json). `windowCrop` is the same crop as continuous
 * fractions of the picture, which is what the screen needs.
 *
 * Candidate photographs are interchangeable (any capture can end up in any window), so nothing here depends on the
 * capture index. A frame with several differently shaped windows gets the area common to all of them.
 */

export interface PixelRect { x0: number; y0: number; x1: number; y1: number }
/** A rectangle as fractions (0..1) of a picture. */
export interface Rect { x: number; y: number; w: number; h: number }
export type Slot = NonNullable<FrameChoice["photoSlots"]>[number];

/** JavaScript's Math.round differs from Go's for negative halves only; every value here is ≥ 0, where they agree. */
const px = (inches: number, dpi: number) => Math.round(inches * dpi);

/** The window's size in pixels: the slot rounded to pixels (edges first, as the compositor does), less the border on each side. */
export function windowSize(slot: Pick<Slot, "w" | "h"> & { x?: number; y?: number }, dpi: number, borderIn = 0): { w: number; h: number } {
  const x = slot.x ?? 0, y = slot.y ?? 0;
  const b = borderIn > 0 ? px(borderIn, dpi) : 0;
  return { w: px(x + slot.w, dpi) - px(x, dpi) - 2 * b, h: px(y + slot.h, dpi) - px(y, dpi) - 2 * b };
}

/** A zero focus coordinate means "centre", exactly as in the compositor. */
export const focusOf = (slot: Pick<Slot, "focusX" | "focusY">) => ({ fx: slot.focusX || 0.5, fy: slot.focusY || 0.5 });

/** The compositor's crop, in source pixels, for a w×h window and a sw×sh photograph. */
export function coverCrop(sw: number, sh: number, w: number, h: number, fx: number, fy: number): PixelRect {
  const scale = Math.max(w / sw, h / sh);
  const cw = w / scale, ch = h / scale;
  const x0 = (sw - cw) * fx, y0 = (sh - ch) * fy;
  const r = (v: number) => Math.round(v);
  return { x0: Math.max(0, r(x0)), y0: Math.max(0, r(y0)), x1: Math.min(sw, r(x0 + cw)), y1: Math.min(sh, r(y0 + ch)) };
}

/** The same crop as fractions of the picture, for a picture whose width/height is `aspect`. */
export function windowCrop(aspect: number, w: number, h: number, fx: number, fy: number): Rect {
  const win = w / h;
  if (win > aspect) { const ch = aspect / win; return { x: 0, y: (1 - ch) * fy, w: 1, h: ch }; } // window wider than the picture: full width
  const cw = win / aspect;
  return { x: (1 - cw) * fx, y: 0, w: cw, h: 1 };
}

const same = (a: Rect, b: Rect) => Math.abs(a.x - b.x) < 1e-6 && Math.abs(a.y - b.y) < 1e-6 && Math.abs(a.w - b.w) < 1e-6 && Math.abs(a.h - b.h) < 1e-6;
function intersect(a: Rect, b: Rect): Rect | null {
  const x0 = Math.max(a.x, b.x), y0 = Math.max(a.y, b.y), x1 = Math.min(a.x + a.w, b.x + b.w), y1 = Math.min(a.y + a.h, b.y + b.h);
  return x1 - x0 > 1e-6 && y1 - y0 > 1e-6 ? { x: x0, y: y0, w: x1 - x0, h: y1 - y0 } : null;
}

/** The part of the still frame the live view shows. Default: all of it. Camera-specific knowledge belongs here, not in CSS. */
export type PreviewFov = Rect;
export const FULL_FOV: PreviewFov = { x: 0, y: 0, w: 1, h: 1 };

/**
 * Where the live view's field of view differs from the final photograph, per camera backend. Canon's EVF stream and
 * its full-resolution JPEG are NOT guaranteed to cover the same area; until a body has been measured (see
 * docs/BOOTH_CAPTURE_GEOMETRY.md) the honest default is "the same", and `fovMismatch` flags a visible difference.
 * A measured body adds an entry keyed by `driver:model` here.
 */
const KNOWN_FOV: Record<string, PreviewFov> = {};
export const previewFovFor = (driver?: string, model?: string): PreviewFov => KNOWN_FOV[`${driver}:${model}`] ?? KNOWN_FOV[`${driver}:*`] ?? FULL_FOV;

/** True when the live picture and the still picture have visibly different shapes (a sign of different fields of view). */
export const fovMismatch = (liveAspect: number, stillAspect: number, tolerance = 0.02) => Math.abs(liveAspect / stillAspect - 1) > tolerance;

export interface CaptureGuide {
  /** The visible area, as fractions of the live picture (mirroring and field of view already applied). */
  rect: Rect;
  /** Width/height of the guide on screen. */
  aspect: number;
  /** "single": every window shows the same crop; "common": windows differ, the guide is the area all of them share. */
  mode: "single" | "common";
  /** True when the guide had to be clipped to the live view's field of view. */
  clipped: boolean;
}

export interface GuideOptions {
  /** width/height of the final photograph (default: the live picture's). */
  stillAspect?: number;
  /** width/height of the live picture the guide is drawn on. */
  liveAspect: number;
  /** The live preview is flipped like a mirror on screen; the saved photo never is. */
  mirrored?: boolean;
  fov?: PreviewFov;
}

/**
 * The capture guide for a frame, or null when the frame's geometry is not known (the live picture is then shown whole,
 * identically for every capture). The same guide applies to PREVIEW, COUNTDOWN and CAPTURING and to every candidate:
 * it depends only on the frame and the camera, never on which photo is being taken.
 */
export function resolveCaptureGuide(frame: Pick<FrameChoice, "photoSlots" | "dpi" | "borderIn"> | undefined, o: GuideOptions): CaptureGuide | null {
  const slots = frame?.photoSlots ?? [];
  const dpi = frame?.dpi || 300;
  const still = o.stillAspect ?? o.liveAspect;
  const crops: Rect[] = [];
  for (const s of slots) {
    const { w, h } = windowSize(s, dpi, frame?.borderIn ?? 0);
    if (w <= 0 || h <= 0) continue; // the compositor skips empty windows too
    const { fx, fy } = focusOf(s);
    crops.push(windowCrop(still, w, h, fx, fy));
  }
  if (crops.length === 0) return null;
  let rect: Rect | null = crops[0];
  let mode: CaptureGuide["mode"] = "single";
  for (const c of crops.slice(1)) {
    if (same(c, rect!)) continue;
    mode = "common";
    rect = intersect(rect!, c);
    if (!rect) return null; // windows share nothing: show the whole picture rather than a misleading part
  }
  // still-frame fractions → live-picture fractions
  const fov = o.fov ?? FULL_FOV;
  let live: Rect = { x: (rect!.x - fov.x) / fov.w, y: (rect!.y - fov.y) / fov.h, w: rect!.w / fov.w, h: rect!.h / fov.h };
  const x0 = Math.max(0, live.x), y0 = Math.max(0, live.y), x1 = Math.min(1, live.x + live.w), y1 = Math.min(1, live.y + live.h);
  const clipped = x0 !== live.x || y0 !== live.y || x1 !== live.x + live.w || y1 !== live.y + live.h;
  live = { x: x0, y: y0, w: x1 - x0, h: y1 - y0 };
  if (live.w <= 0 || live.h <= 0) return null;
  if (o.mirrored) live = { ...live, x: 1 - live.x - live.w }; // the picture is shown flipped, so the same area sits on the other side
  return { rect: live, aspect: (live.w * o.liveAspect) / live.h, mode, clipped };
}

/** CSS for the guide: the box is centred and as large as fits; the live picture inside is enlarged and shifted so only `rect` shows. */
export function guideStyles(g: CaptureGuide): { box: CSSProperties; media: CSSProperties } {
  const { x, y, w, h } = g.rect;
  return {
    box: { ["--shot-ar" as string]: g.aspect } as CSSProperties,
    media: { position: "absolute", width: `${100 / w}%`, height: `${100 / h}%`, left: `${(-100 * x) / w}%`, top: `${(-100 * y) / h}%`, objectFit: "fill", maxWidth: "none" },
  };
}
