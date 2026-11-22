/** Pure geometry for the Frame Builder. Everything is in inches (print space); the screen only scales it. */
export interface Box { x: number; y: number; w: number; h: number }
export type Handle = "move" | "n" | "s" | "e" | "w" | "ne" | "nw" | "se" | "sw";
export const HANDLES: Handle[] = ["nw", "n", "ne", "e", "se", "s", "sw", "w"];

export const MIN_SIZE = 0.05;
const r3 = (v: number) => Math.round(v * 1000) / 1000;

/** Exact conversion between screen pixels and inches for a canvas shown `shownPx` wide. */
export const pxToIn = (px: number, shownPx: number, widthIn: number) => (px * widthIn) / shownPx;
export const inToPx = (inch: number, shownPx: number, widthIn: number) => (inch * shownPx) / widthIn;

/** Percent position of a box on the canvas (used for overlays so zoom never changes the maths). */
export const pct = (b: Box, W: number, H: number) => ({ left: `${(b.x / W) * 100}%`, top: `${(b.y / H) * 100}%`, width: `${(b.w / W) * 100}%`, height: `${(b.h / H) * 100}%` });

export interface DragOptions {
  W: number; H: number;           // canvas size in inches
  snap: boolean;                  // snap to the grid and to guides
  grid: number;                   // grid step in inches
  threshold: number;              // snap distance in inches
  guidesX: number[]; guidesY: number[]; // other elements' edges/centres and the canvas edges/centre
  clamp: boolean;                 // keep the box inside the canvas (photo slots); layers may bleed
  keepAspect?: boolean;
}
export interface DragResult { box: Box; guideX?: number; guideY?: number }

const nearest = (v: number, cands: number[], th: number): number | undefined => {
  let best: number | undefined, d = th;
  for (const c of cands) { const e = Math.abs(c - v); if (e <= d) { d = e; best = c; } }
  return best;
};

/** Collect snap guides: the canvas edges and centre plus every other box's edges and centre. */
export function guidesFor(W: number, H: number, others: Box[]): { x: number[]; y: number[] } {
  const x = [0, W / 2, W], y = [0, H / 2, H];
  for (const o of others) { x.push(o.x, o.x + o.w / 2, o.x + o.w); y.push(o.y, o.y + o.h / 2, o.y + o.h); }
  return { x, y };
}

/** Moves or resizes a box by (dx, dy) inches from its starting geometry. */
export function applyDrag(start: Box, handle: Handle, dx: number, dy: number, o: DragOptions): DragResult {
  let { x, y, w, h } = start;
  let gx: number | undefined, gy: number | undefined;
  const snapX = (v: number): number => {
    if (!o.snap) return v;
    const g = nearest(v, o.guidesX, o.threshold);
    if (g !== undefined) { gx = g; return g; }
    return Math.round(v / o.grid) * o.grid;
  };
  const snapY = (v: number): number => {
    if (!o.snap) return v;
    const g = nearest(v, o.guidesY, o.threshold);
    if (g !== undefined) { gy = g; return g; }
    return Math.round(v / o.grid) * o.grid;
  };

  if (handle === "move") {
    let nx = start.x + dx, ny = start.y + dy;
    if (o.snap) { // snap the left / centre / right edge, whichever is closest to a guide
      const opts = [nx, nx + w / 2, nx + w].map((v, i) => ({ v, i, g: nearest(v, o.guidesX, o.threshold) })).filter((c) => c.g !== undefined);
      if (opts.length) { const c = opts.reduce((a, b) => (Math.abs(a.g! - a.v) <= Math.abs(b.g! - b.v) ? a : b)); nx += c.g! - c.v; gx = c.g; }
      else nx = Math.round(nx / o.grid) * o.grid;
      const optsY = [ny, ny + h / 2, ny + h].map((v) => ({ v, g: nearest(v, o.guidesY, o.threshold) })).filter((c) => c.g !== undefined);
      if (optsY.length) { const c = optsY.reduce((a, b) => (Math.abs(a.g! - a.v) <= Math.abs(b.g! - b.v) ? a : b)); ny += c.g! - c.v; gy = c.g; }
      else ny = Math.round(ny / o.grid) * o.grid;
    }
    x = nx; y = ny;
  } else {
    let left = start.x, right = start.x + start.w, top = start.y, bottom = start.y + start.h;
    if (handle.includes("w")) left = snapX(start.x + dx);
    if (handle.includes("e")) right = snapX(start.x + start.w + dx);
    if (handle.includes("n")) top = snapY(start.y + dy);
    if (handle.includes("s")) bottom = snapY(start.y + start.h + dy);
    if (right - left < MIN_SIZE) { if (handle.includes("w")) left = right - MIN_SIZE; else right = left + MIN_SIZE; }
    if (bottom - top < MIN_SIZE) { if (handle.includes("n")) top = bottom - MIN_SIZE; else bottom = top + MIN_SIZE; }
    if (o.keepAspect && start.h > 0) {
      const ar = start.w / start.h;
      const nw = right - left, nh = bottom - top;
      if (handle.length === 2 || handle === "e" || handle === "w") {
        const h2 = nw / ar; // width leads
        if (handle.includes("n")) top = bottom - h2; else bottom = top + h2;
      } else {
        const w2 = nh * ar; const cx = (left + right) / 2; left = cx - w2 / 2; right = cx + w2 / 2;
      }
    }
    if (o.clamp) { left = Math.max(left, 0); top = Math.max(top, 0); right = Math.min(right, o.W); bottom = Math.min(bottom, o.H); }
    x = left; y = top; w = right - left; h = bottom - top;
  }
  let box = { x, y, w, h };
  if (o.clamp && handle === "move") box = clampBox(box, o.W, o.H);
  return { box: { x: r3(box.x), y: r3(box.y), w: r3(box.w), h: r3(box.h) }, guideX: gx, guideY: gy };
}

/** Keeps a box inside the canvas, shrinking it only if it is larger than the canvas. */
export function clampBox(b: Box, W: number, H: number): Box {
  const w = Math.min(b.w, W), h = Math.min(b.h, H);
  return { x: Math.min(Math.max(b.x, 0), W - w), y: Math.min(Math.max(b.y, 0), H - h), w, h };
}

/** Evenly spaced vertical stack of n boxes (the quick "re-layout" helper). */
export function stack(n: number, W: number, top: number, bottom: number, margin: number, gap: number): Box[] {
  const h = (bottom - top - gap * (n - 1)) / n;
  return Array.from({ length: n }, (_, i) => ({ x: margin, y: r3(top + i * (h + gap)), w: r3(W - 2 * margin), h: r3(h) }));
}
