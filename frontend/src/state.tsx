import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode, type Dispatch, type SetStateAction } from "react";
import { api, on, toAppError, AppError } from "./api";
import { useLocale } from "./locale";
import { formatTranslation, translate, type TranslationKey } from "./i18n";
import type { BoothReadiness, BootInfo, CameraStatus, LibraryStatus, Settings, ShootSnapshot, VideoState } from "./types";

export interface Ctx {
  boot: BootInfo;
  camera: CameraStatus;
  shoot: ShootSnapshot;
  video: VideoState;
  settings: Settings;
  location: LibraryStatus;
  /** Can a guest start a photo session? null until the first answer. Refreshed on every relevant change. */
  readiness: BoothReadiness | null;
  refreshReadiness: () => void;
  /** bumped whenever the library changes (new capture/recording/edit/delete) */
  libraryVersion: number;
  bumpLibrary: () => void;
  mediaUrl: (id: string, kind: "thumb" | "preview" | "basepreview" | "original" | "edited", v?: string) => string;
  /** A frame preview rendered by the Go renderer (sample photos), at the given pixel width. */
  frameUrl: (id: string, variantId: string, width: number, caption?: string) => string;
  previewUrl: string;
  setSettings: Dispatch<SetStateAction<Settings>>;
  toast: (msg: string, kind?: "ok" | "err", undo?: () => void) => void;
}

const C = createContext<Ctx | null>(null);
export const useApp = (): Ctx => {
  const c = useContext(C);
  if (!c) throw new Error("AppProvider missing");
  return c;
};

export function useT() {
  const { settings } = useApp();
  const language = useLocale(settings.language);
  return {
    t: (key: TranslationKey) => translate(language, key),
    tf: (key: TranslationKey, values: Record<string, string | number>) => formatTranslation(language, key, values),
  };
}

interface Toast { id: number; msg: string; kind: "ok" | "err"; undo?: () => void }

export function AppProvider({ children }: { children: ReactNode }) {
  const [boot, setBoot] = useState<BootInfo | null>(null);
  const [fatal, setFatal] = useState<AppError | null>(null);
  const [camera, setCamera] = useState<CameraStatus | null>(null);
  const [shoot, setShoot] = useState<ShootSnapshot | null>(null);
  const [video, setVideo] = useState<VideoState | null>(null);
  const [settings, setSettingsState] = useState<Settings | null>(null);
  const [location, setLocation] = useState<LibraryStatus | null>(null);
  const [libraryVersion, setLib] = useState(0);
  const [readiness, setReadiness] = useState<BoothReadiness | null>(null);
  const readyTimer = useRef<number | undefined>(undefined);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);
  const [previewNonce, setPreviewNonce] = useState(0);

  useEffect(() => {
    let alive = true;
    const load = (attempt: number) =>
      api.bootstrap().then((b) => {
        if (!alive) return;
        setBoot(b); setCamera(b.camera); setShoot(b.capture); setVideo(b.video); setSettingsState(b.settings); setLocation(b.location);
      }).catch((e) => {
        // the Wails bridge can need a moment at start-up
        if (attempt < 20 && alive) setTimeout(() => load(attempt + 1), 250); else setFatal(toAppError(e));
      });
    load(0);
    const offs = [
      // a snapshot that arrives after a newer one (events can be reordered) is dropped
      on<CameraStatus>("camera:status", (s) => { setCamera((cur) => (cur && s.version != null && cur.version != null && s.version < cur.version ? cur : s)); }),
      on<ShootSnapshot>("capture:state", (s) => {
        setShoot((cur) => (cur && s.version < cur.version ? cur : s));
        if (s.state === "REVIEW" || s.state === "COMPLETED") setLib((v) => v + 1);
      }),
      on<VideoState>("video:state", (s) => { setVideo(s); if (s.state === "idle") setLib((v) => v + 1); }),
      on<Settings>("settings:changed", (s) => setSettingsState(s)),
      on<LibraryStatus>("library:changed", (l) => setLocation(l)),
    ];
    return () => { alive = false; offs.forEach((f) => f()); };
  }, []);

  // Booth readiness: asked again whenever something it depends on changes, and now and then
  const refreshReadiness = useCallback(() => {
    window.clearTimeout(readyTimer.current);
    readyTimer.current = window.setTimeout(() => { api.boothReadiness().then(setReadiness).catch(() => {}); }, 150);
  }, []);
  useEffect(() => { refreshReadiness(); }, [refreshReadiness, camera?.version, camera?.device?.id, camera?.state, camera?.reconnecting, shoot?.state, settings, libraryVersion, location]);
  useEffect(() => { const t = window.setInterval(refreshReadiness, 10_000); return () => { window.clearInterval(t); window.clearTimeout(readyTimer.current); }; }, [refreshReadiness]);

  // a live-view <img> must be re-opened whenever the stream restarts
  const lv = camera?.liveView, st = camera?.state;
  useEffect(() => { setPreviewNonce((n) => n + 1); }, [lv, st, camera?.device?.id]);

  const toast = useCallback((msg: string, kind: "ok" | "err" = "ok", undo?: () => void) => {
    const id = nextId.current++;
    setToasts((t) => [...t, { id, msg, kind, undo }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), undo ? 8000 : 5000);
  }, []);

  // Keep media URLs stable when a preference changes; active previews and editors
  // should not reload just because the operator switched languages.
  const mediaBase = boot?.mediaBase, mediaToken = boot?.mediaToken;
  const mediaUrl = useCallback((id: string, kind: "thumb" | "preview" | "basepreview" | "original" | "edited", v?: string) =>
    `${mediaBase}/media/${id}/${kind}?t=${mediaToken}${v ? `&v=${encodeURIComponent(v)}` : ""}`, [mediaBase, mediaToken]);
  const frameUrl = useCallback((id: string, variantId: string, width: number, caption = "") =>
    `${mediaBase}/frame/${encodeURIComponent(id)}/${variantId ? encodeURIComponent(variantId) : "-"}?t=${mediaToken}&w=${width}${caption ? `&caption=${encodeURIComponent(caption)}` : ""}`, [mediaBase, mediaToken]);

  const value = useMemo<Ctx | null>(() => {
    if (!boot || !camera || !shoot || !video || !settings || !location) return null;
    const base = boot.mediaBase, tok = boot.mediaToken;
    return {
      boot, camera, shoot, video, settings, location, readiness, refreshReadiness, libraryVersion,
      bumpLibrary: () => setLib((v) => v + 1),
      mediaUrl, frameUrl,
      previewUrl: `${base}/preview.mjpg?t=${tok}&n=${previewNonce}`,
      setSettings: (next) => setSettingsState(cur => cur ? (typeof next === "function" ? next(cur) : next) : cur),
      toast,
    };
  }, [boot, camera, shoot, video, settings, location, readiness, refreshReadiness, libraryVersion, previewNonce, toast, mediaUrl, frameUrl]);

  if (fatal) {
    return (
      <div className="splash">
        <h1>ApexVoid Photobooth could not start</h1>
        <p>{fatal.message}</p>
        {fatal.action && <p className="hint">{fatal.action}</p>}
      </div>
    );
  }
  if (!value) return <div className="splash"><div className="spinner" /><p>Starting…</p></div>;
  return (
    <C.Provider value={value}>
      {children}
      <div className="toasts" role="status" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} className={`toast ${t.kind}`}>
            <span>{t.msg}</span>
            {t.undo && <button onClick={() => { t.undo?.(); setToasts((x) => x.filter((y) => y.id !== t.id)); }}>Undo</button>}
          </div>
        ))}
      </div>
    </C.Provider>
  );
}
