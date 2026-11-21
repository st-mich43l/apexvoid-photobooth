import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { api, on, toAppError, AppError } from "./api";
import type { BootInfo, CameraStatus, LibraryStatus, Settings, ShootSnapshot, VideoState } from "./types";

export interface Ctx {
  boot: BootInfo;
  camera: CameraStatus;
  shoot: ShootSnapshot;
  video: VideoState;
  settings: Settings;
  location: LibraryStatus;
  /** bumped whenever the library changes (new capture/recording/edit/delete) */
  libraryVersion: number;
  bumpLibrary: () => void;
  mediaUrl: (id: string, kind: "thumb" | "preview" | "basepreview" | "original" | "edited", v?: string) => string;
  previewUrl: string;
  setSettings: (s: Settings) => void;
  toast: (msg: string, kind?: "ok" | "err", undo?: () => void) => void;
}

const C = createContext<Ctx | null>(null);
export const useApp = (): Ctx => {
  const c = useContext(C);
  if (!c) throw new Error("AppProvider missing");
  return c;
};

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
      on<CameraStatus>("camera:status", (s) => { setCamera(s); }),
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

  // a live-view <img> must be re-opened whenever the stream restarts
  const lv = camera?.liveView, st = camera?.state;
  useEffect(() => { setPreviewNonce((n) => n + 1); }, [lv, st, camera?.device?.id]);

  const toast = useCallback((msg: string, kind: "ok" | "err" = "ok", undo?: () => void) => {
    const id = nextId.current++;
    setToasts((t) => [...t, { id, msg, kind, undo }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), undo ? 8000 : 5000);
  }, []);

  const value = useMemo<Ctx | null>(() => {
    if (!boot || !camera || !shoot || !video || !settings || !location) return null;
    const base = boot.mediaBase, tok = boot.mediaToken;
    return {
      boot, camera, shoot, video, settings, location, libraryVersion,
      bumpLibrary: () => setLib((v) => v + 1),
      mediaUrl: (id, kind, v) => `${base}/media/${id}/${kind}?t=${tok}${v ? `&v=${encodeURIComponent(v)}` : ""}`,
      previewUrl: `${base}/preview.mjpg?t=${tok}&n=${previewNonce}`,
      setSettings: setSettingsState,
      toast,
    };
  }, [boot, camera, shoot, video, settings, location, libraryVersion, previewNonce, toast]);

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
