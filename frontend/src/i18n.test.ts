import { describe, expect, it } from "vitest";
import { translate } from "./i18n";
describe("language resources", () => {
  it("uses Vietnamese accents and falls back to English for an unknown language", () => {
    expect(translate("vi", "welcomeTitle")).toBe("Cười lên nào!");
    expect(translate("vi", "takePhotos")).toBe("Chụp ảnh");
    expect(translate("unknown", "takePhotos")).toBe("Take photos");
  });

  it("localizes frame management and its safety messages", () => {
    expect(translate("vi", "framesIntro")).toContain("Khung xác định bố cục");
    expect(translate("vi", "frameDeletedToast")).toContain("Ảnh và các phiên trước không bị ảnh hưởng");
    expect(translate("vi", "deleteFrameHelp")).toContain("mỗi phiên vẫn giữ bản sao khung riêng");
    expect(translate("vi", "builderTitle")).toBe("Trình tạo khung");
    expect(translate("vi", "builderRepeatPhoto")).toContain("Lặp số ảnh");
  });
});

it("has nonempty translations with matching named placeholders", async () => {
  const { enAll, vi } = await import("./i18n");
  expect(Object.keys(vi).sort()).toEqual(Object.keys(enAll).sort());
  const placeholders = (value: string) => [...new Set(value.match(/\{[A-Za-z][A-Za-z0-9_]*\}/g) ?? [])].sort();
  for (const key of Object.keys(enAll) as (keyof typeof enAll)[]) {
    expect(enAll[key].trim(), `empty English ${key}`).not.toBe("");
    expect(vi[key].trim(), `empty Vietnamese ${key}`).not.toBe("");
    expect(placeholders(vi[key]), `placeholders in ${key}`).toEqual(placeholders(enAll[key]));
  }
});


it("has no duplicate keys across dictionary domains", async () => {
  const { translationDomains } = await import("./i18n");
  const keys = translationDomains.flatMap(Object.keys);
  expect(keys.filter((key, i) => keys.indexOf(key) !== i)).toEqual([]);
});

it("translates only built-in frame and colour names without changing stored custom names", async () => {
  const { frameDisplayName, variantDisplayName } = await import("./frameInfo");
  const variant = { id: "white", name: "White" };
  expect(frameDisplayName({ id: "plain.strip4", name: "Plain", builtin: true }, "vi")).toBe("Đơn giản");
  expect(frameDisplayName({ id: "custom.1", name: "Plain", builtin: false }, "vi")).toBe("Plain");
  expect(variantDisplayName({ builtin: true }, variant, "vi")).toBe("Trắng");
  expect(variantDisplayName({ builtin: false }, variant, "vi")).toBe("White");
});
