import { describe, expect, it } from "vitest";
import { applyDrag, clampBox, guidesFor, inToPx, pct, pxToIn, stack, type DragOptions } from "./geometry";
import { canRedo, canUndo, newHistory, push, redo, undo } from "./history";
import { addLayer, addSlot, addVariant, duplicateLayer, imageWarnings, moveLayer, photoCount, removeLayer, removeSlot, removeVariant, updateSlot } from "./draft";
import type { FrameDef } from "../../../shared/types";

const base: FrameDef = { id: "d", name: "D", kind: "custom", category: "Custom", order: 0, revision: 0, enabled: false, widthIn: 2, heightIn: 6, dpi: 300, background: "#fff",
  slots: [{ x: 0.15, y: 0.15, w: 1.7, h: 1.2, photo: 0 }, { x: 0.15, y: 1.45, w: 1.7, h: 1.2, photo: 1 }] };
const opt = (o: Partial<DragOptions> = {}): DragOptions => ({ W: 2, H: 6, snap: false, grid: 0.05, threshold: 0.06, guidesX: [], guidesY: [], clamp: true, ...o });

describe("screen ↔ print coordinates", () => {
  it("round-trips exactly at any zoom", () => {
    for (const shown of [180, 300, 733, 1200]) {
      for (const inch of [0, 0.15, 1.7, 2]) expect(pxToIn(inToPx(inch, shown, 2), shown, 2)).toBeCloseTo(inch, 9);
    }
    expect(inToPx(2, 300, 2)).toBe(300);
  });
  it("positions overlays in percent of the canvas, independent of zoom", () => {
    expect(pct({ x: 0.15, y: 0.15, w: 1.7, h: 1.2 }, 2, 6)).toEqual({ left: "7.5%", top: "2.5%", width: "85%", height: "20%" });
  });
});

describe("dragging and resizing", () => {
  const box = base.slots[0];
  it("moves by exactly the dragged distance when snapping is off", () => {
    expect(applyDrag(box, "move", 0.1, 0.25, opt()).box).toMatchObject({ x: 0.25, y: 0.4, w: 1.7, h: 1.2 });
  });
  it("never lets a photo slot leave the canvas", () => {
    const r = applyDrag(box, "move", 9, 99, opt()).box;
    expect(r.x + r.w).toBeLessThanOrEqual(2 + 1e-9); expect(r.y + r.h).toBeLessThanOrEqual(6 + 1e-9);
    const l = applyDrag(box, "move", -9, -9, opt()).box;
    expect(l.x).toBe(0); expect(l.y).toBe(0);
  });
  it("layers may bleed past the edge when clamping is off", () => {
    expect(applyDrag(box, "move", -1, 0, opt({ clamp: false })).box.x).toBeCloseTo(-0.85, 6);
  });
  it("resizes from each handle without moving the opposite edge", () => {
    const se = applyDrag(box, "se", 0.1, 0.2, opt()).box;
    expect(se).toMatchObject({ x: 0.15, y: 0.15 }); expect(se.w).toBeCloseTo(1.8, 6); expect(se.h).toBeCloseTo(1.4, 6);
    const over = applyDrag(box, "se", 0.5, 0, opt()).box;          // dragged past the canvas edge: the edge stops, the box does not slide
    expect(over.x).toBe(0.15); expect(over.x + over.w).toBeCloseTo(2, 6);
    const nw = applyDrag(box, "nw", 0.05, 0.05, opt()).box;
    expect(nw.x + nw.w).toBeCloseTo(1.85, 6); expect(nw.y + nw.h).toBeCloseTo(1.35, 6);
    const e = applyDrag(box, "e", -0.5, 0, opt()).box;
    expect(e.x).toBe(0.15); expect(e.w).toBeCloseTo(1.2, 6);
  });
  it("keeps a minimum size and flips cleanly past the opposite edge", () => {
    const r = applyDrag(box, "e", -5, 0, opt()).box;
    expect(r.w).toBeCloseTo(0.05, 6); expect(r.x).toBeCloseTo(0.15, 6);
  });
  it("snaps to the grid and to other elements' edges and the canvas centre", () => {
    const g = guidesFor(2, 6, [{ x: 0.15, y: 1.45, w: 1.7, h: 1.2 }]);
    const o = opt({ snap: true, guidesX: g.x, guidesY: g.y, grid: 0.25 });
    const r = applyDrag({ x: 0.5, y: 3, w: 1, h: 1 }, "move", 0.02, 0.01, o);
    expect(r.box.x).toBeCloseTo(0.5, 6);                         // centre 1.02 snaps to the canvas centre 1.0 → x = 0.5
    const down = applyDrag({ x: 0.15, y: 3, w: 1.7, h: 1 }, "move", 0, -0.37, o);   // top 2.63 ≈ bottom of the box above 2.65
    expect(down.guideY).toBeCloseTo(2.65, 6); expect(down.box.y).toBeCloseTo(2.65, 6);
    const free = applyDrag({ x: 0.5, y: 3, w: 1, h: 1 }, "move", 0.02, 0.01, opt({ snap: false }));
    expect(free.box.x).toBeCloseTo(0.52, 6);
  });
  it("optional aspect ratio is kept on corner resize", () => {
    const r = applyDrag({ x: 0.2, y: 0.2, w: 1, h: 0.5 }, "se", 0.4, 0.0, opt({ keepAspect: true })).box;
    expect(r.w / r.h).toBeCloseTo(2, 2);
  });
  it("clampBox and stack helpers", () => {
    expect(clampBox({ x: -1, y: 5.5, w: 1, h: 1 }, 2, 6)).toEqual({ x: 0, y: 5, w: 1, h: 1 });
    const s = stack(4, 2, 0.15, 5.25, 0.15, 0.1);
    expect(s).toHaveLength(4); expect(s[3].y + s[3].h).toBeCloseTo(5.25, 2);
  });
});

describe("undo / redo", () => {
  it("steps back and forward and drops the redo branch on a new edit", () => {
    let h = newHistory(1);
    h = push(h, 2, undefined, 0); h = push(h, 3, undefined, 10_000);
    expect(canUndo(h)).toBe(true); h = undo(h); expect(h.present).toBe(2); expect(canRedo(h)).toBe(true);
    h = redo(h); expect(h.present).toBe(3);
    h = undo(undo(h)); expect(h.present).toBe(1);
    h = push(h, 9, undefined, 20_000); expect(canRedo(h)).toBe(false); expect(undo(h).present).toBe(1);
  });
  it("merges quick edits with the same key into one step", () => {
    let h = newHistory("a");
    h = push(h, "ab", "typing", 1000); h = push(h, "abc", "typing", 1200); h = push(h, "abcd", "typing", 1300);
    expect(undo(h).present).toBe("a");
    h = push(h, "x", "typing", 5000);                              // too late: a separate step
    expect(undo(h).present).toBe("abcd");
  });
  it("ignores no-op edits and bounds memory", () => {
    const h = newHistory({ a: 1 });
    expect(push(h, h.present)).toBe(h);
    let g = newHistory(0);
    for (let i = 1; i < 300; i++) g = push(g, i, undefined, i * 10_000);
    expect(g.past.length).toBeLessThanOrEqual(100);
  });
});

describe("draft edits", () => {
  it("adds, duplicates, reorders and removes layers", () => {
    let { draft, id } = addLayer(base, "text");
    expect(draft.layers![0]).toMatchObject({ id: "text-1", kind: "text", text: "{caption|date}" });
    const r = addLayer(draft, "rect"); draft = r.draft;
    draft = moveLayer(draft, "text-1", "up");
    expect(draft.layers!.map((l) => l.id)).toEqual(["rect-1", "text-1"]);
    expect(moveLayer(draft, "text-1", "up")).toBe(draft);                       // already in front
    const dup = duplicateLayer(draft, "rect-1"); expect(dup.draft.layers!.map((l) => l.id)).toEqual(["rect-1", "rect-2", "text-1"]);
    draft = removeLayer(dup.draft, "rect-1"); expect(draft.layers!.map((l) => l.id)).toEqual(["rect-2", "text-1"]);
    expect(id).toBe("text-1");
  });
  it("keeps photo numbers 1..n without gaps when slots are added and removed", () => {
    let d = addSlot(base); expect(d.slots[2].photo).toBe(2); expect(photoCount(d)).toBe(3);
    d = removeSlot(d, 0); expect(d.slots.map((s) => s.photo)).toEqual([0, 1]);   // renumbered
    expect(removeSlot(removeSlot(d, 0), 0).slots).toHaveLength(1);               // the last slot cannot be removed
  });
  it("repeating a photo (two-up) is not treated as a gap", () => {
    const two: FrameDef = { ...base, slots: [{ ...base.slots[0] }, { ...base.slots[1] }, { ...base.slots[0], x: 1 }] };
    two.slots[2].photo = 0;
    expect(removeSlot(two, 2).slots.map((s) => s.photo)).toEqual([0, 1]);
    expect(removeSlot(two, 0).slots.map((s) => s.photo)).toEqual([1, 0]);       // photo 0 is still used by the repeat: no renumbering
  });
  it("edits are immutable", () => {
    const d = updateSlot(base, 0, { x: 1 }); expect(base.slots[0].x).toBe(0.15); expect(d.slots[0].x).toBe(1);
  });
  it("colour options: the first removal keeps at least two or none", () => {
    let d = addVariant(base); expect(d.variants!.map((v) => v.id)).toEqual(["default", "color-2"]);
    d = addVariant(d); expect(d.variants).toHaveLength(3);
    d = removeVariant(d, "color-3"); expect(d.variants).toHaveLength(2);
    d = removeVariant(d, "color-2"); expect(d.variants).toEqual([]);
  });
  it("warns about overlays that would cover or distort the photos", () => {
    const w = imageWarnings({ name: "a.png", format: "png", width: 300, height: 300, hasAlpha: false, bytes: 1 }, base, "overlay");
    expect(w.join(" ")).toMatch(/stretched/); expect(w.join(" ")).toMatch(/no transparency/); expect(w.join(" ")).toMatch(/soft/);
    expect(imageWarnings({ name: "a.png", format: "png", width: 600, height: 1800, hasAlpha: true, bytes: 1 }, base, "overlay")).toEqual([]);
    expect(imageWarnings({ name: "a.png", format: "png", width: 20, height: 20, hasAlpha: false, bytes: 1 }, base, "image")).toEqual([]);
  });
});
