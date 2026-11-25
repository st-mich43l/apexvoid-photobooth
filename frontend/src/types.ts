// Mirrors the JSON of the Go structs in internal/*. Keep in sync by hand
// (the bound API is small; generated bindings are not used).

export type Support = "supported" | "unsupported" | "unverified";
export type CamState = "disconnected" | "connected" | "liveview" | "busy" | "error";

export interface Mode { width: number; height: number; fpsMin: number; fpsMax: number; format: string }
export type SourceKind = "dslr" | "dslr-uvc" | "webcam" | "capture-card" | "virtual" | "simulated";
export interface DeviceInfo {
  id: string; name: string; driver: string; port?: string; simulated: boolean; kind: SourceKind; vendor?: string; usbId?: string;
  modes?: Mode[]; duplicateOf?: string; note?: string;
}
export interface Preferred { id: string; name: string; driver: string; usbId: string }
export interface CameraSources {
  devices: DeviceInfo[]; activeId: string; photoPreferred: Preferred; videoPreferred: Preferred;
  photoMatchId: string; videoMatchId: string; restoreNote: string; ffmpegFound: boolean;
}
export type CameraMode = "photo" | "video";
export interface ModeReadiness {
  mode: CameraMode; ready: boolean; switched: boolean; needsFallback: boolean; candidate?: DeviceInfo; preferred: Preferred; message: string;
}
export interface SavedMode { width: number; height: number; fps: number; format: string }
export interface Capabilities {
  liveView: Support; stillCapture: Support; originalTransfer: Support;
  movieRecord: Support; movieTransfer: Support; streamedVideo: Support; notes?: string[];
}
export interface DriverStatus { name: string; available: boolean; problem?: string }
export interface CameraStatus {
  device?: DeviceInfo; state: CamState; capabilities?: Capabilities; liveView: boolean;
  mode?: Mode; reconnecting: boolean; lastError?: string; lastAction?: string; lastCode?: string; drivers: DriverStatus[]; version?: number;
}

export type ShootStateName = "IDLE" | "PREVIEW" | "COUNTDOWN" | "CAPTURING" | "REVIEW" | "SAVING" | "COMPLETED" | "ERROR" | "CANCELLED";
export interface Shot { slot: number; mediaId: string; simulated: boolean }
export interface ShootSnapshot {
  state: ShootStateName; version: number; sessionId: string; shots: number; slot: number; remaining: number;
  taken: Shot[]; error?: string; errorAction?: string; warning?: string; warningCode?: string; simulated: boolean; resultId?: string;
}

export interface VideoState {
  state: "idle" | "recording" | "saving" | "error"; sessionId: string; elapsedMs: number; maxSeconds: number;
  lastMediaId: string; error?: string; action?: string; notice?: string;
}

export interface VideoSettings { device: string; audioInput: string; width: number; height: number; fps: number; maxSeconds: number }
export interface Settings {
  language?: "en" | "vi";
  guestLanguage?: "en" | "vi";
  mediaDir: string; countdownSeconds: number; shots: number; templateId: string; outputDpi: number; previewMirror: boolean; autoReconnect: boolean;
  enableMock: boolean; ffmpegPath: string; canonSdkDir: string; printerName: string; printPaper: string; printOrientation?: string; printFit?: string; printCopies?: number; printTwoUp?: string; video: VideoSettings;
  kioskPinSet: boolean; // the PIN record itself never reaches the UI
  photoCamera: Preferred; videoCamera: Preferred; cameraModes: Record<string, SavedMode> | null;
}

export interface LibrarySummary {
  sessions: number; photos: number; videos: number; interrupted: string[]; recovered: number; missing: number; freeBytes: number;
}
export interface LibraryStatus {
  path: string; source: "documents" | "legacy" | "custom" | "fallback" | "default"; defaultPath: string; legacyPath: string;
  migrationPending: boolean; restartRequired: boolean; warning: string; configPath: string; freeBytes: number;
}
export interface MigrationReport {
  dest: string; source: string; files: number; skipped: number; symlinks: string[]; bytes: number; sessions: number;
  mediaRows: number; verified: boolean; sourceKept: boolean; restartRequired: boolean;
}
export interface BootInfo {
  version: string; dataDir: string; mediaBase: string; mediaToken: string; settings: Settings; camera: CameraStatus;
  capture: ShootSnapshot; video: VideoState; library: LibrarySummary; location: LibraryStatus; os: string;
}

export interface Session {
  id: string; createdAt: string; updatedAt: string; status: string; mode: "photo" | "video"; shotsPlanned: number;
  templateId: string; simulated: boolean; note: string; mediaCount: number;
  frameId: string; frameName: string; variantId: string; frameRev: number; caption: string; frameDir: string;
}
export interface SessionCard extends Session { thumbnailId: string; thumbnailVersion: string; photoCount: number; hasStrip: boolean }
export interface SessionQuery { search: string; mode: string; status: string; simulated: string; from: string; to: string; cursor: string; limit: number }
export interface SessionPage { items: SessionCard[]; nextCursor: string; hasMore: boolean }
export interface SessionMediaPage { items: Media[]; nextOffset: number; hasMore: boolean }
export interface Media {
  id: string; sessionId: string; kind: "photo" | "video" | "strip" | "print"; slot: number; relPath: string; thumbRel: string;
  previewRel: string; editedRel: string; edits: string; width: number; height: number; sizeBytes: number; sha256: string;
  durationMs: number; simulated: boolean; source: string; status: string; capturedAt: string;
  frameId: string; frameName: string; variantId: string; frameRev: number; frameDir: string;
}

export interface Rect { x: number; y: number; w: number; h: number }
export interface Edits { rotate: number; crop: Rect | null; brightness: number; contrast: number; saturation: number }

export interface VDevice { name: string; id: string; kind: string; modes?: Mode[] }
export interface VDevices { video: VDevice[]; audio: VDevice[] }

export interface Slot { x: number; y: number; w: number; h: number; photo: number; focusX?: number; focusY?: number; locked?: boolean }
export interface Template {
  id: string; name: string; kind: string; widthIn: number; heightIn: number; dpi: number; background: string;
  borderIn: number; borderColor: string; overlay: string; slots: Slot[];
}
export interface Variant { id: string; name: string; background?: string; borderColor?: string; accent?: string; textColor?: string }
export type LayerKind = "text" | "rect" | "ellipse" | "shape" | "pattern" | "image";
export interface Layer {
  id: string; name?: string; kind: LayerKind; x: number; y: number; w: number; h: number; color?: string; opacity?: number; under?: boolean; hidden?: boolean; locked?: boolean;
  strokeIn?: number; radiusIn?: number; shape?: string; elemH?: number; size?: number; gap?: number; spread?: boolean; colors?: string[];
  text?: string; font?: string; sizeIn?: number; align?: string; asset?: string; fit?: string;
}
/** A frame definition: what the Go renderer draws and the Frame Builder edits (all lengths in inches). */
export interface FrameDef {
  id: string; name: string; kind: string; category: string; description?: string; order: number; revision: number;
  enabled: boolean; builtin?: boolean; legacy?: boolean; editable?: boolean; captionEnabled?: boolean; design?: string; layoutId?: string;
  widthIn: number; heightIn: number; dpi: number; slots: Slot[];
  background: string; borderIn?: number; borderColor?: string; cornerIn?: number; accent?: string; textColor?: string;
  backgroundImage?: string; overlay?: string; layers?: Layer[]; variants?: Variant[];
}
/** A frame as listed to the UI: the definition plus derived facts. */
export interface Frame extends FrameDef { photos: number; format: string; pixelW: number; pixelH: number; favorite?: boolean }
export interface PNGFrameImport { draft: FrameDef; asset: AssetInfo; windows: number; mode: "windows" | "background"; notes: string[] }
export interface AssetInfo { name: string; format: string; width: number; height: number; hasAlpha: boolean; bytes: number }
export interface FramesResult { frames: Frame[]; categories: string[]; errors: string[]; dir: string; default: string }
/** What the guest picked: stays with the session from the first shot to the printed result. */
export interface FrameChoice {
  id: string; name: string; builtin?: boolean; variantId: string; caption: string; photos: number; format: string;
  /** Slot geometry used to frame each guest capture like the saved composition. */
  photoSlots?: Array<{ photo: number; w: number; h: number; focusX?: number; focusY?: number }>;
}
export interface TemplatesResult { templates: Template[]; errors: string[]; dir: string }

export interface Printer { name: string; default: boolean; status: "ready" | "offline" | "error" | "unknown"; detail?: string; port?: string; driver?: string }
export interface Paper { id: number; name: string; widthIn: number; heightIn: number }
export interface PrinterCaps { papers: Paper[]; orientations: string[]; maxCopies: number; color: boolean; resolutions: Array<[number, number]> | null; customPaper: boolean; note?: string }
export interface PrintersResult { printers: Printer[]; driver: string; default: string; note: string }
export type JobStatus = "submitting" | "queued" | "printing" | "completed" | "removed" | "failed" | "unknown";
export interface PrintJob {
  id: string; requestId: string; parentId: string; kind: "print" | "test"; mediaId: string; renderedMediaId: string; sessionId: string; templateId: string;
  file: string; printer: string; paperName: string; paperId: number; orientation: string; fit: string; copies: number; status: JobStatus; detail: string;
  nativeId: string; attempt: number; createdAt: string; updatedAt: string;
}
export interface PrintOptions { requestId: string; mediaId: string; templateId: string; printer: string; paper: string; orientation: string; fit: string; copies: number; twoUp: boolean }
export interface PrintPreview {
  image: string; paperMm: [number, number]; printableMm: [number, number]; marginMm: [number, number, number, number]; unprintableMm?: [number, number, number, number]; scalePct: number; croppedPct: number;
  rotated: boolean; orientation: string; warning: string; assumedGeometry: boolean; imageInches: [number, number]; templateId: string; twoUpOffer: boolean; twoUp: boolean;
}

export type CheckState = "ok" | "problem" | "warn" | "info";
export interface Check { code?: string; name: string; ok: boolean; detail: string; action?: string; state: CheckState; blocksGuests: boolean; scope: string }
export interface Diagnostics {
  platform: string; checks: Check[]; drivers: DriverStatus[]; conflicts: string[]; library: LibrarySummary;
  logDir: string; network: string; movieFindings: string[];
}
export interface Report { checked: number; missing: string[]; recovered: string[]; interrupted: string[]; corrupt: string[] }

// ---- Canon camera setup (internal/app/canon_api.go) ----
export type ReadyState = "ok" | "problem" | "not_tried";
export interface Readiness { key: string; label: string; state: ReadyState; detail?: string; action?: string }
export interface CanonStatus {
  platform: string;
  sdk: { kind: string; problem?: string; action?: string; path?: string; machine?: string; missingImports?: string[]; missingExports?: string[] };
  driver?: { running: boolean; busy: boolean; op?: string; runningMs: number; stuck: boolean; abandoned: number; completed: number };
  detected: DeviceInfo[];
  conflicts: string[];
  active?: { name: string; interface: string; port?: string; usbId?: string; state: CamState; liveView: boolean; capabilities: Capabilities;
    counters?: { captures: number; lateTransfers: number; invalidTransfers: number; lastFile?: string } };
  items: Readiness[];
  recommendation?: string;
  guide: string[]; guideCodes?: string[];
  verification: string;
}

// ---- Booth readiness (internal/app/readiness_api.go) ----
export type ReadinessStatus = "ready" | "setup_required" | "reconnecting" | "attention" | "demo";
export type ReadinessGroup = "camera" | "storage" | "frames" | "printer" | "session";
export type Verification = "verified" | "unverified" | "demo";
export interface ReadinessItem { key: string; label: string; required: boolean; state: "ok" | "problem" | "warn" | "info"; detail?: string; action?: string; group?: ReadinessGroup; verification?: Verification }
export interface BoothReadiness {
  status: ReadinessStatus; canStart: boolean; headline: string; detail?: string; guest: string; items: ReadinessItem[]; at: string;
  /** a SIMULATED camera is the photo camera: usable, but not hardware verification */
  demo?: boolean;
  /** what guests are offered: digital_only | unverified (Windows reports the printer ready) | unavailable */
  printing?: "digital_only" | "unverified" | "unavailable";
}
