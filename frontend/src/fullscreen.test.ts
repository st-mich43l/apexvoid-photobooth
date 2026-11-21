import { describe, expect, it, vi } from "vitest";
import { enterFullscreen, leaveFullscreen, type WindowApi } from "./fullscreen";

const win = (over: Partial<WindowApi> = {}): WindowApi => {
  let fs = false;
  return { Fullscreen: async () => { fs = true; }, UnFullscreen: async () => { fs = false; }, IsFullscreen: async () => fs, ...over };
};

describe("fullscreen transitions report failures instead of hiding them", () => {
  it("succeeds when the window reports the new state", async () => {
    const w = win();
    await enterFullscreen(w, 200);
    expect(await w.IsFullscreen()).toBe(true);
    await leaveFullscreen(w, 200);
    expect(await w.IsFullscreen()).toBe(false);
  });
  it("throws a readable error when the API call fails", async () => {
    const w = win({ UnFullscreen: vi.fn().mockRejectedValue(new Error("window handle is gone")) });
    await expect(leaveFullscreen(w, 100)).rejects.toThrow(/could not leave fullscreen: window handle is gone/);
  });
  it("detects a window that silently stays fullscreen", async () => {
    const w = win({ UnFullscreen: async () => {}, IsFullscreen: async () => true });
    await expect(leaveFullscreen(w, 120)).rejects.toThrow(/still fullscreen/);
  });
  it("detects a window that never entered fullscreen", async () => {
    const w = win({ Fullscreen: async () => {}, IsFullscreen: async () => false });
    await expect(enterFullscreen(w, 120)).rejects.toThrow(/did not switch to fullscreen/);
  });
});
