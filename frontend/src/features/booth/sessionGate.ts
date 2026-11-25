import { useSyncExternalStore } from "react";
import { api, toAppError } from "../../shared/api";

/**
 * One coordinated way out of a photo session.
 *
 * Finish, Home, an error screen and a stale leftover all need the same thing: the
 * engine back at IDLE with live view released. Doing that from several places
 * (an unmount cleanup, a button handler, a start-up effect) used to run
 * overlapping resets that raced the next session's preview start. The gate runs
 * at most ONE reset at a time and hands every caller the same promise, so a
 * double tap or a Finish followed straight by "Take photos" never overlaps.
 *
 * A failure is observable (phase "failed") and recoverable (call reset() again);
 * a hung reset times out instead of leaving a permanent loading screen.
 */
export interface GateState { phase: "idle" | "resetting" | "failed"; error: string }

export const RESET_TIMEOUT_MS = 25_000;

let state: GateState = { phase: "idle", error: "" };
let inflight: Promise<boolean> | null = null;
const subs = new Set<() => void>();
const publish = (s: GateState) => { state = s; subs.forEach((f) => f()); };

function withTimeout<T>(p: Promise<T>, ms: number): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const t = setTimeout(() => reject(new Error("The booth did not finish resetting in time.")), ms);
    p.then((v) => { clearTimeout(t); resolve(v); }, (e) => { clearTimeout(t); reject(e); });
  });
}

export const sessionGate = {
  get state(): GateState { return state; },
  subscribe(f: () => void): () => void { subs.add(f); return () => { subs.delete(f); }; },
  /** Returns the running reset if there is one; otherwise starts it. Resolves true when the engine is back at IDLE. */
  reset(): Promise<boolean> {
    if (inflight) return inflight;
    publish({ phase: "resetting", error: "" });
    const run = (async () => {
      try {
        await withTimeout(api.recoverShooting(), RESET_TIMEOUT_MS);
        publish({ phase: "idle", error: "" });
        return true;
      } catch (e) {
        publish({ phase: "failed", error: toAppError(e).message });
        return false;
      } finally {
        inflight = null;
      }
    })();
    inflight = run;
    return run;
  },
  /** Resolves when no reset is running; false if the last one failed. */
  ready(): Promise<boolean> { return inflight ?? Promise.resolve(state.phase !== "failed"); },
  /** test helper */
  _forget(): void { inflight = null; publish({ phase: "idle", error: "" }); },
};

export function useGate(): GateState {
  return useSyncExternalStore(sessionGate.subscribe, () => state);
}
