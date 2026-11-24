import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BoothReadiness, ReadinessItem } from "../types";

const h = vi.hoisted(() => ({
  S: {} as Record<string, unknown>,
  api: { listMedia: vi.fn(), librarySummary: vi.fn(), listPrinters: vi.fn() },
}));
vi.mock("../state", async () => {
  const { translate, formatTranslation } = await import("../i18n");
  return {
    useApp: () => h.S,
    useT: () => {
      const language = (h.S.settings as { language?: string } | undefined)?.language;
      return {
        t: (key: Parameters<typeof translate>[1]) => translate(language, key),
        tf: (key: Parameters<typeof formatTranslation>[1], values: Record<string, string | number>) => formatTranslation(language, key, values),
      };
    },
  };
});
vi.mock("../api", () => ({ api: h.api, toAppError: (e: unknown) => e }));
import { Dashboard } from "./Dashboard";

const lib = { photos: 3, videos: 0, sessions: 2, freeBytes: 50 * 1024 ** 3, interrupted: [], recovered: 0, missing: 0 };
const item = (key: string, state: ReadinessItem["state"], detail: string, o: Partial<ReadinessItem> = {}): ReadinessItem => ({ key, label: key, required: true, state, detail, ...o });
const readiness = (o: Partial<BoothReadiness> = {}): BoothReadiness => ({ status: "ready", canStart: true, headline: "Ready for guests", guest: "", items: [], at: "", ...o });
const go = vi.fn();
const onBooth = vi.fn();
const refresh = vi.fn();

beforeEach(() => {
  go.mockReset(); onBooth.mockReset(); refresh.mockReset();
  h.S = {
    camera: { device: { id: "c", name: "Canon EOS R50", driver: "canon-edsdk", simulated: false }, state: "connected", reconnecting: false, liveView: false },
    boot: { dataDir: "C:\\Docs\\apexvoid-studio", library: lib }, settings: { printerName: "" }, mediaUrl: () => "", libraryVersion: 0,
    readiness: readiness(), refreshReadiness: refresh,
  };
  h.api.listMedia.mockResolvedValue([]); h.api.librarySummary.mockResolvedValue(lib);
  h.api.listPrinters.mockResolvedValue({ printers: [], driver: "fake", default: "", note: "" });
});
afterEach(() => { cleanup(); vi.useRealTimers(); });

const draw = async () => { render(<Dashboard go={go} onBooth={onBooth} />); await waitFor(() => expect(h.api.listPrinters).toHaveBeenCalled()); };

describe("Studio dashboard shows whether the booth can take guests", () => {
  it("Ready for guests: no attention list, the Booth launch is the main action", async () => {
    await draw();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Ready for guests");
    expect(screen.queryByRole("region", { name: "Needs attention" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /Open Booth Mode/ }));
    expect(onBooth).toHaveBeenCalledTimes(1);
  });

  it("Setup required (no camera): says what to do and one tap goes to camera setup", async () => {
    h.S.readiness = readiness({ status: "setup_required", canStart: false, headline: "Setup required", detail: "No camera is connected. Connect a camera and choose it on the Camera page.",
      items: [item("camera", "problem", "No camera is connected.", { action: "Connect a camera and choose it on the Camera page." })] });
    h.S.camera = { device: undefined, state: "disconnected", reconnecting: false, liveView: false };
    await draw();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Setup required");
    const todo = screen.getByRole("region", { name: "Needs attention" });
    expect(todo.textContent).toMatch(/Connect a camera/);
    fireEvent.click(screen.getAllByRole("button", { name: "Camera setup" })[0]);
    expect(go).toHaveBeenCalledWith("camera");
  });

  it("an unwritable library is explained and linked to settings", async () => {
    h.S.readiness = readiness({ status: "setup_required", canStart: false, headline: "Setup required",
      items: [item("storage", "problem", "Cannot write to C:\\Docs: access is denied", { action: "Choose another media folder in Settings." })] });
    await draw();
    expect(screen.getByText(/access is denied/)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Library settings" }));
    expect(go).toHaveBeenCalledWith("settings");
  });

  it("Reconnecting is its own state", async () => {
    h.S.readiness = readiness({ status: "reconnecting", canStart: false, headline: "Reconnecting", detail: "The camera was disconnected; reconnecting automatically.", items: [item("camera", "problem", "disconnected")] });
    h.S.camera = { device: { id: "c", name: "Canon", driver: "x", simulated: false }, state: "connected", reconnecting: true, liveView: false };
    await draw();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Reconnecting");
    expect(document.querySelector(".dash-hero.status-reconnecting")).toBeTruthy();
    expect(screen.getAllByText("Reconnecting…").length).toBeGreaterThan(0);
  });

  it("no printer chosen is digital-only: not a problem, not on the attention list, guests are not blocked", async () => {
    h.S.readiness = readiness({ items: [item("printer", "info", "No printer selected — guests get digital photos only.", { required: false })] });
    await draw();
    expect(screen.queryByRole("region", { name: "Needs attention" })).toBeNull();
    expect(screen.getByText("Digital only")).toBeTruthy();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Ready for guests");
  });

  it("an unavailable printer is a warning with a link to printer setup, never 'Setup required'", async () => {
    h.S.settings = { printerName: "Lab 4x6" };
    h.S.readiness = readiness({ status: "attention", headline: "Ready — operator attention suggested",
      items: [item("printer", "warn", "“Lab 4x6” is not installed.", { required: false, action: "Choose another printer. Photos are saved digitally either way." })] });
    await draw();
    expect(screen.getByRole("heading", { level: 1 }).textContent).toMatch(/Ready/);
    expect(screen.getByRole("heading", { level: 1 }).textContent).not.toMatch(/Setup required/);
    fireEvent.click(screen.getAllByRole("button", { name: "Printer setup" })[0]);
    expect(go).toHaveBeenCalledWith("printer");
    expect(screen.getByText("Not found")).toBeTruthy();
  });

  it("never claims paper was produced: the printer card says what Windows reports and that paper is not checked", async () => {
    h.S.settings = { printerName: "Lab" };
    h.api.listPrinters.mockResolvedValue({ printers: [{ name: "Lab", default: false, status: "ready" }], driver: "x", default: "", note: "" });
    await draw();
    await screen.findByText("Windows: ready");
    expect(screen.getByText(/Whether paper comes out is not checked/)).toBeTruthy();
    expect(document.body.textContent).not.toMatch(/printed successfully|paper was printed/i);
  });

  it("the printer status is refreshed on its own: a printer that goes offline after the page opened shows up", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    h.S.settings = { printerName: "Lab" };
    h.api.listPrinters.mockResolvedValue({ printers: [{ name: "Lab", default: false, status: "ready" }], driver: "x", default: "", note: "" });
    await draw();
    await screen.findByText("Windows: ready");
    h.api.listPrinters.mockResolvedValue({ printers: [{ name: "Lab", default: false, status: "offline" }], driver: "x", default: "", note: "" });
    await act(async () => { vi.advanceTimersByTime(10_500); });
    await waitFor(() => expect(screen.getByText("offline")).toBeTruthy());
  });

  it("asks for a fresh readiness answer when the page opens", async () => {
    await draw();
    expect(refresh).toHaveBeenCalled();
  });
});
