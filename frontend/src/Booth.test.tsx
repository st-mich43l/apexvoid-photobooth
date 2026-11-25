import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Settings, ShootSnapshot, VideoState } from "./types";

// ---- mocks: state, api, window helpers and the heavy flows ----------------
const h = vi.hoisted(() => ({
  S: {} as { video: VideoState; shoot: ShootSnapshot; settings: Settings; camera: object; toast?: unknown; frameUrl?: unknown },
  api: { verifyKioskPin: vi.fn(), stopRecording: vi.fn(), cancelShooting: vi.fn(), recoverShooting: vi.fn(), prepareMode: vi.fn(), listFrames: vi.fn() },
  enter: vi.fn(), leave: vi.fn(),
}));
const { S, api, enter, leave } = h;
vi.mock("./state", () => ({ useApp: () => h.S }));
vi.mock("./api", () => ({ api: h.api, toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message : String(e)), { action: (e as { action?: string })?.action ?? "", code: (e as { code?: string })?.code, params: (e as { params?: Record<string, number> })?.params }) }));
vi.mock("./fullscreen", () => ({ enterFullscreen: (...a: unknown[]) => h.enter(...a), leaveFullscreen: (...a: unknown[]) => h.leave(...a) }));
vi.mock("./ShootFlow", () => ({
  ShootFlow: (p: { frame: { id: string; variantId: string }; onChangeFrame?: () => void }) => (
    <div data-testid="photo-flow" data-frame={`${p.frame.id}|${p.frame.variantId}`}><button onClick={p.onChangeFrame}>Change frame</button></div>
  ),
}));

import { Booth } from "./Booth";
import { sessionGate } from "./sessionGate";

const frame = (id: string, photos: number) => ({ id, name: id === "rose.strip4" ? "Rose Pink" : "Mono", category: "Korean Pastel", photos, format: "2×6 strip", enabled: true, pixelW: 600, pixelH: 1800,
  variants: [{ id: "a", name: "A" }, { id: "b", name: "B", background: "#fff" }], widthIn: 2, heightIn: 6, dpi: 300, slots: [], background: "#fcc", kind: "strip", order: 1, revision: 1 });
const video = (o: Partial<VideoState> = {}): VideoState => ({ state: "idle", sessionId: "", elapsedMs: 0, maxSeconds: 30, lastMediaId: "", ...o });
const shoot = (state: ShootSnapshot["state"]): ShootSnapshot => ({ state, version: 1, sessionId: "S", shots: 4, slot: 0, remaining: 0, taken: [], simulated: false });

beforeEach(() => {
  sessionGate._forget();
  S.video = video(); S.shoot = shoot("IDLE");
  S.settings = { shots: 4, templateId: "rose.strip4", video: { maxSeconds: 30 }, kioskPinSet: false } as unknown as Settings;
  S.frameUrl = (id: string) => `/frame/${id}`;
  api.listFrames.mockResolvedValue({ frames: [frame("rose.strip4", 4), frame("mono.strip3", 3)], categories: ["Korean Pastel"], errors: [], dir: "", default: "rose.strip4" });
  S.toast = vi.fn();
  S.camera = { device: { id: "c", name: "Cam", driver: "uvc", simulated: false }, state: "connected", reconnecting: false, liveView: true };
  enter.mockResolvedValue(undefined); leave.mockResolvedValue(undefined);
  api.prepareMode.mockResolvedValue({ mode: "photo", ready: true, switched: false, needsFallback: false, preferred: {}, message: "" });
  api.verifyKioskPin.mockResolvedValue(undefined); api.stopRecording.mockResolvedValue(undefined); api.cancelShooting.mockResolvedValue(undefined); api.recoverShooting.mockResolvedValue(undefined);
  window.location.hash = "";
});
afterEach(() => { cleanup(); vi.useRealTimers(); enter.mockReset(); leave.mockReset(); });

/** Photo → frame selection → Continue (the camera is only prepared by Continue). */
const choosePhoto = async () => {
  fireEvent.click(screen.getByRole("button", { name: /Take photos/ }));
  await waitFor(() => expect(screen.getByTestId("frame-selection")).toBeTruthy());
  await waitFor(() => expect((screen.getByRole("button", { name: "Continue" }) as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(screen.getByRole("button", { name: "Continue" }));
};
const exitButton = () => screen.getByRole("button", { name: "Exit Booth Mode" });
const dialog = () => screen.queryByRole("dialog", { name: "Exit Booth Mode and return to Studio?" });

describe("Booth exit", () => {
  it("shows a visible Exit Booth button on every screen", async () => {
    render(<Booth onExited={() => {}} />);
    expect(exitButton()).toBeTruthy();                                  // welcome
    fireEvent.click(screen.getByRole("button", { name: /Take photos/ }));
    await waitFor(() => expect(screen.getByTestId("frame-selection")).toBeTruthy());
    expect(exitButton()).toBeTruthy();                                  // frame selection
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(screen.getByTestId("photo-flow")).toBeTruthy());
    expect(exitButton()).toBeTruthy();                                  // photo
  });

  it("keeps Home and Exit as different controls", () => {
    window.location.hash = "#booth/photo";
    render(<Booth onExited={() => {}} />);
    const home = screen.getByRole("button", { name: /Home/ });
    fireEvent.click(home);
    expect(dialog()).toBeNull();                                         // Home never opens the exit dialog
    expect(screen.getByText("Say cheese!")).toBeTruthy();
  });

  it("opens the confirmation from the button, with the required wording", () => {
    render(<Booth onExited={() => {}} />);
    fireEvent.click(exitButton());
    expect(dialog()).toBeTruthy();
    expect(screen.getByText("Exit Booth Mode and return to Studio?")).toBeTruthy();
  });

  it("opens the same confirmation with the Escape key", () => {
    render(<Booth onExited={() => {}} />);
    fireEvent.keyDown(window, { key: "Escape" });
    expect(dialog()).toBeTruthy();
  });

  it("Escape inside the dialog closes it again (Stay in Booth)", async () => {
    render(<Booth onExited={() => {}} />);
    fireEvent.keyDown(window, { key: "Escape" });
    fireEvent.keyDown(window, { key: "Escape" });
    await waitFor(() => expect(dialog()).toBeNull());
  });

  it("opens the confirmation after holding the corner for 3 s (and not for a short tap)", () => {
    vi.useFakeTimers();
    render(<Booth onExited={() => {}} />);
    const c = screen.getByTestId("exit-corner");
    fireEvent.pointerDown(c); act(() => { vi.advanceTimersByTime(1000); }); fireEvent.pointerUp(c);
    act(() => { vi.advanceTimersByTime(5000); });
    expect(dialog()).toBeNull();
    fireEvent.pointerDown(c); act(() => { vi.advanceTimersByTime(3100); });
    expect(dialog()).toBeTruthy();
  });

  it("exits and restores the window only after confirming", async () => {
    const onExited = vi.fn();
    render(<Booth onExited={onExited} />);
    fireEvent.click(exitButton());
    expect(onExited).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Stay in Booth" }));
    expect(dialog()).toBeNull();
    expect(leave).not.toHaveBeenCalled();
    fireEvent.click(exitButton());
    fireEvent.click(screen.getByRole("button", { name: "Exit Booth" }));
    await waitFor(() => expect(onExited).toHaveBeenCalledTimes(1));
    expect(leave).toHaveBeenCalledTimes(1);
  });

  it("works from the error state of a capture", async () => {
    S.shoot = shoot("ERROR");
    const onExited = vi.fn();
    render(<Booth onExited={onExited} />);
    fireEvent.click(exitButton());
    fireEvent.click(screen.getByRole("button", { name: "Exit Booth" }));
    await waitFor(() => expect(onExited).toHaveBeenCalled());
  });

  it("stops and saves a running recording before leaving, and says so", async () => {
    S.video = video({ state: "recording" });
    api.stopRecording.mockImplementation(async () => { S.video = video({ state: "idle", lastMediaId: "M1" }); });
    const onExited = vi.fn();
    render(<Booth onExited={onExited} />);
    fireEvent.click(exitButton());
    expect(screen.getByText(/stopped and saved/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Stop recording & exit" }));
    await waitFor(() => expect(onExited).toHaveBeenCalled());
    expect(api.stopRecording.mock.invocationCallOrder[0]).toBeLessThan(leave.mock.invocationCallOrder[0]);
  });

  it("does not leave when saving the recording fails", async () => {
    S.video = video({ state: "recording" });
    api.stopRecording.mockImplementation(async () => { S.video = video({ state: "error", error: "disk full" }); });
    const onExited = vi.fn();
    render(<Booth onExited={onExited} />);
    fireEvent.click(exitButton());
    fireEvent.click(screen.getByRole("button", { name: "Stop recording & exit" }));
    await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/disk full/));
    expect(onExited).not.toHaveBeenCalled();
    expect(leave).not.toHaveBeenCalled();
  });

  it("cancels an unfinished photo session but never deletes anything", async () => {
    S.shoot = shoot("REVIEW");
    api.recoverShooting.mockImplementation(async () => { S.shoot = shoot("IDLE"); });
    render(<Booth onExited={() => {}} />);
    fireEvent.click(exitButton());
    expect(screen.getByText(/Photos already taken stay in the library/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Exit Booth" }));
    await waitFor(() => expect(api.recoverShooting).toHaveBeenCalled());
  });

  it("reports a fullscreen restore failure and offers retry / continue", async () => {
    leave.mockRejectedValueOnce(new Error("The window is still fullscreen."));
    const onExited = vi.fn();
    render(<Booth onExited={onExited} />);
    fireEvent.click(exitButton());
    fireEvent.click(screen.getByRole("button", { name: "Exit Booth" }));
    await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/still fullscreen/));
    expect(onExited).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(onExited).toHaveBeenCalledTimes(1));
  });

  it("lets the operator continue to Studio when the window cannot be restored", async () => {
    leave.mockRejectedValue(new Error("The window is still fullscreen."));
    const onExited = vi.fn();
    render(<Booth onExited={onExited} />);
    fireEvent.click(exitButton());
    fireEvent.click(screen.getByRole("button", { name: "Exit Booth" }));
    await waitFor(() => screen.getByRole("button", { name: "Continue to Studio anyway" }));
    fireEvent.click(screen.getByRole("button", { name: "Continue to Studio anyway" }));
    expect(onExited).toHaveBeenCalledTimes(1);
  });

  it("shows a visible message when entering fullscreen fails", async () => {
    enter.mockRejectedValue(new Error("The window did not switch to fullscreen."));
    render(<Booth onExited={() => {}} />);
    await waitFor(() => expect(screen.getByText(/Fullscreen is not active/)).toBeTruthy());
    enter.mockResolvedValue(undefined);
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.queryByText(/Fullscreen is not active/)).toBeNull());
  });
});

describe("Booth exit with kiosk PIN", () => {
  beforeEach(() => { S.settings = { ...S.settings, kioskPinSet: true }; });

  it("requires the PIN and does not exit on a wrong one", async () => {
    api.verifyKioskPin.mockRejectedValueOnce(new Error("Wrong PIN."));
    const onExited = vi.fn();
    render(<Booth onExited={onExited} />);
    fireEvent.click(exitButton());
    const exit = screen.getByRole("button", { name: "Exit Booth" }) as HTMLButtonElement;
    expect(exit.disabled).toBe(true);                                    // no PIN yet
    fireEvent.change(screen.getByLabelText("Operator PIN"), { target: { value: "1111" } });
    fireEvent.click(exit);
    await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/Wrong PIN/));
    expect(onExited).not.toHaveBeenCalled();
    expect(leave).not.toHaveBeenCalled();
    expect((screen.getByLabelText("Operator PIN") as HTMLInputElement).value).toBe("");   // cleared after a failure
  });

  it("exits with the right PIN, also via the Enter key", async () => {
    const onExited = vi.fn();
    render(<Booth onExited={onExited} />);
    fireEvent.keyDown(window, { key: "Escape" });
    const input = screen.getByLabelText("Operator PIN");
    fireEvent.change(input, { target: { value: "2468" } });
    fireEvent.keyDown(input, { key: "Enter" });
    await waitFor(() => expect(onExited).toHaveBeenCalled());
    expect(api.verifyKioskPin).toHaveBeenCalledWith("2468");
  });

  it("only accepts digits in the PIN field", () => {
    render(<Booth onExited={() => {}} />);
    fireEvent.click(exitButton());
    const input = screen.getByLabelText("Operator PIN") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "12ab34" } });
    expect(input.value).toBe("1234");
  });
});


describe("Booth start button prepares the photo camera", () => {

  it("opens the flow when the preferred camera is ready", async () => {
    render(<Booth onExited={() => {}} />);
    await choosePhoto();
    await waitFor(() => expect(screen.getByTestId("photo-flow")).toBeTruthy());
    expect(api.prepareMode).toHaveBeenCalledWith("photo", "");
  });

  it("asks before using a different camera and does not switch by itself", async () => {
    const cand = { id: "uvc:a", name: "Integrated Webcam", driver: "uvc", kind: "webcam", simulated: false };
    api.prepareMode.mockResolvedValueOnce({ mode: "photo", ready: false, switched: false, needsFallback: true, candidate: cand, preferred: { name: "Logitech C920" }, message: "Your preferred photo camera “Logitech C920” is not available." });
    render(<Booth onExited={() => {}} />);
    await choosePhoto();
    await waitFor(() => screen.getByText("Use a different camera?"));
    expect(screen.getByText(/Logitech C920/)).toBeTruthy();
    expect(screen.queryByTestId("photo-flow")).toBeNull();
    expect(api.prepareMode).toHaveBeenCalledTimes(1);                   // nothing was switched yet
    api.prepareMode.mockResolvedValueOnce({ mode: "photo", ready: true, switched: true, needsFallback: false, preferred: {}, message: "Using Integrated Webcam for Photo mode." });
    fireEvent.click(screen.getByRole("button", { name: "Use Integrated Webcam" }));
    await waitFor(() => expect(screen.getByTestId("photo-flow")).toBeTruthy());
    expect(api.prepareMode).toHaveBeenLastCalledWith("photo", "uvc:a");
  });

  it("stays on frame selection (no flow, no switch) when the operator declines the substitute", async () => {
    const cand = { id: "uvc:a", name: "Integrated Webcam", driver: "uvc", kind: "webcam", simulated: false };
    api.prepareMode.mockResolvedValueOnce({ mode: "photo", ready: false, switched: false, needsFallback: true, candidate: cand, preferred: {}, message: "No photo camera has been chosen." });
    render(<Booth onExited={() => {}} />);
    await choosePhoto();
    await waitFor(() => screen.getByText("Use a different camera?"));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByText("Use a different camera?")).toBeNull());
    expect(screen.getByTestId("frame-selection")).toBeTruthy();
    expect(screen.queryByTestId("photo-flow")).toBeNull();
    expect(api.prepareMode).toHaveBeenCalledTimes(1);
  });

  it("requires the kiosk PIN to approve a substitute", async () => {
    S.settings = { ...S.settings, kioskPinSet: true };
    const cand = { id: "uvc:a", name: "Integrated Webcam", driver: "uvc", kind: "webcam", simulated: false };
    api.prepareMode.mockResolvedValueOnce({ mode: "photo", ready: false, switched: false, needsFallback: true, candidate: cand, preferred: {}, message: "m" });
    render(<Booth onExited={() => {}} />);
    await choosePhoto();
    await waitFor(() => screen.getByText("Use a different camera?"));
    const use = screen.getByRole("button", { name: "Use Integrated Webcam" }) as HTMLButtonElement;
    expect(use.disabled).toBe(true);
    fireEvent.change(screen.getByLabelText("Operator PIN"), { target: { value: "2468" } });
    api.prepareMode.mockResolvedValueOnce({ mode: "photo", ready: true, switched: true, needsFallback: false, preferred: {}, message: "ok" });
    fireEvent.click(use);
    await waitFor(() => expect(api.verifyKioskPin).toHaveBeenCalledWith("2468"));
    await waitFor(() => expect(screen.getByTestId("photo-flow")).toBeTruthy());
  });

  it("shows why photo mode cannot start when no camera is available", async () => {
    api.prepareMode.mockRejectedValueOnce(Object.assign(new Error("No camera available for Photo mode.\nConnect a webcam and press Refresh."), {}));
    render(<Booth onExited={() => {}} />);
    await choosePhoto();
    await waitFor(() => expect(screen.getAllByRole("alert").some((a) => /camera is not ready/i.test(a.textContent ?? ""))).toBe(true));
    expect(screen.queryByTestId("photo-flow")).toBeNull();
  });
});

describe("Booth frame selection comes before the camera", () => {
  const photo = () => fireEvent.click(screen.getByRole("button", { name: /Take photos/ }));

  it("Photo opens frame selection and does NOT touch the camera until a frame is confirmed", async () => {
    render(<Booth onExited={() => {}} />);
    photo();
    await waitFor(() => expect(screen.getByRole("radio", { name: /Rose Pink/ })).toBeTruthy());
    expect(api.prepareMode).not.toHaveBeenCalled();
    expect(screen.queryByTestId("photo-flow")).toBeNull();
  });

  it("shows name, format and photo count on every card, and the preselected frame is highlighted", async () => {
    render(<Booth onExited={() => {}} />);
    photo();
    const card = await screen.findByRole("radio", { name: /Rose Pink/ });
    expect(card.textContent).toMatch(/2×6.*strip.*4 photos/);
    expect(card.getAttribute("aria-checked")).toBe("true");
    expect(screen.getByRole("radio", { name: /Mono/ }).textContent).toMatch(/3 photos/);
  });

  it("starts the flow with the chosen frame and colour variant", async () => {
    render(<Booth onExited={() => {}} />);
    photo();
    fireEvent.click(await screen.findByRole("radio", { name: /Mono/ }));
    fireEvent.click(screen.getByRole("radio", { name: "B" }));
    fireEvent.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(screen.getByTestId("photo-flow").getAttribute("data-frame")).toBe("mono.strip3|b"));
    expect(api.prepareMode).toHaveBeenCalledWith("photo", "");
  });

  it("filters by category and by photo count", async () => {
    render(<Booth onExited={() => {}} />);
    photo();
    await screen.findByRole("radio", { name: /Rose Pink/ });
    fireEvent.click(screen.getByRole("button", { name: "3 photos" }));
    expect(screen.queryByRole("radio", { name: /Rose Pink/ })).toBeNull();
    expect(screen.getByRole("radio", { name: /Mono/ })).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Any photos" }));
    fireEvent.click(screen.getByRole("button", { name: "Korean Pastel" }));
    expect(screen.getAllByRole("radio", { name: /Rose Pink|Mono/ }).length).toBeGreaterThanOrEqual(2);
  });

  it("Back returns to the welcome screen without preparing the camera or creating a session", async () => {
    render(<Booth onExited={() => {}} />);
    photo();
    await screen.findByRole("radio", { name: /Rose Pink/ });
    fireEvent.click(screen.getByRole("button", { name: /Home/ }));
    expect(screen.getByText("Say cheese!")).toBeTruthy();
    expect(api.prepareMode).not.toHaveBeenCalled();
  });

  it("goes back to welcome after 90 s without touches on the frame screen, with no camera work", async () => {
    vi.useFakeTimers();
    render(<Booth onExited={() => {}} />);
    photo();
    await act(async () => { await vi.advanceTimersByTimeAsync(50); });
    expect(screen.getByTestId("frame-selection")).toBeTruthy();
    await act(async () => { await vi.advanceTimersByTimeAsync(91_000); });
    expect(screen.getByText("Say cheese!")).toBeTruthy();
    expect(api.prepareMode).not.toHaveBeenCalled();
  });

  it("'Change frame' from the camera returns to frame selection with the same frame selected", async () => {
    render(<Booth onExited={() => {}} />);
    await choosePhoto();
    await waitFor(() => screen.getByTestId("photo-flow"));
    fireEvent.click(screen.getByRole("button", { name: "Change frame" }));
    const card = await screen.findByRole("radio", { name: /Rose Pink/ });
    expect(card.getAttribute("aria-checked")).toBe("true");
  });

  it("explains a loading failure and lets the guest try again; shows an empty state", async () => {
    api.listFrames.mockRejectedValueOnce(Object.assign(new Error("disk unreadable"), {}));
    render(<Booth onExited={() => {}} />);
    photo();
    await waitFor(() => expect(screen.getByRole("alert").textContent).toMatch(/could not be loaded/));
    api.listFrames.mockResolvedValueOnce({ frames: [], categories: [], errors: [], dir: "", default: "" });
    fireEvent.click(screen.getByRole("button", { name: "Try again" }));
    await waitFor(() => expect(screen.getByText(/No frames are available/)).toBeTruthy());
  });

  it("offers no video option: Booth Mode is photo-only", () => {
    render(<Booth onExited={() => {}} />);
    expect(screen.queryByRole("button", { name: /Video/ })).toBeNull();
    expect(screen.getAllByRole("button", { name: /Take photos/ })).toHaveLength(1);
  });
});


it("localizes structured PIN lockout without allowing exit", async () => {
  S.settings = { ...S.settings, kioskPinSet: true, guestLanguage: "vi", language: "en" };
  api.verifyKioskPin.mockRejectedValueOnce(Object.assign(new Error("locked"), { code: "pin_locked", params: { seconds: 18 } }));
  const exited = vi.fn(); render(<Booth onExited={exited} />);
  fireEvent.click(screen.getByRole("button", { name: "Thoát chế độ buồng chụp" }));
  fireEvent.change(screen.getByLabelText("Mã PIN nhân viên"), { target: { value: "1111" } });
  fireEvent.click(screen.getByRole("button", { name: "Thoát buồng chụp" }));
  expect((await screen.findByRole("alert")).textContent).toContain("18 giây");
  expect(exited).not.toHaveBeenCalled();
});
