import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const h = vi.hoisted(() => ({ recover: vi.fn() }));
vi.mock("./api", () => ({ api: { recoverShooting: h.recover }, toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message : String(e)), { action: "" }) }));
import { RESET_TIMEOUT_MS, sessionGate } from "./sessionGate";

beforeEach(() => { sessionGate._forget(); h.recover.mockReset(); });
afterEach(() => { vi.useRealTimers(); });

describe("sessionGate: one coordinated reset", () => {
  it("runs a single reset however many callers ask (double tap, Finish + unmount + Exit)", async () => {
    let done!: () => void;
    h.recover.mockReturnValue(new Promise<void>((r) => { done = r; }));
    const a = sessionGate.reset(), b = sessionGate.reset(), c = sessionGate.reset();
    expect(h.recover).toHaveBeenCalledTimes(1);
    expect(sessionGate.state.phase).toBe("resetting");
    expect(a).toBe(b); expect(b).toBe(c);
    done();
    expect(await a).toBe(true);
    expect(sessionGate.state).toEqual({ phase: "idle", error: "" });
  });

  it("ready() waits for the running reset, and is immediate when there is none", async () => {
    expect(await sessionGate.ready()).toBe(true);
    let done!: () => void;
    h.recover.mockReturnValue(new Promise<void>((r) => { done = r; }));
    void sessionGate.reset();
    let ready = false;
    const p = sessionGate.ready().then((v) => { ready = v; });
    await Promise.resolve();
    expect(ready).toBe(false);          // the next session must not start on top of the reset
    done(); await p;
    expect(ready).toBe(true);
  });

  it("a failed reset is observable and can be retried", async () => {
    h.recover.mockRejectedValueOnce(new Error("The photos are still being saved."));
    expect(await sessionGate.reset()).toBe(false);
    expect(sessionGate.state).toEqual({ phase: "failed", error: "The photos are still being saved." });
    expect(await sessionGate.ready()).toBe(false);
    h.recover.mockResolvedValueOnce(undefined);
    expect(await sessionGate.reset()).toBe(true);
    expect(sessionGate.state.phase).toBe("idle");
    expect(h.recover).toHaveBeenCalledTimes(2);
  });

  it("a reset that never answers times out instead of leaving a permanent loading screen", async () => {
    vi.useFakeTimers();
    h.recover.mockReturnValue(new Promise(() => {}));
    const p = sessionGate.reset();
    await vi.advanceTimersByTimeAsync(RESET_TIMEOUT_MS + 10);
    expect(await p).toBe(false);
    expect(sessionGate.state.phase).toBe("failed");
    expect(sessionGate.state.error).toMatch(/did not finish resetting/);
    // and it can be retried straight away
    h.recover.mockResolvedValue(undefined);
    expect(await sessionGate.reset()).toBe(true);
  });
});
