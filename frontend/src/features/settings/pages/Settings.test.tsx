import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Settings } from "../../../shared/types";

const h = vi.hoisted(() => ({ S: {} as Record<string, unknown>, api: { patchSettings: vi.fn(), saveSettings: vi.fn() } }));
vi.mock("../../../app/state", () => ({ useApp: () => h.S }));
vi.mock("../../../shared/api", () => ({ api: h.api, toAppError: (e: unknown) => Object.assign(new Error(String(e)), { action: "" }) }));
vi.mock("../MigrationDialog", () => ({ MigrationDialog: () => null }));
import { SettingsPage } from "./Settings";

const base = (): Settings => ({ mediaDir: "", countdownSeconds: 3, shots: 4, templateId: "plain.strip4", outputDpi: 300, previewMirror: false, autoReconnect: true, enableMock: false, printerName: "",
  photoCamera: { id: "", name: "", driver: "", usbId: "" }, videoCamera: { id: "", name: "", driver: "", usbId: "" }, kioskPinSet: false, video: { width: 1280, height: 720, fps: 30, maxSeconds: 30 } } as unknown as Settings);

let view: ReturnType<typeof render>;
const set = (s: Settings) => { h.S.settings = s; view.rerender(<SettingsPage />); };

beforeEach(() => {
  h.S = { settings: base(), setSettings: vi.fn(), toast: vi.fn(), bumpLibrary: vi.fn(), location: { path: "C:\\Docs", source: "default", defaultPath: "C:\\Docs", legacyPath: "", migrationPending: false, restartRequired: false, warning: "", configPath: "C:\\cfg", freeBytes: 5e10 } };
  h.api.patchSettings.mockReset().mockImplementation(async (p: Partial<Settings>) => ({ restartRequired: false, settings: { ...(h.S.settings as Settings), ...p } }));
  view = render(<SettingsPage />);
});
afterEach(cleanup);

describe("Settings: a stale copy can never overwrite newer preferences", () => {
  it("saves only what this page changed", async () => {
    fireEvent.change(screen.getByLabelText(/Countdown/), { target: { value: "5" } });
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
    await waitFor(() => expect(h.api.patchSettings).toHaveBeenCalledTimes(1));
    expect(h.api.patchSettings).toHaveBeenCalledWith({ countdownSeconds: 5 });
    expect(h.api.saveSettings).not.toHaveBeenCalled();
  });

  it("a background update (camera chosen elsewhere, frame preselected) is kept while the page has unsaved edits", async () => {
    fireEvent.change(screen.getByLabelText(/Countdown/), { target: { value: "6" } });
    // meanwhile another page changed other preferences
    set({ ...base(), templateId: "rose.strip4", printerName: "Lab", photoCamera: { id: "c1", name: "Cam", driver: "uvc", usbId: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
    await waitFor(() => expect(h.api.patchSettings).toHaveBeenCalled());
    // only the countdown is sent: the newer template/printer/camera are never rolled back by this stale page
    expect(h.api.patchSettings).toHaveBeenCalledWith({ countdownSeconds: 6 });
  });

  it("an untouched page simply follows background updates and shows nothing to save", () => {
    set({ ...base(), countdownSeconds: 8 });
    expect((screen.getByLabelText(/Countdown/) as HTMLInputElement).value).toBe("8");
    expect((screen.getByRole("button", { name: "Save settings" }) as HTMLButtonElement).disabled).toBe(true);
  });
});
