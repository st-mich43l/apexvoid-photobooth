import { useCallback, useEffect, useRef, useState } from "react";
import { ExitDialog } from "./ExitDialog";
import { FrameSelection } from "./FrameSelection";
import { enterFullscreen } from "./fullscreen";
import { ShootFlow } from "./ShootFlow";
import { SimulatedRibbon } from "./ui";
import { useApp } from "./state";
import { VideoFlow } from "./VideoFlow";
import type { FrameChoice } from "./types";
import { usePrepareMode } from "./usePrepareMode";

type Screen = "welcome" | "frames" | "photo" | "video";

/** Booth Mode — the fullscreen guest interface. Touch-first, large targets,
 *  portrait and landscape. The operator leaves through the visible "Exit Booth"
 *  button, the Esc key, or by holding the top-left corner for 3 s — all three
 *  open the same confirmation (and the PIN prompt when kiosk protection is on). */
export function Booth({ onExited }: { onExited: () => void }) {
  const { camera, settings } = useApp();
  // #booth/photo and #booth/video deep-link into a flow (support, screenshots)
  const [screen, setScreen] = useState<Screen>(() => (window.location.hash === "#booth/photo" ? "frames" : window.location.hash === "#booth/video" ? "video" : "welcome"));
  const [asking, setAsking] = useState(false);
  const [fsError, setFsError] = useState("");
  const { prepare, dialog: modeDialog, error: modeError } = usePrepareMode();
  // the frame is chosen first (no camera, no live view, no session yet); the video flow is unchanged
  const [frame, setFrame] = useState<FrameChoice | null>(null);
  const [preparing, setPreparing] = useState(false);
  const choosePhoto = () => setScreen("frames");
  const chooseVideo = async () => { if (await prepare("video")) setScreen("video"); };
  const confirmFrame = async (c: FrameChoice) => {
    setPreparing(true);
    try { if (await prepare("photo")) { setFrame(c); setScreen("photo"); } } finally { setPreparing(false); }
  };
  const idle = useRef<number | undefined>(undefined);

  const goFullscreen = useCallback(() => {
    setFsError("");
    enterFullscreen().catch((e) => setFsError(e instanceof Error ? e.message : String(e)));
  }, []);
  useEffect(() => { goFullscreen(); }, [goFullscreen]);

  // Esc opens the exit confirmation (the dialog handles Esc itself while open)
  useEffect(() => {
    if (asking) return;
    const f = (e: KeyboardEvent) => { if (e.key === "Escape") { e.preventDefault(); setAsking(true); } };
    window.addEventListener("keydown", f);
    return () => window.removeEventListener("keydown", f);
  }, [asking]);

  // no touches for 90 s on a non-welcome screen → back to the welcome screen
  const resetIdle = useCallback(() => {
    window.clearTimeout(idle.current);
    if (screen !== "welcome" && !asking) idle.current = window.setTimeout(() => setScreen("welcome"), 90_000);
  }, [screen, asking]);
  useEffect(() => { resetIdle(); return () => window.clearTimeout(idle.current); }, [resetIdle]);

  // secondary shortcut: hold the top-left corner for 3 s
  const hold = useRef<number | undefined>(undefined);
  const [holding, setHolding] = useState(false);
  const startHold = () => { setHolding(true); hold.current = window.setTimeout(() => { setHolding(false); setAsking(true); }, 3000); };
  const endHold = () => { setHolding(false); window.clearTimeout(hold.current); };
  useEffect(() => () => window.clearTimeout(hold.current), []);

  const ready = camera.device && camera.state !== "error" && !camera.reconnecting;

  return (
    <div className="booth-root" onPointerDown={resetIdle}>
      <SimulatedRibbon />
      <div className={`exit-corner ${holding ? "holding" : ""}`} data-testid="exit-corner" onPointerDown={startHold} onPointerUp={endHold} onPointerLeave={endHold} onPointerCancel={endHold} aria-hidden />
      <button className="exit-btn" onClick={() => setAsking(true)} aria-label="Exit Booth Mode" title="Exit Booth Mode (Esc)">
        <span aria-hidden>⎋</span> Exit Booth
      </button>
      {fsError && (
        <div className="note warn fs-note" role="alert">
          <div>Fullscreen is not active: {fsError} Booth Mode keeps working in a window.</div>
          <button onClick={goFullscreen}>Retry</button>
        </div>
      )}
      {screen === "welcome" && (
        <div className="welcome">
          <h1>Say cheese!</h1>
          <p>Choose what you would like to make</p>
          {!ready && <div className="note warn">The camera is not ready yet. Please ask the host.</div>}
          {modeError && <div className="note err" role="alert"><div>{modeError}</div></div>}
          <div className="choices">
            <button className="choice photo" onClick={choosePhoto}><span aria-hidden>📷</span>Photo<small>choose a frame</small></button>
            <button className="choice video" onClick={chooseVideo}><span aria-hidden>🎬</span>Video<small>up to {settings.video.maxSeconds}s</small></button>
          </div>
        </div>
      )}
      {screen === "frames" && (
        <>
          {modeError && <div className="note err" role="alert"><div>{modeError}</div></div>}
          <FrameSelection variant="booth" busy={preparing} initial={frame ? { id: frame.id, variantId: frame.variantId, caption: frame.caption } : undefined}
            onConfirm={confirmFrame} onBack={() => setScreen("welcome")} backLabel="Home" />
        </>
      )}
      {screen === "photo" && frame && <ShootFlow variant="booth" frame={frame} onChangeFrame={() => setScreen("frames")} onDone={() => { setFrame(null); setScreen("welcome"); }} />}
      {screen === "video" && <VideoFlow variant="booth" onDone={() => setScreen("welcome")} />}
      {screen !== "welcome" && screen !== "frames" && <button className="home-btn" onClick={() => setScreen("welcome")}>← Home</button>}
      {modeDialog}
      {asking && <ExitDialog onClose={() => setAsking(false)} onExited={onExited} />}
    </div>
  );
}
