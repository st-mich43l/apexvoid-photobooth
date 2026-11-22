import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CameraSources, DeviceInfo } from "./types";

const h = vi.hoisted(() => ({
  S: {} as Record<string, unknown>,
  api: {
    listCameraSources: vi.fn(), connectCamera: vi.fn(), disconnectCamera: vi.fn(), setPreferredCamera: vi.fn(), clearPreferredCamera: vi.fn(),
    setCameraMode: vi.fn(), startLiveView: vi.fn(), stopLiveView: vi.fn(),
  },
}));
vi.mock("./state", () => ({ useApp: () => h.S }));
vi.mock("./api", () => ({ api: h.api, toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message : String(e)), { action: "" }) }));
vi.mock("./ui", async (orig) => ({ ...(await orig<typeof import("./ui")>()), LivePreview: () => <div data-testid="preview" />, SimulatedRibbon: () => null }));

import { CameraPage } from "./pages/CameraPage";

const modes = [
  { width: 1920, height: 1080, fpsMin: 5, fpsMax: 30, format: "mjpeg" },
  { width: 1280, height: 720, fpsMin: 5, fpsMax: 30, format: "mjpeg" },
];
const web = (o: Partial<DeviceInfo> = {}): DeviceInfo => ({ id: "uvc:a", name: "Integrated Webcam", driver: "uvc", kind: "webcam", simulated: false, usbId: "0bda:5830", modes, ...o });
const canon: DeviceInfo = { id: "eds:1", name: "Canon EOS R50", driver: "canon-edsdk", kind: "dslr", simulated: false, vendor: "Canon" };
const sources = (devices: DeviceInfo[], o: Partial<CameraSources> = {}): CameraSources => ({
  devices, activeId: "", photoPreferred: { id: "", name: "", driver: "", usbId: "" }, videoPreferred: { id: "", name: "", driver: "", usbId: "" },
  photoMatchId: "", videoMatchId: "", restoreNote: "", ffmpegFound: true, ...o });

beforeEach(() => {
  h.S.camera = { device: undefined, state: "disconnected", reconnecting: false, liveView: false, drivers: [{ name: "uvc", available: true }, { name: "canon-edsdk", available: false, problem: "Canon EDSDK not found" }] };
  h.S.settings = { photoCamera: { id: "", name: "" }, videoCamera: { id: "", name: "" } };
  h.S.toast = vi.fn();
  for (const f of Object.values(h.api)) f.mockReset().mockResolvedValue(undefined);
});
afterEach(cleanup);

describe("Camera sources screen", () => {
  it("lists every source with kind, backend, USB id and resolutions", async () => {
    h.api.listCameraSources.mockResolvedValue(sources([canon, web()]));
    render(<CameraPage />);
    await screen.findAllByText("Integrated Webcam");
    expect(screen.getByText(/Webcam · Windows video device/)).toBeTruthy();
    expect(screen.getByText(/USB 0bda:5830/)).toBeTruthy();
    expect(screen.getByText(/Resolutions: 1920×1080 · 1280×720/)).toBeTruthy();
    expect(screen.getByText(/Canon camera \(Canon SDK\) · Canon EDSDK/)).toBeTruthy();
    expect(screen.getByText("Canon EDSDK not found")).toBeTruthy();         // backend problem is visible
  });

  it("connects a selected source and refreshes without a restart", async () => {
    h.api.listCameraSources.mockResolvedValue(sources([web()]));
    render(<CameraPage />);
    await screen.findAllByText("Integrated Webcam");
    fireEvent.click(screen.getByRole("button", { name: "Connect" }));
    await waitFor(() => expect(h.api.connectCamera).toHaveBeenCalledWith("uvc:a"));
    h.api.listCameraSources.mockClear();
    fireEvent.click(screen.getByRole("button", { name: "Refresh devices" }));
    await waitFor(() => expect(h.api.listCameraSources).toHaveBeenCalled());
  });

  it("saves separate Photo and Video defaults, and refuses video for the Canon SDK source", async () => {
    h.api.listCameraSources.mockResolvedValue(sources([canon, web()]));
    render(<CameraPage />);
    await screen.findAllByText("Integrated Webcam");
    const videoBtns = screen.getAllByRole("button", { name: "Use for Video" }) as HTMLButtonElement[];
    expect(videoBtns).toHaveLength(1);                                       // Canon SDK: photo only, so it is not offered for video at all
    fireEvent.click(videoBtns[0]);
    await waitFor(() => expect(h.api.setPreferredCamera).toHaveBeenCalledWith("video", "uvc:a"));
    fireEvent.click(screen.getAllByRole("button", { name: "Use as the Photo camera" })[0]);
    await waitFor(() => expect(h.api.setPreferredCamera).toHaveBeenCalledWith("photo", "eds:1"));
  });

  it("blocks duplicate entries and explains why", async () => {
    h.api.listCameraSources.mockResolvedValue(sources([web(), web({ id: "uvc:b", duplicateOf: "uvc:a", note: "Same physical device as another listed source; open only one." })]));
    render(<CameraPage />);
    await screen.findByText(/open only one/);
    const connects = screen.getAllByRole("button", { name: "Connect" }) as HTMLButtonElement[];
    expect(connects[1].disabled).toBe(true);
    expect(screen.getByText("duplicate")).toBeTruthy();
  });

  it("marks the preferred sources and shows the start-up note when the preferred camera is missing", async () => {
    h.api.listCameraSources.mockResolvedValue(sources([web()], { photoMatchId: "uvc:a", restoreNote: "Your preferred photo camera “Logitech C920” was not found." }));
    render(<CameraPage />);
    await screen.findByText("photo default");
    expect(screen.getByText(/Logitech C920” was not found/)).toBeTruthy();
  });

  it("offers the supported resolutions of the connected camera and applies the choice", async () => {
    h.S.camera = { ...(h.S.camera as object), device: web(), state: "connected", liveView: true, mode: { width: 1920, height: 1080, fpsMax: 30, fpsMin: 0, format: "mjpeg" },
      capabilities: { liveView: "supported", stillCapture: "supported", originalTransfer: "unsupported", movieRecord: "unsupported", movieTransfer: "unsupported", streamedVideo: "supported", notes: [] } };
    h.api.listCameraSources.mockResolvedValue(sources([web()], { activeId: "uvc:a" }));
    render(<CameraPage />);
    const sel = (await screen.findByLabelText("Resolution and frame rate")) as HTMLSelectElement;
    expect(sel.value).toBe("1920x1080@30");
    fireEvent.change(sel, { target: { value: "1280x720@30" } });
    await waitFor(() => expect(h.api.setCameraMode).toHaveBeenCalledWith("uvc:a", { width: 1280, height: 720, fps: 30, format: "mjpeg" }));
    expect(screen.getByText(/frame grabs/)).toBeTruthy();                    // a webcam photo is a frame grab, never an original
    expect(screen.getByTestId("preview")).toBeTruthy();
  });

  it("warns when FFmpeg is missing but still lists the cameras", async () => {
    h.api.listCameraSources.mockResolvedValue(sources([web()], { ffmpegFound: false }));
    render(<CameraPage />);
    await screen.findAllByText("Integrated Webcam");
    expect(screen.getByText(/FFmpeg was not found/)).toBeTruthy();
  });

  it("shows device-specific guidance for a connection error", async () => {
    h.S.camera = { ...(h.S.camera as object), device: canon, lastError: "the camera is busy or owned by another program", lastAction: "" };
    h.api.listCameraSources.mockResolvedValue(sources([canon], { activeId: "eds:1" }));
    render(<CameraPage />);
    await screen.findByText(/owned by another program/);
    expect(screen.getByText(/Close EOS Utility/)).toBeTruthy();
  });
});
