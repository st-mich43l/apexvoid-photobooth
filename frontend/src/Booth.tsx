import { useCallback, useEffect, useRef, useState } from "react";
import { Window } from "@wailsio/runtime";
import { ShootFlow } from "./ShootFlow";
import { SimulatedRibbon } from "./ui";
import { useApp } from "./state";
import { VideoFlow } from "./VideoFlow";

type Screen = "welcome" | "photo" | "video";

/** Booth Mode — the fullscreen guest interface. Touch-first, large targets,
 *  works in portrait and landscape. The operator leaves by holding the
 *  top-left corner for 3 s (an operator PIN is a later milestone). */
export function Booth({ onExit }: { onExit: () => void }) {
  const { camera, settings } = useApp();
  // #booth/photo and #booth/video deep-link into a flow (support, screenshots)
  const [screen, setScreen] = useState<Screen>(() => (location.hash === "#booth/photo" ? "photo" : location.hash === "#booth/video" ? "video" : "welcome"));
  const idle = useRef<number | undefined>(undefined);

  useEffect(() => { Window.Fullscreen().catch(() => {}); return () => { Window.UnFullscreen().catch(() => {}); }; }, []);

  // no touches for 90 s on a non-welcome screen → back to the welcome screen
  const resetIdle = useCallback(() => {
    window.clearTimeout(idle.current);
    if (screen !== "welcome") idle.current = window.setTimeout(() => setScreen("welcome"), 90_000);
  }, [screen]);
  useEffect(() => { resetIdle(); return () => window.clearTimeout(idle.current); }, [resetIdle]);

  // hidden operator exit
  const hold = useRef<number | undefined>(undefined);
  const [holding, setHolding] = useState(false);
  const startHold = () => { setHolding(true); hold.current = window.setTimeout(onExit, 3000); };
  const endHold = () => { setHolding(false); window.clearTimeout(hold.current); };

  const ready = camera.device && camera.state !== "error" && !camera.reconnecting;

  return (
    <div className="booth-root" onPointerDown={resetIdle}>
      <SimulatedRibbon />
      <div className={`exit-corner ${holding ? "holding" : ""}`} onPointerDown={startHold} onPointerUp={endHold} onPointerLeave={endHold} onPointerCancel={endHold} aria-hidden />
      {screen === "welcome" && (
        <div className="welcome">
          <h1>Say cheese!</h1>
          <p>Choose what you would like to make</p>
          {!ready && <div className="note warn">The camera is not ready yet. Please ask the host.</div>}
          <div className="choices">
            <button className="choice photo" disabled={!ready} onClick={() => setScreen("photo")}><span aria-hidden>📷</span>Photo<small>{settings.shots} shots</small></button>
            <button className="choice video" disabled={!ready} onClick={() => setScreen("video")}><span aria-hidden>🎬</span>Video<small>up to {settings.video.maxSeconds}s</small></button>
          </div>
        </div>
      )}
      {screen === "photo" && <ShootFlow variant="booth" onDone={() => setScreen("welcome")} />}
      {screen === "video" && <VideoFlow variant="booth" onDone={() => setScreen("welcome")} />}
      {screen !== "welcome" && <button className="back-btn" onClick={() => setScreen("welcome")}>← Home</button>}
    </div>
  );
}
