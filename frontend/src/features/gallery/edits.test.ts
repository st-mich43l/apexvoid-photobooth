import { describe, expect, it } from "vitest";
import { applyColor, cropRect } from "./edits";

describe("editor colour maths matches the Go renderer", () => {
  it("is the identity for a zero recipe", () => {
    const px = new Uint8ClampedArray([10, 120, 250, 255]);
    applyColor(px, { brightness: 0, contrast: 0, saturation: 0 });
    expect([...px]).toEqual([10, 120, 250, 255]);
  });
  it("saturation -1 yields grey", () => {
    const px = new Uint8ClampedArray([200, 50, 10, 255]);
    applyColor(px, { brightness: 0, contrast: 0, saturation: -1 });
    expect(px[0]).toBe(px[1]);
    expect(px[1]).toBe(px[2]);
  });
  it("brightness adds b*255 and clamps", () => {
    const px = new Uint8ClampedArray([100, 100, 100, 255, 250, 250, 250, 255]);
    applyColor(px, { brightness: 0.2, contrast: 0, saturation: 0 });
    expect(px[0]).toBe(151);
    expect(px[4]).toBe(255);
  });
});

describe("crop rectangle", () => {
  it("returns null for no crop", () => expect(cropRect({ aspect: null, zoom: 1, cx: 0.5, cy: 0.5 }, 400, 300)).toBeNull());
  it("fits a 1:1 crop inside a landscape image and stays in bounds", () => {
    const r = cropRect({ aspect: 1, zoom: 1, cx: 0.9, cy: 0.5 }, 400, 200)!;
    expect(r.w).toBeCloseTo(0.5);
    expect(r.h).toBeCloseTo(1);
    expect(r.x + r.w).toBeLessThanOrEqual(1.0000001);
  });
});
