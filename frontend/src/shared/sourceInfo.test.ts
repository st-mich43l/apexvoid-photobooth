import { describe, expect, it } from "vitest";
import { canRecordVideo, guidance, modeOptions, resolutions } from "./sourceInfo";
import type { DeviceInfo, Mode } from "./types";

const dev = (o: Partial<DeviceInfo> = {}): DeviceInfo => ({ id: "x", name: "Cam", driver: "uvc", simulated: false, kind: "webcam", ...o });
const modes: Mode[] = [
  { width: 640, height: 480, fpsMin: 5, fpsMax: 30, format: "yuyv422" },
  { width: 1920, height: 1080, fpsMin: 5, fpsMax: 5, format: "yuyv422" },
  { width: 1920, height: 1080, fpsMin: 5, fpsMax: 30, format: "mjpeg" },
  { width: 1280, height: 720, fpsMin: 5, fpsMax: 60, format: "mjpeg" },
];

describe("source helpers", () => {
  it("lists distinct resolutions largest first", () => expect(resolutions(modes)).toEqual(["1920×1080", "1280×720", "640×480"]));
  it("offers one option per size, preferring MJPEG and capping fps at 30", () => {
    const o = modeOptions(modes);
    expect(o.map((x) => x.key)).toEqual(["1920x1080@30", "1280x720@30", "640x480@30"]);
    expect(o[0].format).toBe("mjpeg");
  });
  it("only UVC sources can record video; the Canon SDK source and duplicates cannot", () => {
    expect(canRecordVideo(dev())).toBe(true);
    expect(canRecordVideo(dev({ driver: "canon-edsdk", kind: "dslr" }))).toBe(false);
    expect(canRecordVideo(dev({ driver: "mock", kind: "simulated" }))).toBe(false);
    expect(canRecordVideo(dev({ duplicateOf: "a" }))).toBe(false);
  });
  it("gives device-specific guidance", () => {
    expect(guidance(dev({ driver: "canon-edsdk", kind: "dslr" }), "the camera is busy")).toMatch(/EOS Utility/);
    expect(guidance(dev(), "could not be opened. It is probably in use by another program")).toMatch(/Teams|Zoom/);
    expect(guidance(dev(), "FFmpeg is required")).toMatch(/ffmpeg\.exe/);
    expect(guidance(dev({ kind: "dslr-uvc" }), "x")).toMatch(/streaming/);
    expect(guidance(undefined, "x")).toBe("");
  });
});
