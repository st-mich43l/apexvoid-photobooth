import { Window } from "@wailsio/runtime";

/** The subset of the Wails window API used here (injectable for tests). */
export interface WindowApi {
  Fullscreen(): Promise<void>;
  UnFullscreen(): Promise<void>;
  IsFullscreen(): Promise<boolean>;
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/** Polls until the window reports the wanted fullscreen state (the OS applies it asynchronously). */
async function settle(win: WindowApi, want: boolean, timeoutMs: number): Promise<boolean> {
  const end = Date.now() + timeoutMs;
  for (;;) {
    if ((await win.IsFullscreen()) === want) return true;
    if (Date.now() >= end) return false;
    await sleep(50);
  }
}

/** Enters fullscreen and verifies it; throws a readable error instead of failing silently. */
export async function enterFullscreen(win: WindowApi = Window, timeoutMs = 1500): Promise<void> {
  try { await win.Fullscreen(); } catch (e) { throw new Error(`The window could not enter fullscreen: ${msg(e)}`); }
  if (!(await settle(win, true, timeoutMs))) throw new Error("The window did not switch to fullscreen.");
}

/** Restores the normal desktop window and verifies it. */
export async function leaveFullscreen(win: WindowApi = Window, timeoutMs = 1500): Promise<void> {
  try { await win.UnFullscreen(); } catch (e) { throw new Error(`The window could not leave fullscreen: ${msg(e)}`); }
  if (!(await settle(win, false, timeoutMs))) throw new Error("The window is still fullscreen.");
}

const msg = (e: unknown) => (e instanceof Error ? e.message : String(e));
