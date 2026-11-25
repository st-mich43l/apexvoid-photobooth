import { formatTranslation, translate } from "../../../shared/i18n";
import type { AssetInfo, FrameDef, Layer, LayerKind, Slot } from "../../../shared/types";

/** Pure edits of a frame draft. Every function returns a new object (the editor keeps history of them). */
export type Selection = { type: "frame" } | { type: "slot"; index: number } | { type: "layer"; id: string };

const r3 = (v: number) => Math.round(v * 1000) / 1000;

export function nextLayerId(d: FrameDef, prefix: string): string {
  const ids = new Set((d.layers ?? []).map((l) => l.id));
  for (let i = 1; ; i++) if (!ids.has(`${prefix}-${i}`)) return `${prefix}-${i}`;
}

export function photoCount(d: FrameDef): number { return d.slots.reduce((n, s) => Math.max(n, s.photo + 1), 0); }

export const updateSlot = (d: FrameDef, i: number, p: Partial<Slot>): FrameDef => ({ ...d, slots: d.slots.map((s, k) => (k === i ? { ...s, ...p } : s)) });
export const updateLayer = (d: FrameDef, id: string, p: Partial<Layer>): FrameDef => ({ ...d, layers: (d.layers ?? []).map((l) => (l.id === id ? { ...l, ...p } : l)) });

export function addSlot(d: FrameDef): FrameDef {
  const last = d.slots[d.slots.length - 1];
  const w = last ? last.w : Math.min(1.5, d.widthIn - 0.3), h = last ? last.h : Math.min(1.2, d.heightIn - 0.3);
  const s: Slot = { x: r3(Math.min(0.15, d.widthIn - w)), y: r3(Math.min(0.15, d.heightIn - h)), w: r3(w), h: r3(h), photo: photoCount(d) };
  return { ...d, slots: [...d.slots, s] };
}

/** Removes a slot and renumbers photos so numbers stay 1..n without gaps (the renderer requires that). */
export function removeSlot(d: FrameDef, index: number): FrameDef {
  if (d.slots.length <= 1) return d;
  const gone = d.slots[index];
  const slots = d.slots.filter((_, k) => k !== index);
  const stillUsed = slots.some((s) => s.photo === gone.photo);
  return { ...d, slots: stillUsed ? slots : slots.map((s) => (s.photo > gone.photo ? { ...s, photo: s.photo - 1 } : s)) };
}

export function duplicateSlot(d: FrameDef, index: number): FrameDef {
  const s = d.slots[index];
  return { ...d, slots: [...d.slots, { ...s, x: r3(Math.min(s.x + 0.1, d.widthIn - s.w)), y: r3(Math.min(s.y + 0.1, d.heightIn - s.h)), photo: photoCount(d), locked: false }] };
}

const DEFAULTS: Record<LayerKind, (d: FrameDef) => Partial<Layer>> = {
  text: (d) => ({ name: "Text", x: 0.1, y: r3(d.heightIn - 0.6), w: r3(d.widthIn - 0.2), h: 0.4, text: "{caption|date}", font: "bold", sizeIn: 0.16, align: "center", color: "text" }),
  rect: () => ({ name: "Rectangle", x: 0.3, y: 0.3, w: 0.8, h: 0.5, color: "accent" }),
  ellipse: () => ({ name: "Ellipse", x: 0.3, y: 0.3, w: 0.6, h: 0.6, color: "accent" }),
  shape: () => ({ name: "Shape", x: 0.3, y: 0.3, w: 0.4, h: 0.4, shape: "heart", color: "accent" }),
  pattern: (d) => ({ name: "Pattern", x: 0.04, y: 0.04, w: r3(d.widthIn - 0.08), h: r3(d.heightIn - 0.08), shape: "dot", size: 0.08, gap: 0.2, spread: true, color: "accent", opacity: 0.6, under: true }),
  image: () => ({ name: "Image", x: 0.3, y: 0.3, w: 0.8, h: 0.8, fit: "contain" }),
};

export function addLayer(d: FrameDef, kind: LayerKind, extra: Partial<Layer> = {}): { draft: FrameDef; id: string } {
  const id = nextLayerId(d, kind);
  const l: Layer = { id, kind, x: 0, y: 0, w: 0.5, h: 0.5, ...DEFAULTS[kind](d), ...extra };
  return { draft: { ...d, layers: [...(d.layers ?? []), l] }, id };
}

export const removeLayer = (d: FrameDef, id: string): FrameDef => ({ ...d, layers: (d.layers ?? []).filter((l) => l.id !== id) });

export function duplicateLayer(d: FrameDef, id: string): { draft: FrameDef; id: string } {
  const src = (d.layers ?? []).find((l) => l.id === id);
  if (!src) return { draft: d, id };
  const nid = nextLayerId(d, src.kind);
  const copy: Layer = { ...src, id: nid, name: (src.name ?? src.kind) + " copy", x: r3(src.x + 0.1), y: r3(src.y + 0.1), locked: false };
  const i = (d.layers ?? []).findIndex((l) => l.id === id);
  const layers = [...(d.layers ?? [])];
  layers.splice(i + 1, 0, copy);
  return { draft: { ...d, layers }, id: nid };
}

/** Moves a layer one step up (towards the front) or down (towards the back) in drawing order. */
export function moveLayer(d: FrameDef, id: string, dir: "up" | "down"): FrameDef {
  const layers = [...(d.layers ?? [])];
  const i = layers.findIndex((l) => l.id === id);
  const j = dir === "up" ? i + 1 : i - 1;
  if (i < 0 || j < 0 || j >= layers.length) return d;
  [layers[i], layers[j]] = [layers[j], layers[i]];
  return { ...d, layers };
}

export function addVariant(d: FrameDef): FrameDef {
  const vs = d.variants ?? [];
  const base = vs.length === 0 ? [{ id: "default", name: "Default" }] : vs;
  let n = base.length + 1;
  while (base.some((v) => v.id === `color-${n}`)) n++;
  return { ...d, variants: [...base, { id: `color-${n}`, name: `Color ${n}`, background: d.background, accent: d.accent, textColor: d.textColor, borderColor: d.borderColor }] };
}
export const removeVariant = (d: FrameDef, id: string): FrameDef => {
  const vs = (d.variants ?? []).filter((v) => v.id !== id);
  return { ...d, variants: vs.length > 1 ? vs : [] };
};

/** Hints about an imported image relative to where it will be used, so print problems show up before saving. */
export function imageWarnings(info: AssetInfo, d: FrameDef, use: "overlay" | "background" | "image", language?: string): string[] {
  const out: string[] = [];
  const cw = Math.round(d.widthIn * d.dpi), ch = Math.round(d.heightIn * d.dpi);
  if (use !== "image") {
    const a = info.width / info.height, b = cw / ch;
    if (Math.abs(a - b) / b > 0.02) out.push(formatTranslation(language, "imageProportions", { width: cw, height: ch }));
    if (info.width < cw * 0.9 || info.height < ch * 0.9) out.push(formatTranslation(language, "imageSoft", { width: cw, height: ch, dpi: d.dpi }));
  }
  if (use === "overlay" && !info.hasAlpha) out.push(translate(language, "imageNoAlpha"));
  return out;
}
