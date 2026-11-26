import { describe, expect, it } from "vitest";
import { aspectOf, compositionFormat, defaultComposition, dominantOrientation, groupSessionMedia, photoGridBand, sessionActions } from "./sessionMedia";
import { translate } from "../../shared/i18n";
import type { Media } from "../../shared/types";

const m = (id: string, kind: Media["kind"], width = 100, height = 100) => ({ id, kind, width, height }) as Media;

describe("session media classification", () => {
  it("separates originals, compositions and videos and never treats a composition as a photo", () => {
    const g = groupSessionMedia([m("p1", "photo"), m("s1", "strip"), m("v1", "video"), m("p2", "photo"), m("r1", "print")]);
    expect(g.originals.map((x) => x.id)).toEqual(["p1", "p2"]);
    expect(g.compositions.map((x) => x.id)).toEqual(["s1", "r1"]);
    expect(g.videos.map((x) => x.id)).toEqual(["v1"]);
  });
  it("picks the newest composition unless one was chosen", () => {
    const list = [m("s1", "strip"), m("s2", "strip")];
    expect(defaultComposition(list)?.id).toBe("s2");
    expect(defaultComposition(list, "s1")?.id).toBe("s1");
    expect(defaultComposition(list, "gone")?.id).toBe("s2");
    expect(defaultComposition([])).toBeNull();
  });
});

describe("composition format", () => {
  it("recognises 2×6 strips and 4×6 prints in both orientations", () => {
    expect(compositionFormat(600, 1800)).toEqual({ size: "2×6", kind: "strip", orientation: "portrait" });
    expect(compositionFormat(1200, 1800)).toEqual({ size: "4×6", kind: "sheet", orientation: "portrait" });
    expect(compositionFormat(1800, 1200)).toEqual({ size: "4×6", kind: "sheet", orientation: "landscape" });
    expect(compositionFormat(1800, 600)).toEqual({ size: "2×6", kind: "strip", orientation: "landscape" });
    expect(compositionFormat(1500, 2100)?.size).toBe("5×7");
  });
  it("does not guess a print size for other shapes or unknown dimensions", () => {
    expect(compositionFormat(1000, 1000)).toEqual({ size: "", kind: null, orientation: "square" });
    expect(compositionFormat(0, 1800)).toBeNull();
    expect(aspectOf(0, 0)).toBeNull();
    expect(aspectOf(1600, 1067)).toBeCloseTo(1.4995, 3);
  });
});

describe("photo grid", () => {
  it("bands the photo count and detects portrait-dominant sessions", () => {
    expect([1, 2, 3, 4, 5, 8, 20].map(photoGridBand)).toEqual(["one", "two", "three", "four", "many", "many", "many"]);
    expect(dominantOrientation([m("a", "photo", 1067, 1600), m("b", "photo", 1067, 1600), m("c", "photo", 1600, 1067)])).toBe("portrait");
    expect(dominantOrientation([m("a", "photo", 1600, 1067)])).toBe("landscape");
  });
});

describe("session actions", () => {
  it("keeps the existing availability rules", () => {
    expect(sessionActions("completed")).toEqual({ canExport: true, canRestore: false, canTrash: true });
    expect(sessionActions("active")).toEqual({ canExport: false, canRestore: false, canTrash: false });
    expect(sessionActions("trashed")).toEqual({ canExport: false, canRestore: true, canTrash: false });
    expect(sessionActions("interrupted")).toEqual({ canExport: true, canRestore: false, canTrash: true });
  });
});

describe("session detail terminology", () => {
  it("uses the agreed English and Vietnamese labels", () => {
    const keys = ["sdSessionDetails", "sdOriginalPhotos", "sdFinalComposition", "sdPrintHistory", "sdExportSession", "sdSessionId", "sdStatusCompleted"] as const;
    expect(keys.map((k) => translate("en", k))).toEqual(["Session details", "Original photos", "Final composition", "Print history", "Export session", "Session ID", "Completed"]);
    expect(keys.map((k) => translate("vi", k))).toEqual(["Chi tiết phiên", "Ảnh gốc", "Ảnh ghép", "Lịch sử in", "Xuất phiên", "Mã phiên", "Hoàn tất"]);
  });
});
