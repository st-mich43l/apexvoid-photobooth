import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { BoothReadiness, DeviceInfo, ReadinessItem } from "../../../shared/types";

const h = vi.hoisted(() => ({
  S: {} as Record<string, unknown>,
  api: {
    listPrinters: vi.fn(), listFrames: vi.fn(), librarySummary: vi.fn(), listCameraSources: vi.fn(), openLibraryFolder: vi.fn(), testPrint: vi.fn(), printJobs: vi.fn(),
    boothReadiness: vi.fn(), patchSettings: vi.fn(), shootState: vi.fn(), startPreview: vi.fn(), startShooting: vi.fn(), startShootingWithCount: vi.fn(), accept: vi.fn(), labelTestSession: vi.fn(),
    recoverShooting: vi.fn(), startLiveView: vi.fn(), stopLiveView: vi.fn(), connectCamera: vi.fn(), disconnectCamera: vi.fn(), setPreferredCamera: vi.fn(), clearPreferredCamera: vi.fn(),
    diagnostics: vi.fn(), diagnosticSummary: vi.fn(), pickFolder: vi.fn(), exportSupportBundle: vi.fn(), listMedia: vi.fn(), printerCapabilities: vi.fn(),
  },
}));
vi.mock("../../../app/state", async () => {
  const { translate, formatTranslation } = await import("../../../shared/i18n");
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
vi.mock("../../../shared/api", () => ({ api: h.api, on: () => () => {}, toAppError: (e: unknown) => Object.assign(new Error(e instanceof Error ? e.message : String(e)), { action: (e as { action?: string })?.action ?? "" }) }));
vi.mock("../../../shared/ui", async (orig) => ({ ...(await orig<typeof import("../../../shared/ui")>()), LivePreview: () => <div data-testid="preview" /> }));
vi.mock("../../camera/CanonPanel", () => ({ CanonPanel: () => null }));

import { sessionGate } from "../../booth/sessionGate";
import { CameraPage } from "../../camera/pages/CameraPage";
import { DiagnosticsPage } from "../../diagnostics/DiagnosticsPage";
import { PrinterPage } from "../../printer/pages/PrinterPage";
import { CameraTest } from "../../camera/operator/CameraTest";
import { PreparePage } from "./PreparePage";
import { QuickSettings } from "./QuickSettings";
import { blockers, groupView, itemsOf, readinessDetail, viewOf } from "./status";
import { translate } from "../../../shared/i18n";
import { useOpenBooth } from "./useOpenBooth";
import { useLatest } from "./latest";

const item = (key: string, group: ReadinessItem["group"], state: ReadinessItem["state"], o: Partial<ReadinessItem> = {}): ReadinessItem =>
  ({ key, group, label: key, required: !["printer", "space", "interrupted"].includes(key), state, detail: `${key} detail`, ...o });
const R = (items: ReadinessItem[], o: Partial<BoothReadiness> = {}): BoothReadiness =>
  ({ status: "ready", canStart: !items.some((i) => i.required && i.state === "problem"), headline: "Ready for guests", guest: "", items, at: "", demo: false, printing: "digital_only", ...o });
const web: DeviceInfo = { id: "uvc:a", name: "Logitech C920", driver: "uvc", kind: "webcam", simulated: false };
const sim: DeviceInfo = { id: "mock-0", name: "SIMULATED camera (mock)", driver: "mock", kind: "simulated", simulated: true };
const OK = [item("camera", "camera", "ok"), item("capture", "camera", "ok", { verification: "unverified" }), item("storage", "storage", "ok"), item("space", "storage", "ok"), item("frames", "frames", "ok", { detail: "12 offered" })];
const caps = { liveView: "supported", stillCapture: "unverified", originalTransfer: "unsupported", movieRecord: "unsupported", movieTransfer: "unsupported", streamedVideo: "supported" };

beforeEach(() => {
  for (const f of Object.values(h.api)) f.mockReset().mockResolvedValue(undefined);
  h.api.listPrinters.mockResolvedValue({ printers: [{ name: "Lab", default: true, status: "ready" }], driver: "fake", default: "Lab", note: "" });
  h.api.listFrames.mockResolvedValue({ frames: [{ id: "plain.strip4", name: "Plain", format: "2×6 strip", photos: 4 }], categories: [], errors: [], dir: "", default: "plain.strip4" });
  h.api.librarySummary.mockResolvedValue({ photos: 0, videos: 0, sessions: 0, freeBytes: 50 * 1024 ** 3, interrupted: [], recovered: 0, missing: 0 });
  h.api.listCameraSources.mockResolvedValue({ devices: [web], activeId: "", photoPreferred: {}, videoPreferred: {}, photoMatchId: "", videoMatchId: "", restoreNote: "", ffmpegFound: true });
  h.api.printJobs.mockResolvedValue([]); h.api.listMedia.mockResolvedValue([]);
  h.api.boothReadiness.mockResolvedValue(R(OK));
  h.api.shootState.mockResolvedValue({ state: "IDLE", sessionId: "", taken: [], shots: 1 });
  sessionGate._forget();
  h.S = {
    camera: { device: web, state: "connected", reconnecting: false, liveView: false, capabilities: caps, drivers: [{ name: "uvc", available: true }, { name: "canon-edsdk", available: false, problem: "Canon SDK not found" }] },
    settings: { printerName: "", printPaper: "", photoCamera: { id: "", name: "" }, videoCamera: { id: "", name: "" }, countdownSeconds: 3, previewMirror: false, outputDpi: 300, autoReconnect: true, templateId: "" },
    readiness: R(OK), refreshReadiness: vi.fn(), location: { path: "C:\\Docs\\apexvoid", freeBytes: 50 * 1024 ** 3 }, boot: { library: { photos: 0, videos: 0, sessions: 0, freeBytes: 0, interrupted: [] } },
    frameUrl: (id: string) => `/f/${id}`, mediaUrl: (id: string) => `/m/${id}`, toast: vi.fn(), setSettings: vi.fn(), shoot: { state: "IDLE", taken: [] },
  };
});
afterEach(cleanup);
const prepare = async (go = vi.fn(), open = vi.fn()) => { render(<PreparePage go={go} onOpenBooth={open} />); await waitFor(() => expect(h.api.listPrinters).toHaveBeenCalled()); return { go, open }; };
const card = (name: string) => screen.getByRole("region", { name });

describe("readiness vocabulary", () => {
  it("summarizes backend readiness codes in the selected language", () => {
    const r = R([], { status: "demo", detail: "A SIMULATED camera is connected." });
    expect(readinessDetail(r, (key) => translate("vi", key))).toContain("Máy ảnh mô phỏng");
    expect(readinessDetail(r, (key) => translate("en", key))).toContain("A simulated camera");
  });
  it("maps backend lines to the six operator states", () => {
    expect(viewOf(item("camera", "camera", "problem"))).toBe("setup");
    expect(viewOf(item("printer", "printer", "problem", { required: false }))).toBe("warning");
    expect(viewOf(item("printer", "printer", "warn"))).toBe("unavailable");
    expect(viewOf(item("space", "storage", "warn"))).toBe("warning");
    expect(viewOf(item("camera", "camera", "ok", { verification: "demo" }))).toBe("demo");
    expect(viewOf(item("capture", "camera", "ok", { verification: "unverified" }))).toBe("unverified");
    expect(viewOf(item("storage", "storage", "ok"))).toBe("ready");
  });
  it("a section shows its most urgent line, and required problems are the only blockers", () => {
    const items = [item("camera", "camera", "ok", { verification: "demo" }), item("capture", "camera", "problem"), item("printer", "printer", "warn")];
    expect(groupView(itemsOf(R(items), "camera"))).toBe("setup");
    expect(blockers(R(items)).map((i) => i.key)).toEqual(["capture"]);
  });
});

describe("Prepare Booth", () => {
  it("initial set-up with no camera: Setup required, and the way forward is one tap", async () => {
    h.S.camera = { device: undefined, state: "disconnected", reconnecting: false, liveView: false, drivers: [] };
    h.S.readiness = R([item("camera", "camera", "problem", { detail: "No camera is connected.", action: "Connect a camera and choose it on the Camera page." }), ...OK.slice(2)], { status: "setup_required", headline: "Setup required" });
    const { go } = await prepare();
    expect(card("Camera").getAttribute("data-view")).toBe("setup");
    expect(within(card("Camera")).getAllByText("Setup required").length).toBeGreaterThan(0);
    expect(within(card("Camera")).getByText(/No camera is connected/)).toBeTruthy();
    fireEvent.click(within(card("Camera")).getByRole("button", { name: "Choose a camera" }));
    expect(go).toHaveBeenCalledWith("camera");
  });

  it("a valid webcam-only set-up is Ready and never mentions the Canon SDK", async () => {
    await prepare();
    expect(card("Camera").getAttribute("data-view")).toBe("unverified"); // present, but no photo has proved it yet
    expect(screen.getByText(/frame grabs/)).toBeTruthy();
    expect(document.body.textContent).not.toMatch(/Canon SDK|EDSDK/);
    expect(screen.getByRole("heading", { name: "Ready for guests" })).toBeTruthy();
  });

  it("a SIMULATED camera is Demo / test mode, never plain Ready", async () => {
    h.S.camera = { ...(h.S.camera as object), device: sim };
    h.S.readiness = R([item("camera", "camera", "ok", { verification: "demo", detail: "SIMULATED camera — demo/test only." }), item("capture", "camera", "ok", { verification: "demo" }), ...OK.slice(2)], { status: "demo", headline: "Demo / test mode", demo: true });
    await prepare();
    expect(card("Camera").getAttribute("data-view")).toBe("demo");
    expect(within(card("Camera")).getAllByText("Demo / test mode").length).toBeGreaterThan(0);
    expect(screen.getByText(/Not a hardware check/)).toBeTruthy();
    expect(within(card("Camera")).queryByText("Ready")).toBeNull();
  });

  it("digital-only is a normal choice: nothing is wrong and there is no test-print button", async () => {
    await prepare();
    const p = card("Printer");
    expect(within(p).getByText("Digital only")).toBeTruthy();
    expect(within(p).getByText(/guests get digital photos only/i)).toBeTruthy();
    expect(within(p).queryByRole("button", { name: /Test print/ })).toBeNull();
    expect(p.getAttribute("data-view")).toBe("info");
  });

  it("a printer that is configured but offline says photos are still saved, and cannot be test-printed", async () => {
    (h.S.settings as Record<string, unknown>).printerName = "Lab"; (h.S.settings as Record<string, unknown>).printPaper = "4x6 in";
    h.api.listPrinters.mockResolvedValue({ printers: [{ name: "Lab", default: true, status: "offline" }], driver: "fake", default: "Lab", note: "" });
    h.S.readiness = R([...OK, item("printer", "printer", "warn", { detail: "Lab is offline.", action: "Check the printer. Photos are saved digitally either way." })], { printing: "unavailable", status: "attention" });
    await prepare();
    const p = card("Printer");
    await waitFor(() => expect(within(p).getByText("offline")).toBeTruthy());
    expect(p.getAttribute("data-view")).toBe("unavailable");
    expect(p.textContent).toMatch(/saved digitally/);
    expect((within(p).getByRole("button", { name: /Test print/ }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("insufficient disk space and no frames are Setup required in their own sections", async () => {
    h.S.readiness = R([...OK.filter((i) => i.key !== "space" && i.key !== "frames"), item("space", "storage", "problem", { required: true, detail: "0.1 GB free — too little to save photos safely.", action: "Free some disk space." }),
      item("frames", "frames", "problem", { detail: "No frame is switched on.", action: "Switch at least one frame on under Frames." })], { status: "setup_required", headline: "Setup required" });
    await prepare();
    expect(card("Storage").getAttribute("data-view")).toBe("setup");
    expect(card("Storage").textContent).toMatch(/too little/);
    expect(card("Frames").getAttribute("data-view")).toBe("setup");
  });

  it("setup complete: every section is clear and shows what it uses", async () => {
    h.S.readiness = R(OK.map((i) => (i.verification ? { ...i, verification: "verified" as const } : i)));
    await prepare();
    for (const n of ["Camera", "Frames", "Storage"]) expect(card(n).getAttribute("data-view")).toBe("ready");
    expect(within(card("Frames")).getByText("Plain", { exact: false })).toBeTruthy();
    expect(within(card("Storage")).getByText("C:\\Docs\\apexvoid")).toBeTruthy();
  });

  it("opening Prepare Booth never sends a test print or captures anything", async () => {
    (h.S.settings as Record<string, unknown>).printerName = "Lab";
    await prepare();
    expect(h.api.testPrint).not.toHaveBeenCalled(); expect(h.api.startShooting).not.toHaveBeenCalled(); expect(h.api.startPreview).not.toHaveBeenCalled();
  });

  it("a test print needs an explicit confirmation", async () => {
    (h.S.settings as Record<string, unknown>).printerName = "Lab"; (h.S.settings as Record<string, unknown>).printPaper = "4x6 in";
    h.S.readiness = R(OK, { printing: "unverified" });
    h.api.testPrint.mockResolvedValue({ id: "J1" });
    await prepare();
    await waitFor(() => expect(within(card("Printer")).getByText("ready")).toBeTruthy());
    fireEvent.click(within(card("Printer")).getByRole("button", { name: /Test print/ }));
    expect(h.api.testPrint).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Print test page" }));
    await waitFor(() => expect(h.api.testPrint).toHaveBeenCalledWith("Lab", "4x6 in", "portrait"));
  });

  it("a refresh already running is not started twice", async () => {
    let release!: (v: unknown) => void;
    h.api.listPrinters.mockReset().mockReturnValue(new Promise((r) => { release = r; }));
    render(<PreparePage go={vi.fn()} onOpenBooth={vi.fn()} />);
    const btn = screen.getByRole("button", { name: /Checking/ }) as HTMLButtonElement;
    expect(btn.disabled).toBe(true);
    fireEvent.click(btn);
    expect(h.api.listPrinters).toHaveBeenCalledTimes(1);
    await act(async () => { release({ printers: [], driver: "f", default: "", note: "" }); });
    await waitFor(() => expect((screen.getByRole("button", { name: /Refresh hardware status/ }) as HTMLButtonElement).disabled).toBe(false));
  });

  it("refresh reads hardware lists without blocking the page", async () => {
    await prepare();
    h.api.listCameraSources.mockClear();
    fireEvent.click(screen.getByRole("button", { name: /Refresh hardware status/ }));
    expect((screen.getByRole("button", { name: /Checking/ }) as HTMLButtonElement).disabled).toBe(true);
    await waitFor(() => expect(h.api.listCameraSources).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("heading", { name: "Ready for guests" })).toBeTruthy(); // still interactive
  });
});

describe("Open Booth", () => {
  const Harness = ({ launch, go = vi.fn() }: { launch: () => void; go?: (p: string) => void }) => { const b = useOpenBooth(launch, go); return <>{<button onClick={b.open}>open</button>}{b.dialog}</>; };

  it("re-reads readiness, then launches exactly once even on repeated clicks", async () => {
    const launch = vi.fn();
    let release!: (v: BoothReadiness) => void;
    h.api.boothReadiness.mockReturnValue(new Promise((r) => { release = r; }));
    render(<Harness launch={launch} />);
    fireEvent.click(screen.getByText("open")); fireEvent.click(screen.getByText("open")); fireEvent.click(screen.getByText("open"));
    await act(async () => { release(R(OK)); });
    await waitFor(() => expect(launch).toHaveBeenCalledTimes(1));
    expect(h.api.boothReadiness).toHaveBeenCalledTimes(1);
  });

  it("lists the failing required items with where to fix them, and launches nothing", async () => {
    const launch = vi.fn(), go = vi.fn();
    h.api.boothReadiness.mockResolvedValue(R([item("camera", "camera", "problem", { detail: "Your preferred photo camera “Logitech C920” is not connected.", action: "Connect it." }), ...OK.slice(2)]));
    render(<Harness launch={launch} go={go} />);
    fireEvent.click(screen.getByText("open"));
    const dlg = await screen.findByRole("dialog");
    expect(within(dlg).getByText(/preferred photo camera/)).toBeTruthy();
    expect(launch).not.toHaveBeenCalled();
    fireEvent.click(within(dlg).getByRole("button", { name: "Camera setup" }));
    expect(go).toHaveBeenCalledWith("camera");
    expect(h.api.connectCamera).not.toHaveBeenCalled(); // never silently picks another camera
  });

  it("an optional printer problem does not stop the launch", async () => {
    const launch = vi.fn();
    h.api.boothReadiness.mockResolvedValue(R([...OK, item("printer", "printer", "warn")]));
    render(<Harness launch={launch} />);
    fireEvent.click(screen.getByText("open"));
    await waitFor(() => expect(launch).toHaveBeenCalledTimes(1));
  });

  it("a failed readiness read is reported, not ignored", async () => {
    h.api.boothReadiness.mockRejectedValue(new Error("backend busy"));
    render(<Harness launch={vi.fn()} />);
    fireEvent.click(screen.getByText("open"));
    expect((await screen.findByRole("dialog")).textContent).toMatch(/backend busy/);
  });
});

describe("settings from several pages", () => {
  it("quick settings send only the key that changed (PatchSettings merges on the backend)", async () => {
    h.api.patchSettings.mockResolvedValue({ restartRequired: false, settings: { ...(h.S.settings as object), countdownSeconds: 4 } });
    render(<QuickSettings />);
    fireEvent.click(screen.getByRole("button", { name: "Longer countdown" }));
    await waitFor(() => expect(h.api.patchSettings).toHaveBeenCalledWith({ countdownSeconds: 4 }));
    fireEvent.click(screen.getByRole("checkbox"));
    await waitFor(() => expect(h.api.patchSettings).toHaveBeenLastCalledWith({ previewMirror: true }));
    fireEvent.change(screen.getByLabelText("Output DPI"), { target: { value: "600" } });
    await waitFor(() => expect(h.api.patchSettings).toHaveBeenLastCalledWith({ outputDpi: 600 }));
    for (const c of h.api.patchSettings.mock.calls) expect(Object.keys(c[0])).toHaveLength(1);
  });
  it("the printer page changes only the printer key, and Digital only clears just that", async () => {
    (h.S.settings as Record<string, unknown>).printerName = "Lab";
    h.api.patchSettings.mockResolvedValue({ restartRequired: false });
    render(<PrinterPage />);
    await screen.findByText("Printing mode");
    fireEvent.click(screen.getByRole("radio", { name: /Digital only/ }));
    await waitFor(() => expect(h.api.patchSettings).toHaveBeenCalledWith({ printerName: "" }));
  });
});

describe("printer page", () => {
  it("shows Digital only without any printer, and never claims the Windows default is in use", async () => {
    render(<PrinterPage />);
    await screen.findByText("Printing mode");
    expect(screen.getByRole("radio", { name: /Digital only/ }).getAttribute("aria-checked")).toBe("true");
    expect(screen.getByText(/none — digital only/)).toBeTruthy();
    expect(screen.queryByText("used by the booth")).toBeNull();
    expect(h.api.testPrint).not.toHaveBeenCalled();
  });
  it("a failed test print is reported with the printer's reason", async () => {
    (h.S.settings as Record<string, unknown>).printerName = "Lab"; (h.S.settings as Record<string, unknown>).printPaper = "4x6 in";
    h.api.printerCapabilities.mockResolvedValue({ papers: [{ id: 1, name: "4x6 in", widthIn: 4, heightIn: 6 }], orientations: [], maxCopies: 1, color: true, resolutions: null, customPaper: false });
    h.api.testPrint.mockRejectedValue(Object.assign(new Error("The printer rejected the job."), { action: "Check the printer, then try again." }));
    render(<PrinterPage />);
    await screen.findByText("Printing mode");
    await waitFor(() => expect((screen.getAllByRole("button", { name: /Test print/ })[0] as HTMLButtonElement).disabled).toBe(false));
    fireEvent.click(screen.getAllByRole("button", { name: /Test print/ })[0]);
    fireEvent.click(screen.getByRole("button", { name: "Print test page" }));
    expect((await screen.findByRole("alert")).textContent).toMatch(/rejected the job/);
  });
});

describe("camera page", () => {
  it("camera changes are locked while a photo session is active", async () => {
    h.S.shoot = { state: "COUNTDOWN", taken: [] };
    h.api.listCameraSources.mockResolvedValue({ devices: [web, { ...web, id: "uvc:b", name: "Other" }], activeId: "uvc:a", photoPreferred: {}, videoPreferred: {}, photoMatchId: "", videoMatchId: "", restoreNote: "", ffmpegFound: true });
    render(<CameraPage />);
    await screen.findAllByText("Other");
    expect(screen.getByText(/camera cannot be changed right now/)).toBeTruthy();
    for (const b of screen.getAllByRole("button", { name: /Connect|Disconnect|Use as the Photo camera/ })) expect((b as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: /Run camera test/ }) as HTMLButtonElement).disabled).toBe(true);
    expect(h.api.connectCamera).not.toHaveBeenCalled();
  });
  it("keeps Photo and Video preferences separate and puts drivers under Advanced", async () => {
    render(<CameraPage />);
    await screen.findAllByText("Logitech C920");
    const adv = document.querySelector("details.advanced")!;
    expect(adv.textContent).toMatch(/Video camera/); expect(adv.textContent).toMatch(/Camera drivers/); expect(adv.textContent).toMatch(/Canon SDK not found/);
    expect(adv.hasAttribute("open")).toBe(false);
  });
  it("the simulated camera is labelled Demo / test mode on the Camera page", async () => {
    h.S.camera = { ...(h.S.camera as object), device: sim };
    render(<CameraPage />);
    expect(await screen.findByText(/Demo \/ test mode\./)).toBeTruthy();
  });
});

describe("device refresh", () => {
  it("concurrent refreshes: a slow older scan never overwrites a newer one", async () => {
    const list = (name: string) => ({ devices: [{ ...web, id: name, name }], activeId: "", photoPreferred: {}, videoPreferred: {}, photoMatchId: "", videoMatchId: "", restoreNote: "", ffmpegFound: true });
    let late!: (v: unknown) => void;
    h.api.listCameraSources.mockReset();
    h.api.listCameraSources.mockReturnValueOnce(Promise.resolve(list("First camera")));       // mount
    const { rerender } = render(<CameraPage />);
    await screen.findAllByText("First camera");
    h.api.listCameraSources.mockReturnValueOnce(new Promise((r) => { late = r; }));            // a camera event: slow
    h.api.listCameraSources.mockResolvedValueOnce(list("Newest camera"));                      // the next event: fast
    h.S.camera = { ...(h.S.camera as object), device: { ...web, id: "x1" } }; rerender(<CameraPage />);
    h.S.camera = { ...(h.S.camera as object), device: { ...web, id: "x2" } }; rerender(<CameraPage />);
    await screen.findAllByText("Newest camera");
    await act(async () => { late(list("Stale camera")); });
    expect(screen.queryByText("Stale camera")).toBeNull();
    expect(screen.getAllByText("Newest camera").length).toBeGreaterThan(0);
  });
});

describe("camera test", () => {
  const frame = { id: "single", name: "Single", photos: 1 };
  beforeEach(() => { h.api.listFrames.mockResolvedValue({ frames: [frame], categories: [], errors: [], dir: "", default: "single" }); });
  const states = (...seq: Array<Record<string, unknown>>) => { for (const s of seq) h.api.shootState.mockResolvedValueOnce(s); h.api.shootState.mockResolvedValue(seq[seq.length - 1]); };

  it("tells the operator the shutter will fire, and does nothing until they agree", async () => {
    render(<CameraTest />);
    fireEvent.click(screen.getByRole("button", { name: /Run camera test/ }));
    const dlg = await screen.findByRole("dialog");
    expect(dlg.textContent).toMatch(/will fire its shutter/);
    expect(dlg.textContent).toMatch(/Operator camera test/);
    expect(h.api.startShooting).not.toHaveBeenCalled(); expect(h.api.startPreview).not.toHaveBeenCalled();
    fireEvent.click(within(dlg).getByRole("button", { name: "Cancel" }));
    expect(h.api.startShooting).not.toHaveBeenCalled();
  });

  it("uses the normal session lifecycle, labels the session as a test and keeps it", async () => {
    states({ state: "IDLE", sessionId: "", taken: [] }, { state: "PREVIEW", sessionId: "", taken: [] }, { state: "REVIEW", sessionId: "S9", taken: [{ slot: 0, mediaId: "m0" }] }, { state: "COMPLETED", sessionId: "S9", taken: [{ slot: 0, mediaId: "m0" }], resultId: "R9" });
    render(<CameraTest />);
    fireEvent.click(screen.getByRole("button", { name: /Run camera test/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Take test photo" }));
    expect(await screen.findByText("Test photo saved", {}, { timeout: 5000 })).toBeTruthy();
    expect(h.api.startShootingWithCount).toHaveBeenCalledWith("single", "", "", 1);
    expect(h.api.labelTestSession).toHaveBeenCalledWith("S9");
    expect(h.api.accept).toHaveBeenCalled();
    expect(h.api.recoverShooting).toHaveBeenCalled(); // the one safe way back to idle (sessionGate)
    expect(screen.getByAltText("Camera test photo").getAttribute("src")).toBe("/m/R9");
    expect(screen.getByRole("status").textContent).toMatch(/frame grabs/);
  });

  it("a simulated camera is reported as a demo, not as a hardware check", async () => {
    h.S.camera = { ...(h.S.camera as object), device: sim };
    states({ state: "PREVIEW", sessionId: "", taken: [] }, { state: "PREVIEW", sessionId: "", taken: [] }, { state: "REVIEW", sessionId: "S1", taken: [] }, { state: "COMPLETED", sessionId: "S1", taken: [{ slot: 0, mediaId: "m" }], resultId: "R" });
    render(<CameraTest />);
    fireEvent.click(screen.getByRole("button", { name: /Run camera test/ }));
    expect((await screen.findByRole("dialog")).textContent).toMatch(/does not test real hardware/);
    fireEvent.click(screen.getByRole("button", { name: "Run demo test" }));
    const done = await screen.findByText("Demo test finished", {}, { timeout: 5000 });
    expect(done.parentElement?.textContent).toMatch(/does not verify real hardware/);
  });

  it("a failed test explains what to do, offers a retry and still resets safely", async () => {
    states({ state: "PREVIEW", sessionId: "", taken: [] }, { state: "ERROR", sessionId: "S2", taken: [], error: "The camera did not answer.", errorAction: "Switch the camera on and try again." });
    render(<CameraTest />);
    fireEvent.click(screen.getByRole("button", { name: /Run camera test/ }));
    fireEvent.click(await screen.findByRole("button", { name: "Take test photo" }));
    const alert = await screen.findByRole("alert", {}, { timeout: 5000 });
    expect(alert.textContent).toMatch(/The camera test failed/); expect(alert.textContent).toMatch(/did not answer/); expect(alert.textContent).toMatch(/Switch the camera on/);
    expect(within(alert).getByRole("button", { name: "Try again" })).toBeTruthy();
    expect(h.api.recoverShooting).toHaveBeenCalled();
    expect(h.api.accept).not.toHaveBeenCalled();
  });

  it("is unavailable without a camera or while reconnecting, and says why", () => {
    h.S.camera = { device: undefined, state: "disconnected", reconnecting: false, liveView: false, drivers: [] };
    render(<CameraTest />);
    expect((screen.getByRole("button", { name: /Run camera test/ }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("Connect a camera first.")).toBeTruthy();
  });
});

describe("diagnostics page", () => {
  const diag = (checks: unknown[]) => ({ platform: "windows/amd64", checks, drivers: [], conflicts: [], library: {}, logDir: "<logs>", network: "none", movieFindings: ["unverified"] });
  it("separates what stops guests from what does not apply to this set-up", async () => {
    h.api.diagnostics.mockResolvedValue(diag([
      { name: "Canon SDK files", ok: true, state: "info", detail: "Not installed — only needed for Canon cameras. Webcams work without it.", scope: "canon", blocksGuests: false },
      { name: "Free disk space", ok: false, state: "problem", detail: "0.1 GB free", action: "Free some disk space.", scope: "storage", blocksGuests: true },
      { name: "Media folder writable", ok: true, state: "ok", detail: "<media>", scope: "storage", blocksGuests: false },
    ]));
    render(<DiagnosticsPage />);
    await screen.findByText("Free disk space");
    expect(screen.getByText("Stops guests")).toBeTruthy();
    expect(screen.getByText("Not needed")).toBeTruthy();
    expect(screen.getByRole("status").textContent).toMatch(/1 problem stops guests/);
    const names = Array.from(document.querySelectorAll(".diag strong")).map((n) => n.textContent);
    expect(names[0]).toBe("Free disk space"); expect(names[names.length - 1]).toBe("Canon SDK files");
    expect(screen.getByText(/safe to run the check again/)).toBeTruthy();
  });
  it("a Canon SDK warning alone does not read as 'guests cannot take photos'", async () => {
    h.api.diagnostics.mockResolvedValue(diag([{ name: "Canon SDK files", ok: true, state: "info", detail: "Not installed", scope: "canon", blocksGuests: false }]));
    render(<DiagnosticsPage />);
    expect(await screen.findByText(/Nothing here stops guests/)).toBeTruthy();
  });
  it("exports a support bundle only after the operator chooses a folder", async () => {
    h.api.diagnostics.mockResolvedValue(diag([]));
    h.api.pickFolder.mockResolvedValue("/chosen");
    h.api.exportSupportBundle.mockResolvedValue("/chosen/support.zip");
    render(<DiagnosticsPage />);
    fireEvent.click(await screen.findByRole("button", { name: /Export support bundle/ }));
    await waitFor(() => expect(h.api.exportSupportBundle).toHaveBeenCalledWith("/chosen"));
  });
  it("copies the sanitized support summary from the backend, unchanged", async () => {
    h.api.diagnostics.mockResolvedValue(diag([]));
    h.api.diagnosticSummary.mockResolvedValue("ApexVoid Photobooth — support summary\nPrinter configured: true");
    const write = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText: write }, configurable: true });
    render(<DiagnosticsPage />);
    fireEvent.click(await screen.findByRole("button", { name: /Copy support summary/ }));
    await waitFor(() => expect(write).toHaveBeenCalledWith("ApexVoid Photobooth — support summary\nPrinter configured: true"));
    expect((screen.getByLabelText("Support summary") as HTMLTextAreaElement).value).toMatch(/support summary/);
  });
});

describe("latest-wins helper", () => {
  it("only the newest id is current", () => {
    let l!: ReturnType<typeof useLatest>;
    const T = () => { l = useLatest(); return null; };
    render(<T />);
    const a = l.next(), b = l.next();
    expect(l.isCurrent(a)).toBe(false); expect(l.isCurrent(b)).toBe(true);
  });
});
