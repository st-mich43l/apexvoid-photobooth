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
