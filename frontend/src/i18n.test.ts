import { describe, expect, it } from "vitest";
import { translate } from "./i18n";
describe("language resources", () => {
  it("uses Vietnamese accents and falls back to English for an unknown language", () => {
    expect(translate("vi", "welcomeTitle")).toBe("Cười lên nào!");
    expect(translate("vi", "takePhotos")).toBe("Chụp ảnh");
    expect(translate("unknown", "takePhotos")).toBe("Take photos");
  });
});
