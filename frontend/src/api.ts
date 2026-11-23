import { Call, Events } from "@wailsio/runtime";
import type {
  BootInfo, CameraMode, CameraSources, CameraStatus, ModeReadiness, SavedMode, LibraryStatus, MigrationReport, DeviceInfo, Diagnostics, Edits, LibrarySummary, Media, PrintJob, PrintersResult, Report,
  Session, SessionPage, SessionQuery, SessionMediaPage, Settings, ShootSnapshot, CanonStatus, BoothReadiness, FramesResult, FrameDef, Frame, AssetInfo, PNGFrameImport, VDevices, VideoState, PrinterCaps, PrintOptions, PrintPreview,
} from "./types";

// Wails addresses a bound method by "<go package path>.<type>.<method>".
const SVC = "github.com/st-mich43l/apexvoid-photobooth/internal/app.Service";
const call = <T>(method: string, ...args: unknown[]): Promise<T> =>
  Call.ByName(`${SVC}.${method}`, ...args) as unknown as Promise<T>;

/** An operator-facing error: what happened + what to do about it. */
export class AppError extends Error {
  action: string;
  constructor(message: string, action = "") { super(message); this.action = action; }
}

/** Go sends "message\naction" (see app.UserError). */
export function toAppError(e: unknown): AppError {
  if (e instanceof AppError) return e;
  const raw = e instanceof Error ? e.message : String(e);
  const [msg, ...rest] = raw.split("\n");
  return new AppError(msg || "Unexpected error", rest.join(" ").trim());
}

async function wrap<T>(p: Promise<T>): Promise<T> {
  try { return await p; } catch (e) { throw toAppError(e); }
}

export const api = {
  bootstrap: () => wrap(call<BootInfo>("Bootstrap")),
  listCameras: () => wrap(call<DeviceInfo[]>("ListCameras")),
  listCameraSources: () => wrap(call<CameraSources>("ListCameraSources")),
  canonStatus: () => wrap(call<CanonStatus>("CanonStatus")),
  setPreferredCamera: (mode: CameraMode, id: string) => wrap(call<void>("SetPreferredCamera", mode, id)),
  clearPreferredCamera: (mode: CameraMode) => wrap(call<void>("ClearPreferredCamera", mode)),
  setCameraMode: (id: string, m: SavedMode) => wrap(call<void>("SetCameraMode", id, m)),
  prepareMode: (mode: CameraMode, acceptId = "") => wrap(call<ModeReadiness>("PrepareMode", mode, acceptId)),
  cameraStatus: () => wrap(call<CameraStatus>("CameraStatus")),
  connectCamera: (id: string) => wrap(call<void>("ConnectCamera", id)),
  disconnectCamera: () => wrap(call<void>("DisconnectCamera")),
  startLiveView: () => wrap(call<void>("StartLiveView")),
  stopLiveView: () => wrap(call<void>("StopLiveView")),

  shootState: () => wrap(call<ShootSnapshot>("ShootState")),
  startPreview: () => wrap(call<void>("StartPreview")),
  startShooting: (frameId: string, variantId: string, caption: string) => wrap(call<void>("StartShooting", frameId, variantId, caption)),
  setSessionFrame: (sessionId: string, frameId: string, variantId: string, caption: string, useFirst: boolean) => wrap(call<void>("SetSessionFrame", sessionId, frameId, variantId, caption, useFirst)),
  retake: (slot: number) => wrap(call<void>("RetakePhoto", slot)),
  accept: () => wrap(call<void>("AcceptPhotos")),
  cancelShooting: () => wrap(call<void>("CancelShooting")),
  resetShooting: () => wrap(call<void>("ResetShooting")),

  videoDevices: () => wrap(call<VDevices>("VideoDevices")),
  videoState: () => wrap(call<VideoState>("VideoState")),
  startRecording: () => wrap(call<void>("StartRecording")),
  stopRecording: () => wrap(call<void>("StopRecording")),

  listSessions: (limit = 200, offset = 0) => wrap(call<Session[]>("ListSessions", limit, offset)),
  querySessions: (q: SessionQuery) => wrap(call<SessionPage>("QuerySessions", q)),
  querySessionMedia: (id: string, limit = 50, offset = 0) => wrap(call<SessionMediaPage>("QuerySessionMedia", id, limit, offset)),
  restoreSession: (id: string) => wrap(call<void>("RestoreSession", id)),
  listSessionPrintJobs: (id: string, limit = 50, offset = 0) => wrap(call<PrintJob[]>("ListSessionPrintJobs", id, limit, offset)),
  exportSession: (id: string, dest: string, edited: boolean) => wrap(call<string>("ExportSession", id, dest, edited)),
  listMedia: (sessionId = "", limit = 500, offset = 0) => wrap(call<Media[]>("ListMedia", sessionId, limit, offset)),
  getMedia: (id: string) => wrap(call<Media>("GetMedia", id)),
  saveEdits: (id: string, e: Edits) => wrap(call<Media>("SaveEdits", id, e)),
  exportMedia: (id: string, dest: string, edited: boolean) => wrap(call<string>("ExportMedia", id, dest, edited)),
  trashMedia: (id: string) => wrap(call<void>("TrashMedia", id)),
  restoreMedia: (id: string) => wrap(call<void>("RestoreMedia", id)),
  trashSession: (id: string) => wrap(call<void>("TrashSession", id)),
  emptyTrash: () => wrap(call<number>("EmptyTrash")),
  verifyLibrary: (deep: boolean) => wrap(call<Report>("VerifyLibrary", deep)),
  revealMedia: (id: string) => wrap(call<void>("RevealMedia", id)),
  pickFolder: () => wrap(call<string>("PickFolder")),
  libraryStatus: () => wrap(call<LibraryStatus>("LibraryStatus")),
  migrateLibrary: () => wrap(call<MigrationReport>("MigrateLibrary")),
  keepLegacyLibrary: () => wrap(call<void>("KeepLegacyLibrary")),
  openLibraryFolder: () => wrap(call<void>("OpenLibraryFolder")),
  setKioskPin: (current: string, next: string) => wrap(call<void>("SetKioskPin", current, next)),
  verifyKioskPin: (pin: string) => wrap(call<void>("VerifyKioskPin", pin)),
  librarySummary: () => wrap(call<LibrarySummary>("LibrarySummary")),

  listFrames: (includeDisabled = false) => wrap(call<FramesResult>("ListFrames", includeDisabled)),
  setFrameEnabled: (id: string, enabled: boolean) => wrap(call<void>("SetFrameEnabled", id, enabled)),
  setFrameCatalogue: (order: string[], favorites: Record<string, boolean>) => wrap(call<void>("SetFrameCatalogue", order, favorites)),
  previewFrame: (id: string, variantId: string, caption: string, width: number) => wrap(call<string>("PreviewFrame", id, variantId, caption, width)),
  getFrame: (id: string) => wrap(call<FrameDef>("GetFrame", id)),
  newFrameDraft: (sourceId: string, name = "") => wrap(call<FrameDef>("NewFrameDraft", sourceId, name)),
  importFrameImage: (dataUrl: string) => wrap(call<AssetInfo>("ImportFrameImage", dataUrl)),
  importPngFrame: (dataUrl: string, name: string) => wrap(call<PNGFrameImport>("ImportPNGFrame", dataUrl, name)),
  previewDraft: (f: FrameDef, variantId: string, caption: string, width: number) => wrap(call<string>("PreviewDraft", f, variantId, caption, width)),
  saveFrame: (f: FrameDef) => wrap(call<Frame>("SaveFrame", f)),
  deleteFrame: (id: string) => wrap(call<void>("DeleteFrame", id)),
  composeSession: (sessionId: string, frameId: string, variantId: string, caption: string, useFirst: boolean) => wrap(call<Media>("ComposeSession", sessionId, frameId, variantId, caption, useFirst)),

  listPrinters: () => wrap(call<PrintersResult>("ListPrinters")),
  printerCapabilities: (printer: string) => wrap(call<PrinterCaps>("PrinterCapabilities", printer)),
  printPreview: (o: PrintOptions) => wrap(call<PrintPreview>("PrintPreview", o)),
  printMedia: (o: PrintOptions) => wrap(call<PrintJob>("PrintMedia", o)),
  printResult: (mediaId: string, requestId: string) => wrap(call<PrintJob>("PrintResult", mediaId, requestId)),
  testPrint: (printer: string, paper: string, orientation: string) => wrap(call<PrintJob>("TestPrint", printer, paper, orientation)),
  printJobs: () => wrap(call<PrintJob[]>("PrintJobs")),
  reprint: (id: string, confirmed: boolean) => wrap(call<PrintJob>("Reprint", id, confirmed)),

  getSettings: () => wrap(call<Settings>("GetSettings")),
  /** Changes only the named settings (the backend merges them into its current copy). */
  patchSettings: (p: Partial<Settings>) => wrap(call<{ restartRequired: boolean; settings?: Settings }>("PatchSettings", p)),
  boothReadiness: () => wrap(call<BoothReadiness>("BoothReadiness")),
  recoverShooting: () => wrap(call<void>("RecoverShooting")),
  saveSettings: (s: Settings) => wrap(call<{ restartRequired: boolean }>("SaveSettings", s)),
  diagnostics: () => wrap(call<Diagnostics>("RunDiagnostics")),
  diagnosticSummary: () => wrap(call<string>("DiagnosticSummary")),
  labelTestSession: (sessionId: string) => wrap(call<void>("LabelTestSession", sessionId)),
};

/** Subscribe to a Go-emitted event; returns the unsubscribe function. */
export function on<T>(name: string, cb: (data: T) => void): () => void {
  return Events.On(name, (ev) => cb(ev.data as T));
}
