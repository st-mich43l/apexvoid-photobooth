import { describe, expect, it, vi } from "vitest";
import { exitImpact, safeExit, ExitBlockedError, type ExitDeps } from "./safeExit";
import type { ShootSnapshot, VideoState } from "../../shared/types";

const vid = (o: Partial<VideoState> = {}): VideoState => ({ state: "idle", sessionId: "", elapsedMs: 0, maxSeconds: 30, lastMediaId: "", ...o });
const sh = (state: ShootSnapshot["state"]): ShootSnapshot => ({ state, version: 1, sessionId: "S", shots: 4, slot: 0, remaining: 0, taken: [], simulated: false });

function deps(over: Partial<ExitDeps> & { video?: VideoState; shoot?: ShootSnapshot } = {}) {
  const calls: string[] = [];
  let video = over.video ?? vid();
  let shoot = over.shoot ?? sh("PREVIEW");
  const d: ExitDeps & { calls: string[]; set: (v?: VideoState, s?: ShootSnapshot) => void } = {
    calls,
    set: (v, s) => { if (v) video = v; if (s) shoot = s; },
    getVideo: () => video, getShoot: () => shoot, sleep: async () => {},
    stopRecording: async () => { calls.push("stopRecording"); video = vid({ state: "idle", lastMediaId: "M1" }); },
    cancelShooting: async () => { calls.push("cancelShooting"); shoot = sh("CANCELLED"); },
    leaveFullscreen: async () => { calls.push("leaveFullscreen"); },
    ...over,
  };
  return d;
}

describe("safeExit ordering protects media", () => {
  it("just restores the window from an idle welcome/preview screen", async () => {
    const d = deps();
    await safeExit(d);
    expect(d.calls).toEqual(["leaveFullscreen"]);
  });

  it("stops AND saves a recording before the window changes", async () => {
    const d = deps({ video: vid({ state: "recording" }) });
    await safeExit(d);
    expect(d.calls).toEqual(["stopRecording", "leaveFullscreen"]);
  });

  it("aborts the exit (window untouched) if the recording could not be saved", async () => {
    const d = deps({ video: vid({ state: "recording" }), stopRecording: async () => { d.set(vid({ state: "error", error: "disk full" })); } });
    await expect(safeExit(d)).rejects.toThrow(/could not be saved: disk full/);
    expect(d.calls).not.toContain("leaveFullscreen");
  });

  it("waits for a video that is already saving", async () => {
    let polls = 0;
    const d = deps({ video: vid({ state: "saving" }) });
    d.getVideo = () => (++polls > 3 ? vid() : vid({ state: "saving" }));
    await safeExit(d);
    expect(d.calls).toEqual(["leaveFullscreen"]);
  });

  it("lets a shutter in flight finish before cancelling the session", async () => {
    let polls = 0;
    const d = deps({ shoot: sh("CAPTURING") });
    d.getShoot = () => (++polls > 4 ? sh("REVIEW") : sh("CAPTURING"));
    await safeExit(d);
    expect(d.calls).toEqual(["cancelShooting", "leaveFullscreen"]);
  });

  it("closes an unfinished session (originals stay in the library) in COUNTDOWN and REVIEW", async () => {
    for (const st of ["COUNTDOWN", "REVIEW"] as const) {
      const d = deps({ shoot: sh(st) });
      await safeExit(d);
      expect(d.calls).toEqual(["cancelShooting", "leaveFullscreen"]);
    }
  });

  it("works after a capture error", async () => {
    const d = deps({ shoot: sh("ERROR") });
    await safeExit(d);
    expect(d.calls).toEqual(["leaveFullscreen"]);
  });

  it("blocks instead of cutting off a save that never finishes", async () => {
    const d = deps({ shoot: sh("SAVING") });
    await expect(safeExit(d)).rejects.toBeInstanceOf(ExitBlockedError);
    expect(d.calls).toEqual([]);
  });

  it("propagates a window error after data was handled", async () => {
    const d = deps({ video: vid({ state: "recording" }), leaveFullscreen: async () => { throw new Error("The window is still fullscreen."); } });
    await expect(safeExit(d)).rejects.toThrow(/still fullscreen/);
    expect(d.calls).toEqual(["stopRecording"]);
  });

  it("reports progress steps", async () => {
    const onStep = vi.fn();
    await safeExit(deps({ video: vid({ state: "recording" }), onStep }));
    expect(onStep.mock.calls.map((c) => c[0])).toEqual(expect.arrayContaining([expect.stringMatching(/Stopping and saving/), expect.stringMatching(/Restoring/)]));
  });
});

describe("exitImpact", () => {
  it("flags what an exit would interrupt", () => {
    expect(exitImpact(vid({ state: "recording" }), sh("PREVIEW"))).toEqual({ recording: true, session: false, saving: false });
    expect(exitImpact(vid(), sh("COUNTDOWN")).session).toBe(true);
    expect(exitImpact(vid(), sh("SAVING")).saving).toBe(true);
    expect(exitImpact(vid(), sh("ERROR"))).toEqual({ recording: false, session: false, saving: false });
  });
});

it("reports stable progress codes while preserving the legacy diagnostic text", async () => {
  const onProgress = vi.fn(), onStep = vi.fn();
  await safeExit(deps({ video: vid({ state: "recording" }), onProgress, onStep }));
  expect(onProgress.mock.calls.map(c => c[0])).toEqual(["stoppingVideo", "restoringWindow"]);
  expect(onStep.mock.calls[0][0]).toContain("Stopping and saving");
});
