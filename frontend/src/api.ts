import { Call, Events } from "@wailsio/runtime";
import type {
  BootInfo, CameraStatus, LibraryStatus, MigrationReport, DeviceInfo, Diagnostics, Edits, LibrarySummary, Media, PrintJob, PrintersResult, Report,
  Session, Settings, ShootSnapshot, TemplatesResult, VDevices, VideoState,
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
  cameraStatus: () => wrap(call<CameraStatus>("CameraStatus")),
  connectCamera: (id: string) => wrap(call<void>("ConnectCamera", id)),
  disconnectCamera: () => wrap(call<void>("DisconnectCamera")),
  startLiveView: () => wrap(call<void>("StartLiveView")),
  stopLiveView: () => wrap(call<void>("StopLiveView")),

  shootState: () => wrap(call<ShootSnapshot>("ShootState")),
  startPreview: () => wrap(call<void>("StartPreview")),
  startShooting: (shots: number) => wrap(call<void>("StartShooting", shots)),
  retake: (slot: number) => wrap(call<void>("RetakePhoto", slot)),
  accept: () => wrap(call<void>("AcceptPhotos")),
  cancelShooting: () => wrap(call<void>("CancelShooting")),
  resetShooting: () => wrap(call<void>("ResetShooting")),

  videoDevices: () => wrap(call<VDevices>("VideoDevices")),
  videoState: () => wrap(call<VideoState>("VideoState")),
  startRecording: () => wrap(call<void>("StartRecording")),
  stopRecording: () => wrap(call<void>("StopRecording")),

  listSessions: (limit = 200, offset = 0) => wrap(call<Session[]>("ListSessions", limit, offset)),
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

  listTemplates: () => wrap(call<TemplatesResult>("ListTemplates")),
  previewTemplate: (id: string) => wrap(call<string>("PreviewTemplate", id)),
  composeSession: (sessionId: string, templateId: string) => wrap(call<Media>("ComposeSession", sessionId, templateId)),

  listPrinters: () => wrap(call<PrintersResult>("ListPrinters")),
  printMedia: (mediaId: string, printer: string, paper: string, copies: number, templateId: string) =>
    wrap(call<PrintJob>("PrintMedia", mediaId, printer, paper, copies, templateId)),
  printJobs: () => wrap(call<PrintJob[]>("PrintJobs")),
  reprint: (id: string) => wrap(call<PrintJob>("Reprint", id)),

  getSettings: () => wrap(call<Settings>("GetSettings")),
  saveSettings: (s: Settings) => wrap(call<{ restartRequired: boolean }>("SaveSettings", s)),
  diagnostics: () => wrap(call<Diagnostics>("RunDiagnostics")),
};

/** Subscribe to a Go-emitted event; returns the unsubscribe function. */
export function on<T>(name: string, cb: (data: T) => void): () => void {
  return Events.On(name, (ev) => cb(ev.data as T));
}
