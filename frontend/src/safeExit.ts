import type { ShootSnapshot, VideoState } from "./types";

export interface ExitDeps {
  /** always the latest state (read through refs, not captured values) */
  getVideo(): VideoState;
  getShoot(): ShootSnapshot;
  stopRecording(): Promise<void>;
  cancelShooting(): Promise<void>;
  leaveFullscreen(): Promise<void>;
  sleep?(ms: number): Promise<void>;
  /** progress text for the dialog */
  onStep?(text: string): void;
}

export class ExitBlockedError extends Error {}
/** The session was closed safely, but the native window could not be restored. */
export class ExitWindowError extends Error {}

const real = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

async function waitUntil(cond: () => boolean, sleep: (ms: number) => Promise<void>, limitMs: number): Promise<boolean> {
  let waited = 0;
  while (!cond()) {
    if (waited >= limitMs) return false;
    await sleep(100); waited += 100;
  }
  return true;
}

/** True when leaving Booth Mode would interrupt something the operator should know about. */
export function exitImpact(video: VideoState, shoot: ShootSnapshot): { recording: boolean; session: boolean; saving: boolean } {
  return {
    recording: video.state === "recording",
    session: ["COUNTDOWN", "CAPTURING", "REVIEW"].includes(shoot.state),
    saving: video.state === "saving" || shoot.state === "SAVING",
  };
}

/**
 * The only way out of Booth Mode. Order matters for data safety:
 *  1. an active recording is stopped and *saved* first (the exit is aborted if saving fails);
 *  2. a shutter in flight is allowed to finish, so its original reaches disk;
 *  3. an unfinished photo session is cancelled — originals already captured stay in the library;
 *  4. only then is the normal window restored.
 */
export async function safeExit(d: ExitDeps): Promise<void> {
  const sleep = d.sleep ?? real;
  const step = d.onStep ?? (() => {});

  if (d.getVideo().state === "saving") {
    step("Waiting for the video to finish saving…");
    if (!(await waitUntil(() => d.getVideo().state !== "saving", sleep, 30_000))) throw new ExitBlockedError("The video is still being saved. Try again in a moment.");
  }
  if (d.getVideo().state === "recording") {
    step("Stopping and saving the recording…");
    await d.stopRecording();
    if (!(await waitUntil(() => !["recording", "saving"].includes(d.getVideo().state), sleep, 30_000))) throw new ExitBlockedError("The recording is still being saved. Try again in a moment.");
    const v = d.getVideo();
    if (v.state === "error") throw new ExitBlockedError(`The recording could not be saved: ${v.error ?? "unknown error"}. Booth Mode was not closed.`);
  }

  if (d.getShoot().state === "CAPTURING") {
    step("Finishing the photo being taken…");
    await waitUntil(() => d.getShoot().state !== "CAPTURING", sleep, 15_000);
  }
  if (d.getShoot().state === "SAVING") {
    step("Waiting for the photos to finish saving…");
    if (!(await waitUntil(() => d.getShoot().state !== "SAVING", sleep, 30_000))) throw new ExitBlockedError("The photos are still being saved. Try again in a moment.");
  }
  if (["COUNTDOWN", "CAPTURING", "REVIEW"].includes(d.getShoot().state)) {
    step("Closing the photo session (photos already taken are kept)…");
    await d.cancelShooting();
  }

  step("Restoring the desktop window…");
  try { await d.leaveFullscreen(); } catch (e) { throw new ExitWindowError(e instanceof Error ? e.message : String(e)); }
}
