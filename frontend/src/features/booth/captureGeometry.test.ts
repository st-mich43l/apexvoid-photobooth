import { describe, expect, it } from "vitest";
import { coverCrop, fovMismatch, FULL_FOV, guideStyles, previewFovFor, resolveCaptureGuide, windowCrop, windowSize, type Slot } from "./captureGeometry";
import type { FrameChoice } from "../../shared/types";

// the project has no @types/node; the tests run in node, so read the compositor's golden file from disk
// @ts-ignore
const { readFileSync } = (await import("node:fs")) as { readFileSync: (p: string, e: string) => string };
const cwd = (globalThis as unknown as { process: { cwd(): string } }).process.cwd();
interface GoRect { Min: { X: number; Y: number }; Max: { X: number; Y: number } }
interface GoCase { id: string; dpi: number; borderIn: number; slots: Array<Slot & { x: number; y: number }>; sources: Array<{ w: number; h: number; crops: Array<{ photo: number; window: GoRect; crop: GoRect }> }> }
const golden: GoCase[] = JSON.parse(readFileSync(`${cwd}/../internal/templates/testdata/slotcrops.json`, "utf8"));

const frame = (slots: Slot[], extra: Partial<FrameChoice> = {}): FrameChoice => ({ id: "f", name: "F", variantId: "", caption: "", photos: Math.max(...slots.map((s) => s.photo)) + 1, format: "2×6 strip", photoSlots: slots, dpi: 300, borderIn: 0, ...extra });
const slot = (photo: number, w: number, h: number, focusX?: number, focusY?: number): Slot => ({ photo, w, h, focusX, focusY });

describe("the crop matches the Go compositor exactly", () => {
  it("has golden data for every kind of frame", () => {
    expect(golden.length).toBeGreaterThan(15);
    expect(golden.some((c) => c.borderIn > 0)).toBe(true);
    expect(golden.some((c) => c.slots.some((s) => s.focusX && s.focusX !== 0.5))).toBe(true);
    expect(new Set(golden.flatMap((c) => c.sources.map((s) => `${s.w}x${s.h}`))).size).toBeGreaterThanOrEqual(5);
  });

  for (const c of golden) {
    it(`${c.id}: window size and integer crop equal the compositor's for every camera size`, () => {
      for (const src of c.sources) {
        const usable = c.slots.filter((s) => { const { w, h } = windowSize(s, c.dpi, c.borderIn); return w > 0 && h > 0; });
        expect(usable.length).toBe(src.crops.length);
        usable.forEach((s, i) => {
          const want = src.crops[i];
          const { w, h } = windowSize(s, c.dpi, c.borderIn);
          expect({ w, h }).toEqual({ w: want.window.Max.X - want.window.Min.X, h: want.window.Max.Y - want.window.Min.Y });
          const { fx, fy } = { fx: s.focusX || 0.5, fy: s.focusY || 0.5 };
          expect(coverCrop(src.w, src.h, w, h, fx, fy), `${c.id} slot ${i} on ${src.w}x${src.h}`).toEqual({ x0: want.crop.Min.X, y0: want.crop.Min.Y, x1: want.crop.Max.X, y1: want.crop.Max.Y });
        });
      }
    });

    it(`${c.id}: the on-screen fractions agree with the compositor to within a pixel`, () => {
      for (const src of c.sources) {
        const usable = c.slots.filter((s) => { const { w, h } = windowSize(s, c.dpi, c.borderIn); return w > 0 && h > 0; });
        usable.forEach((s, i) => {
          const { w, h } = windowSize(s, c.dpi, c.borderIn);
          const want = src.crops[i];
          const r = windowCrop(src.w / src.h, w, h, s.focusX || 0.5, s.focusY || 0.5);
          expect(Math.abs(r.x * src.w - want.crop.Min.X)).toBeLessThanOrEqual(1);
          expect(Math.abs(r.y * src.h - want.crop.Min.Y)).toBeLessThanOrEqual(1);
          expect(Math.abs((r.x + r.w) * src.w - want.crop.Max.X)).toBeLessThanOrEqual(1);
          expect(Math.abs((r.y + r.h) * src.h - want.crop.Max.Y)).toBeLessThanOrEqual(1);
        });
      }
    });
  }
});

describe("capture guide", () => {
  const R100 = 3 / 2;
  it("uses the photo WINDOW shape, not the paper shape", () => {
    // 2×6 strip with four 1.7×1.2 in windows: the paper is 1:3, the window is wider than tall
    const g = resolveCaptureGuide(frame([0, 1, 2, 3].map((p) => slot(p, 1.7, 1.2))), { liveAspect: R100 })!;
    expect(g.mode).toBe("single");
    expect(g.aspect).toBeCloseTo(1.7 / 1.2, 2);
  });

  it("is identical for every capture, whatever the capture count: it never depends on a capture index", () => {
    for (const [photos, name] of [[3, "2×6 three-photo"], [4, "2×6 four-photo"], [6, "4×6 six-photo"], [1, "single"]] as const) {
      const slots = Array.from({ length: photos }, (_, i) => slot(i, 1.7, 1.2));
      const f = frame(slots);
      const guides = Array.from({ length: 8 }, () => resolveCaptureGuide(f, { liveAspect: R100 }));
      expect(guides.every((g) => g !== null), name).toBe(true);
      expect(new Set(guides.map((g) => JSON.stringify(g))).size, `${name}: eight captures, one guide`).toBe(1);
    }
  });

  it("recalculates when the frame changes before the first shot", () => {
    const strip = resolveCaptureGuide(frame([slot(0, 1.7, 1.2), slot(1, 1.7, 1.2)]), { liveAspect: R100 })!;
    const portrait = resolveCaptureGuide(frame([slot(0, 1.7, 2.6), slot(1, 1.7, 2.6)]), { liveAspect: R100 })!;
    expect(strip.aspect).toBeGreaterThan(1);
    expect(portrait.aspect).toBeLessThan(1);
  });

  it("applies the frame's border inset (the compositor draws inside it)", () => {
    const plain = resolveCaptureGuide(frame([slot(0, 1.7, 1.2)], { borderIn: 0 }), { liveAspect: R100 })!;
    const bordered = resolveCaptureGuide(frame([slot(0, 1.7, 1.2)], { borderIn: 0.2 }), { liveAspect: R100 })!;
    expect(bordered.aspect).toBeCloseTo(1.3 / 0.8, 2);
    expect(bordered.aspect).not.toBeCloseTo(plain.aspect, 2);
  });

  it("honours a non-central focus point", () => {
    const left = resolveCaptureGuide(frame([slot(0, 1, 1, 0.1, 0.5)]), { liveAspect: R100 })!;
    const right = resolveCaptureGuide(frame([slot(0, 1, 1, 0.9, 0.5)]), { liveAspect: R100 })!;
    expect(left.rect.x).toBeLessThan(right.rect.x);
    expect(left.rect.x).toBeCloseTo((1 - 1 / 1.5) * 0.1, 4);
  });

  it("mirrors the guide for a mirrored live view, because the saved photo is never mirrored", () => {
    const f = frame([slot(0, 1, 1, 0.1, 0.5)]);
    const plain = resolveCaptureGuide(f, { liveAspect: R100 })!, mirrored = resolveCaptureGuide(f, { liveAspect: R100, mirrored: true })!;
    expect(mirrored.rect.x).toBeCloseTo(1 - plain.rect.x - plain.rect.w, 6);
    expect(mirrored.rect.w).toBeCloseTo(plain.rect.w, 6);
    // a centred guide is unchanged by mirroring
    const centred = frame([slot(0, 1, 1)]);
    expect(resolveCaptureGuide(centred, { liveAspect: R100, mirrored: true })!.rect.x).toBeCloseTo(resolveCaptureGuide(centred, { liveAspect: R100 })!.rect.x, 6);
  });

  it("mixed windows: the guide is the area every window shows (conservative common safe area)", () => {
    const g = resolveCaptureGuide(frame([slot(0, 1.7, 1.7), slot(1, 3.4, 1.2), slot(2, 1.0, 2.6)]), { liveAspect: R100 })!;
    expect(g.mode).toBe("common");
    const wide = windowCrop(R100, 3.4 * 300, 1.2 * 300, 0.5, 0.5), tall = windowCrop(R100, 300, 780, 0.5, 0.5), sq = windowCrop(R100, 510, 510, 0.5, 0.5);
    for (const c of [wide, tall, sq]) {
      expect(g.rect.x).toBeGreaterThanOrEqual(c.x - 1e-9); expect(g.rect.y).toBeGreaterThanOrEqual(c.y - 1e-9);
      expect(g.rect.x + g.rect.w).toBeLessThanOrEqual(c.x + c.w + 1e-9); expect(g.rect.y + g.rect.h).toBeLessThanOrEqual(c.y + c.h + 1e-9);
    }
  });

  it("repeated photo references do not change the result (windows count, indices do not)", () => {
    const twoUp = frame([slot(0, 0.9, 2.7), slot(1, 0.9, 2.7), slot(0, 0.9, 2.7), slot(1, 0.9, 2.7)]);
    const one = frame([slot(0, 0.9, 2.7)]);
    expect(resolveCaptureGuide(twoUp, { liveAspect: R100 })).toEqual(resolveCaptureGuide(one, { liveAspect: R100 }));
  });

  it("windows that share nothing, empty windows and unknown geometry fall back to the whole picture (null), never to a wrong part", () => {
    expect(resolveCaptureGuide(frame([slot(0, 1, 1, 0.01, 0.5), slot(1, 1, 1, 0.99, 0.5)], { photos: 2 }), { liveAspect: 3 })).toBeNull();
    expect(resolveCaptureGuide(frame([slot(0, 0, 0)]), { liveAspect: R100 })).toBeNull();
    expect(resolveCaptureGuide(undefined, { liveAspect: R100 })).toBeNull();
    expect(resolveCaptureGuide({ photoSlots: [] }, { liveAspect: R100 })).toBeNull();
  });

  it("maps through the live view's field of view when it differs from the still frame (capability layer, not CSS)", () => {
    const f = frame([slot(0, 1, 1)]);
    const same = resolveCaptureGuide(f, { liveAspect: R100 })!;
    const narrower = resolveCaptureGuide(f, { liveAspect: R100, fov: { x: 0.1, y: 0.1, w: 0.8, h: 0.8 } })!;
    expect(narrower.rect.w).toBeGreaterThan(same.rect.w); // the live view shows less, so the same area is a larger share of it
    const tooNarrow = resolveCaptureGuide(f, { liveAspect: R100, fov: { x: 0.4, y: 0, w: 0.2, h: 1 } })!;
    expect(tooNarrow.clipped).toBe(true);
    expect(tooNarrow.rect.w).toBeLessThanOrEqual(1);
  });

  it("the live picture's shape is kept: the guide box and the enlarged picture use the same fractions", () => {
    const g = resolveCaptureGuide(frame([slot(0, 1.7, 1.2)]), { liveAspect: R100 })!;
    const st = guideStyles(g);
    expect(parseFloat(String(st.media.width))).toBeCloseTo(100 / g.rect.w, 4);
    expect(parseFloat(String(st.media.height))).toBeCloseTo(100 / g.rect.h, 4);
    expect(st.media.objectFit).toBe("fill");
  });

  it("known cameras default to the same field of view; a visible shape difference is flagged", () => {
    expect(previewFovFor("canon-edsdk", "Canon EOS R100")).toBe(FULL_FOV);
    expect(fovMismatch(3 / 2, 3 / 2)).toBe(false);
    expect(fovMismatch(16 / 9, 3 / 2)).toBe(true);
  });
});
