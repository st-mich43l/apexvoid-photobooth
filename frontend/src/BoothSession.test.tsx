import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BoothReadiness, Settings, ShootSnapshot, VideoState } from "./types";

// Session safety of Booth Mode. The real Booth runs against a mocked backend and a mocked ShootFlow, so each
// engine state can be dictated and every unsafe transition (timer, Home, Exit, Finish, a new session) checked.
const h = vi.hoisted(() => ({
  S: {} as Record<string, unknown>,
  api: { verifyKioskPin: vi.fn(), stopRecording: vi.fn(), cancelShooting: vi.fn(), recoverShooting: vi.fn(), prepareMode: vi.fn(), listFrames: vi.fn() },
  enter: vi.fn(), leave: vi.fn(),
}));
const { S, api } = h;
vi.mock("./state", () => ({ useApp: () => h.S }));
vi.mock("./api", () => ({ api: h.api, toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message : String(e)), { action: "" }) }));
vi.mock("./fullscreen", () => ({ enterFullscreen: (...a: unknown[]) => h.enter(...a), leaveFullscreen: (...a: unknown[]) => h.leave(...a) }));
vi.mock("./ShootFlow", () => ({
  ShootFlow: (p: { onDone?: () => void; onChangeFrame?: () => void }) => (
    <div data-testid="photo-flow"><button onClick={p.onDone}>Finish</button><button onClick={p.onChangeFrame}>Change frame</button></div>
  ),
}));

import { Booth, IDLE_DONE_MS, IDLE_FRAMES_MS } from "./Booth";
import { sessionGate } from "./sessionGate";

const frame = (id: string, photos: number) => ({ id, name: id, category: "Classic", photos, format: "2×6 strip", enabled: true, pixelW: 600, pixelH: 1800, variants: [], widthIn: 2, heightIn: 6, dpi: 300, slots: [], background: "#fff", kind: "strip", order: 1, revision: 1 });
const video = (): VideoState => ({ state: "idle", sessionId: "", elapsedMs: 0, maxSeconds: 30, lastMediaId: "" });
const shoot = (state: ShootSnapshot["state"], taken = 0): ShootSnapshot =>
  ({ state, version: 1, sessionId: "S", shots: 4, slot: 0, remaining: 0, taken: Array.from({ length: taken }, (_, i) => ({ slot: i, mediaId: `m${i}`, simulated: false })), simulated: false });
const ready = (o: Partial<BoothReadiness> = {}): BoothReadiness => ({ status: "ready", canStart: true, headline: "Ready for guests", guest: "", items: [], at: "", ...o });
const cam = (o: object = {}) => ({ device: { id: "c", name: "Cam", driver: "uvc", simulated: false }, state: "connected", reconnecting: false, liveView: true, ...o });

let view: ReturnType<typeof render>;
const exited = vi.fn(); // one stable callback: a re-render must not swap it under an exit that is already running
const mount = () => { view = render(<Booth onExited={exited} />); };
const redraw = () => view.rerender(<Booth onExited={exited} />);
const setShoot = (s: ShootSnapshot) => { S.shoot = s; redraw(); };

/** Welcome → Take photos → frame → Continue → photo screen */
async function toPhotoScreen() {
  fireEvent.click(screen.getByRole("button", { name: /Take photos/ }));
  await waitFor(() => expect(screen.getByTestId("frame-selection")).toBeTruthy());
  await waitFor(() => expect((screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
  await waitFor(() => expect(screen.getByTestId("photo-flow")).toBeTruthy());
}
const advance = (ms: number) => act(() => { vi.advanceTimersByTime(ms); });

beforeEach(() => {
  exited.mockReset();
  sessionGate._forget();
  S.video = video(); S.shoot = shoot("PREVIEW"); S.camera = cam(); S.readiness = ready(); S.toast = vi.fn(); S.frameUrl = (id: string) => `/f/${id}`;
  S.settings = { shots: 4, templateId: "a", video: { maxSeconds: 30 }, kioskPinSet: false } as unknown as Settings;
  api.listFrames.mockResolvedValue({ frames: [frame("a", 4), frame("b", 3)], categories: ["Classic"], errors: [], dir: "", default: "a" });
  api.prepareMode.mockResolvedValue({ mode: "photo", ready: true, switched: false, needsFallback: false, preferred: {}, message: "" });
  api.recoverShooting.mockReset().mockResolvedValue(undefined);
  api.stopRecording.mockResolvedValue(undefined); api.verifyKioskPin.mockResolvedValue(undefined);
  h.enter.mockResolvedValue(undefined); h.leave.mockResolvedValue(undefined);
  window.location.hash = "";
});
afterEach(() => { cleanup(); vi.useRealTimers(); });

describe("inactivity never abandons a session", () => {
  it.each(["COUNTDOWN", "CAPTURING", "REVIEW", "SAVING"] as const)("a guest who stops touching the screen during %s keeps the session", async (state) => {
    mount(); await toPhotoScreen();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    setShoot(shoot(state, state === "REVIEW" || state === "SAVING" ? 4 : 1));
    advance(10 * 60_000);
    expect(screen.getByTestId("photo-flow")).toBeTruthy();
    expect(api.recoverShooting).not.toHaveBeenCalled();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("does not reset during a camera reconnection, even before the first photo", async () => {
    mount(); await toPhotoScreen();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    S.camera = cam({ reconnecting: true });
    setShoot(shoot("PREVIEW"));
    advance(10 * 60_000);
    expect(screen.getByTestId("photo-flow")).toBeTruthy();
    expect(api.recoverShooting).not.toHaveBeenCalled();
  });

  it("returns to Welcome from the live preview before any photo exists (nothing at stake), with one reset", async () => {
    mount(); await toPhotoScreen();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    fireEvent.pointerDown(document.querySelector(".booth-root")!); // a touch re-arms the idle timer under fake time
    await act(async () => { vi.advanceTimersByTime(IDLE_FRAMES_MS + 100); });
    await act(async () => { await Promise.resolve(); });
    expect(api.recoverShooting).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId("photo-flow")).toBeNull();
    expect(screen.getByText("Say cheese!")).toBeTruthy();
  });

  it("closes a session that is already saved after a long idle time (originals and strip are on disk)", async () => {
    mount(); await toPhotoScreen();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    setShoot(shoot("COMPLETED", 4));
    advance(IDLE_DONE_MS - 1000);
    expect(screen.getByTestId("photo-flow")).toBeTruthy();
    await act(async () => { vi.advanceTimersByTime(2000); });
    expect(screen.queryByTestId("photo-flow")).toBeNull();
    expect(api.recoverShooting).toHaveBeenCalledTimes(1);
  });

  it("a touch restarts the idle clock", async () => {
    mount(); await toPhotoScreen();
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
    setShoot(shoot("PREVIEW"));
    advance(IDLE_FRAMES_MS - 5000);
    fireEvent.pointerDown(document.querySelector(".booth-root")!);
    advance(IDLE_FRAMES_MS - 5000);
    expect(screen.getByTestId("photo-flow")).toBeTruthy();
  });
});

describe("Home", () => {
  it.each(["CAPTURING", "SAVING"] as const)("is ignored while a photo is being %s", async (state) => {
    mount(); await toPhotoScreen();
    setShoot(shoot(state, 2));
    const home = screen.getByRole("button", { name: /Home/ }) as HTMLButtonElement;
    expect(home.disabled).toBe(true);
    fireEvent.click(home);
    expect(screen.queryByRole("dialog", { name: "Leave this photo session?" })).toBeNull();
    expect(screen.getByTestId("photo-flow")).toBeTruthy();
    expect(api.recoverShooting).not.toHaveBeenCalled();
  });

  it("asks before leaving during the countdown; 'Keep going' is the default and changes nothing", async () => {
    mount(); await toPhotoScreen();
    setShoot(shoot("COUNTDOWN", 0));
    fireEvent.click(screen.getByRole("button", { name: /Home/ }));
    const dlg = await screen.findByRole("dialog", { name: "Leave this photo session?" });
    expect(dlg.textContent).toMatch(/not been taken yet/);
    expect(document.activeElement).toBe(screen.getByRole("button", { name: "Keep going" }));
    fireEvent.click(screen.getByRole("button", { name: "Keep going" }));
    expect(screen.queryByRole("dialog", { name: "Leave this photo session?" })).toBeNull();
    expect(api.recoverShooting).not.toHaveBeenCalled();
    expect(screen.getByTestId("photo-flow")).toBeTruthy();
  });

  it("explains in REVIEW that the photos will not become a strip but the originals are kept", async () => {
    mount(); await toPhotoScreen();
    setShoot(shoot("REVIEW", 3));
    fireEvent.click(screen.getByRole("button", { name: /Home/ }));
    const dlg = await screen.findByRole("dialog", { name: "Leave this photo session?" });
    expect(dlg.textContent).toMatch(/3 photos/);
    expect(dlg.textContent).toMatch(/not.*made into your strip/);
    expect(dlg.textContent).toMatch(/host keeps the originals/);
  });

  it("confirming closes the session through ONE coordinated reset, however often Leave is tapped", async () => {
    let done!: () => void;
    api.recoverShooting.mockReturnValue(new Promise<void>((r) => { done = r; }));
    mount(); await toPhotoScreen();
    setShoot(shoot("REVIEW", 4));
    fireEvent.click(screen.getByRole("button", { name: /Home/ }));
    const leave = await screen.findByRole("button", { name: "Leave" });
    fireEvent.click(leave); fireEvent.click(leave); fireEvent.click(leave);
    expect(api.recoverShooting).toHaveBeenCalledTimes(1);
    expect(screen.getByTestId("photo-flow")).toBeTruthy(); // not gone until the session is really closed
    await act(async () => { done(); });
    await waitFor(() => expect(screen.getByText("Say cheese!")).toBeTruthy());
    expect(api.recoverShooting).toHaveBeenCalledTimes(1);
  });

  it("a reset that fails keeps the guest where they are and says so, with a way to retry", async () => {
    api.recoverShooting.mockRejectedValueOnce(new Error("busy"));
    mount(); await toPhotoScreen();
    setShoot(shoot("COUNTDOWN"));
    fireEvent.click(screen.getByRole("button", { name: /Home/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Leave" }));
    await screen.findByText(/could not close this session yet/);
    expect(screen.getByTestId("photo-flow")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Leave" }));
    await waitFor(() => expect(screen.getByText("Say cheese!")).toBeTruthy());
  });

  it("leaves straight away from the live preview (no photos at stake) — still through the reset", async () => {
    mount(); await toPhotoScreen();
    setShoot(shoot("PREVIEW"));
    fireEvent.click(screen.getByRole("button", { name: /Home/ }));
    await waitFor(() => expect(screen.getByText("Say cheese!")).toBeTruthy());
    expect(api.recoverShooting).toHaveBeenCalledTimes(1);
  });

  it("Escape in the leave dialog closes only that dialog, not Booth Mode's exit dialog", async () => {
    mount(); await toPhotoScreen();
    setShoot(shoot("COUNTDOWN"));
    fireEvent.click(screen.getByRole("button", { name: /Home/ }));
    await screen.findByRole("dialog", { name: "Leave this photo session?" });
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Leave this photo session?" })).toBeNull());
    expect(screen.queryByRole("dialog", { name: "Exit Booth Mode and return to Studio?" })).toBeNull();
  });
});

describe("Exit (operator) during a session", () => {
  it("waits for the photo being taken, then closes the session through the same reset", async () => {
    const onExited = exited;
    mount();
    await toPhotoScreen();
    setShoot(shoot("CAPTURING", 1));
    fireEvent.click(screen.getByRole("button", { name: "Exit Booth Mode" }));
    fireEvent.click(await screen.findByRole("button", { name: "Exit Booth" }));
    await new Promise((r) => setTimeout(r, 350));
    expect(api.recoverShooting).not.toHaveBeenCalled(); // the shutter is still in flight
    expect(onExited).not.toHaveBeenCalled();
    S.shoot = shoot("REVIEW", 2);
    redraw();
    await waitFor(() => expect(onExited).toHaveBeenCalled(), { timeout: 3000 });
    expect(api.recoverShooting).toHaveBeenCalledTimes(1);
    expect(h.leave).toHaveBeenCalled();
  });

  it("the exit dialog is not blocked by the leave dialog and Home stays a separate control", async () => {
    mount(); await toPhotoScreen();
    setShoot(shoot("REVIEW", 4));
    fireEvent.click(screen.getByRole("button", { name: "Exit Booth Mode" }));
    const dlg = await screen.findByRole("dialog", { name: "Exit Booth Mode and return to Studio?" });
    expect(dlg.textContent).toMatch(/Photos already taken stay in the library/);
  });
});

describe("Finish and the next session", () => {
  it("rapid Finish taps start one reset and show Welcome at once", async () => {
    let done!: () => void;
    api.recoverShooting.mockReturnValue(new Promise<void>((r) => { done = r; }));
    mount(); await toPhotoScreen();
    setShoot(shoot("COMPLETED", 4));
    const fin = screen.getByRole("button", { name: "Finish" });
    fireEvent.click(fin); fireEvent.click(fin); fireEvent.click(fin);
    expect(screen.getByText("Say cheese!")).toBeTruthy();
    expect(api.recoverShooting).toHaveBeenCalledTimes(1);
    await act(async () => { done(); });
  });

  it("a new session cannot start until the previous reset has finished, and then starts normally", async () => {
    let done!: () => void;
    api.recoverShooting.mockReturnValue(new Promise<void>((r) => { done = r; }));
    mount(); await toPhotoScreen();
    setShoot(shoot("COMPLETED", 4));
    fireEvent.click(screen.getByRole("button", { name: "Finish" }));
    const start = screen.getByRole("button", { name: /One moment/ }) as HTMLButtonElement;
    expect(start.disabled).toBe(true);
    fireEvent.click(start);
    expect(screen.queryByTestId("frame-selection")).toBeNull();
    await act(async () => { done(); });
    S.shoot = shoot("IDLE"); redraw();
    const go = await screen.findByRole("button", { name: /Take photos/ });
    expect((go as HTMLButtonElement).disabled).toBe(false);
    api.prepareMode.mockClear();
    await toPhotoScreen();
    expect(api.prepareMode).toHaveBeenCalledTimes(1);
  });

  it("choosing a frame waits for a reset that is still running before the camera is prepared", async () => {
    mount();
    fireEvent.click(screen.getByRole("button", { name: /Take photos/ }));
    await waitFor(() => expect(screen.getByTestId("frame-selection")).toBeTruthy());
    await waitFor(() => expect((screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement).disabled).toBe(false));
    let done!: () => void;
    api.recoverShooting.mockReturnValue(new Promise<void>((r) => { done = r; }));
    void sessionGate.reset();
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await new Promise((r) => setTimeout(r, 50));
    expect(api.prepareMode).not.toHaveBeenCalled();
    await act(async () => { done(); });
    await waitFor(() => expect(api.prepareMode).toHaveBeenCalledTimes(1));
  });

  it("a failed reset is shown calmly with a retry, never a permanent loading screen", async () => {
    api.recoverShooting.mockRejectedValueOnce(new Error("the photos are still being saved"));
    mount(); await toPhotoScreen();
    setShoot(shoot("COMPLETED", 4));
    fireEvent.click(screen.getByRole("button", { name: "Finish" }));
    await screen.findByText(/still tidying up the last session/);
    const take = screen.getByRole("button", { name: /Take photos/ }) as HTMLButtonElement;
    expect(take.disabled).toBe(false);               // the guest is never stuck behind it
    expect(screen.getByText(/Host: the photos are still being saved/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(screen.queryByText(/still tidying up/)).toBeNull());
  });
});

describe("readiness on the Welcome screen", () => {
  const welcomeButton = () => screen.getByRole("button", { name: /Take photos/ }) as HTMLButtonElement;

  it("camera disconnected before starting: guests cannot start and see a calm message, not a technical one", () => {
    S.camera = cam({ device: undefined });
    S.readiness = ready({ status: "setup_required", canStart: false, headline: "Setup required", guest: "The photo booth is not ready yet. Please ask the host.", detail: "No camera is connected. Connect a camera and choose it on the Camera page." });
    mount();
    expect(welcomeButton().disabled).toBe(true);
    expect(screen.getByText(/Please ask the host/)).toBeTruthy();
    expect(document.body.textContent).not.toMatch(/Camera page|EDSDK|USB/);
    expect(screen.getByText(/Host: Setup required/)).toBeTruthy();
  });

  it("a camera that is reconnecting shows the 'Reconnecting' state", () => {
    S.readiness = ready({ status: "reconnecting", canStart: false, headline: "Reconnecting", guest: "We are getting the camera ready again. This only takes a moment." });
    mount();
    expect(welcomeButton().disabled).toBe(true);
    expect(screen.getByText(/only takes a moment/)).toBeTruthy();
    expect(screen.getByText(/Host: Reconnecting/)).toBeTruthy();
  });

  it("an unwritable library or a missing preferred camera also keeps guests out (setup required)", () => {
    S.readiness = ready({ status: "setup_required", canStart: false, headline: "Setup required", guest: "The photo booth is not ready yet. Please ask the host.",
      items: [{ key: "storage", label: "Library folder is writable", required: true, state: "problem", detail: "Cannot write to C:\\Docs: access is denied" }] });
    mount();
    expect(welcomeButton().disabled).toBe(true);
    expect(document.body.textContent).not.toMatch(/access is denied/); // detail is for the operator in Studio
  });

  it("no printer (digital only) does not stop guests", () => {
    S.readiness = ready({ status: "ready", canStart: true, items: [{ key: "printer", label: "Printer", required: false, state: "info", detail: "No printer selected — guests get digital photos only." }] });
    mount();
    expect(welcomeButton().disabled).toBe(false);
    expect(screen.getByText(/Pick a frame/)).toBeTruthy();
  });

  it("a session that is still being closed right after Finish is not shown to guests as a setup problem", () => {
    S.readiness = ready({ status: "attention", canStart: false, headline: "Operator attention required", guest: "Please ask the host for a moment.",
      items: [{ key: "session", label: "No other session running", required: true, state: "problem", detail: "A photo session is already open (COMPLETED)." }] });
    mount();
    expect(welcomeButton().disabled).toBe(false);
    expect(screen.queryByText(/ask the host/i)).toBeNull();
  });

  it("without a readiness answer yet it falls back to the camera state", () => {
    S.readiness = null; S.camera = cam({ reconnecting: true });
    mount();
    expect(welcomeButton().disabled).toBe(true);
  });
});

describe("portrait and landscape", () => {
  it("reports the orientation on the booth root and follows a rotation", () => {
    let portrait = true;
    const listeners = new Set<() => void>();
    vi.stubGlobal("matchMedia", (q: string) => ({ matches: q.includes("portrait") ? portrait : !portrait, addEventListener: (_: string, f: () => void) => listeners.add(f), removeEventListener: (_: string, f: () => void) => listeners.delete(f) }));
    mount();
    const root = () => document.querySelector(".booth-root") as HTMLElement;
    expect(root().dataset.orientation).toBe("portrait");
    expect(screen.getByRole("button", { name: "Exit Booth Mode" })).toBeTruthy();
    expect((screen.getByRole("button", { name: /Take photos/ }) as HTMLButtonElement).disabled).toBe(false);
    portrait = false;
    act(() => listeners.forEach((f) => f()));
    expect(root().dataset.orientation).toBe("landscape");
    expect(screen.getByRole("button", { name: /Take photos/ })).toBeTruthy();
    vi.unstubAllGlobals();
  });
});
