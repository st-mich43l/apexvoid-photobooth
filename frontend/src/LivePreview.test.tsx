import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, render } from "@testing-library/react";

const S = vi.hoisted(() => ({ mirror: false }));
vi.mock("./state", () => ({
  useApp: () => ({ camera: { liveView: true, state: "liveview", reconnecting: false, device: { name: "c" } }, previewUrl: "http://x/preview", settings: { previewMirror: S.mirror } }),
}));
import { LivePreview } from "./ui";

afterEach(cleanup);

describe("LivePreview mirror option", () => {
  it("flips only the displayed preview when the operator turns the mirror on", () => {
    S.mirror = true;
    const { container } = render(<LivePreview />);
    expect(container.querySelector(".preview.mirrored img")).toBeTruthy();
  });
  it("is not mirrored by default", () => {
    S.mirror = false;
    const { container } = render(<LivePreview />);
    expect(container.querySelector(".preview")).toBeTruthy();
    expect(container.querySelector(".preview.mirrored")).toBeNull();
  });
});
