import type { Edits } from "../../shared/types";

/** Same maths as internal/media.Apply (saturation, then contrast and brightness)
 *  so the editor preview matches the saved render. Mutates `px` (RGBA). */
export function applyColor(px: Uint8ClampedArray, e: Pick<Edits, "brightness" | "contrast" | "saturation">): void {
  if (e.brightness === 0 && e.contrast === 0 && e.saturation === 0) return;
  const cf = 1 + e.contrast, sf = 1 + e.saturation, add = e.brightness * 255;
  for (let i = 0; i < px.length; i += 4) {
    const r = px[i], g = px[i + 1], b = px[i + 2];
    const y = 0.299 * r + 0.587 * g + 0.114 * b;
    px[i] = (y + (r - y) * sf - 127.5) * cf + 127.5 + add;
    px[i + 1] = (y + (g - y) * sf - 127.5) * cf + 127.5 + add;
    px[i + 2] = (y + (b - y) * sf - 127.5) * cf + 127.5 + add;
  }
}

export interface CropUI { aspect: number | null; zoom: number; cx: number; cy: number }

/** Normalised crop rectangle (in the rotated image) for the crop controls, or null for "no crop". */
export function cropRect(c: CropUI, w: number, h: number): { x: number; y: number; w: number; h: number } | null {
  if (c.aspect === null && c.zoom >= 1) return null;
  let cw = 1, ch = 1;
  if (c.aspect !== null) {
    const imgAspect = w / h;
    if (c.aspect > imgAspect) { cw = 1; ch = imgAspect / c.aspect; } else { ch = 1; cw = c.aspect / imgAspect; }
  }
  cw *= c.zoom; ch *= c.zoom;
  const x = Math.min(Math.max(c.cx - cw / 2, 0), 1 - cw);
  const y = Math.min(Math.max(c.cy - ch / 2, 0), 1 - ch);
  return { x, y, w: cw, h: ch };
}
