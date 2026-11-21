import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { useApp } from "./state";
import { SourceChip } from "./SourceChip";
import { ErrorNote, LivePreview, SimulatedRibbon, useAction } from "./ui";

/** The photo workflow, shared by Studio and Booth:
 *  PREVIEW → COUNTDOWN → CAPTURING → REVIEW (retake/accept) → SAVING → COMPLETED. */
export function ShootFlow({ variant, shots: fixedShots, onDone }: { variant: "studio" | "booth"; shots?: number; onDone?: () => void }) {
  const { shoot, camera, settings, mediaUrl } = useApp();
  const [shots, setShots] = useState(fixedShots ?? settings.shots);
  const { busy, error, run, clear } = useAction();
  const booth = variant === "booth";

  // enter: bring live view up as soon as a camera is connected
  const started = useRef(false);
  useEffect(() => {
    if (shoot.state === "IDLE" && camera.device && camera.state !== "error" && !camera.reconnecting && !started.current) {
      started.current = true;
      run(() => api.startPreview()).finally(() => { started.current = false; });
    }
  }, [shoot.state, camera.device, camera.state, camera.reconnecting]); // eslint-disable-line react-hooks/exhaustive-deps

  // leave: release live view; abandon a half-finished session
  const stateRef = useRef(shoot.state);
  stateRef.current = shoot.state;
  useEffect(() => () => {
    const s = stateRef.current;
    const finish = () => api.resetShooting().catch(() => {});
    if (s === "COUNTDOWN" || s === "CAPTURING" || s === "REVIEW") api.cancelShooting().then(finish, finish);
    else finish();
  }, []);

  const s = shoot;
  const progress = `${Math.min(s.taken.length + (s.state === "COUNTDOWN" || s.state === "CAPTURING" ? 1 : 0), s.shots)} / ${s.shots}`;
  const showPreview = ["PREVIEW", "COUNTDOWN", "CAPTURING"].includes(s.state);

  return (
    <div className={`shoot ${booth ? "booth" : ""}`}>
      {!booth && <SimulatedRibbon />}
      <SourceChip />
      {camera.reconnecting && <div className="note warn">Camera disconnected — reconnecting automatically…</div>}
      <ErrorNote error={error} onClose={clear} />

      {!camera.device && !camera.reconnecting && (
        <div className="note warn">{booth ? "The camera is not ready. Please ask the host." : "No camera connected. Connect one on the Camera page."}</div>
      )}

      {showPreview && (
        <div className="stage">
          <LivePreview className="stage-preview" />
          {s.state === "COUNTDOWN" && s.remaining > 0 && <div className="count" key={`${s.slot}-${s.remaining}`}>{s.remaining}</div>}
          {s.state === "CAPTURING" && <div className="flash">Smile!</div>}
          {s.state !== "PREVIEW" && <div className="progress-pill">Photo {progress}</div>}
        </div>
      )}

      {s.state === "IDLE" && <div className="center-msg"><div className="spinner" />Getting the camera ready…</div>}

      {s.state === "PREVIEW" && (
        <div className="controls">
          {!booth && !fixedShots && (
            <div className="seg" role="radiogroup" aria-label="Number of photographs">
              {[1, 2, 4, 6].map((n) => (
                <button key={n} role="radio" aria-checked={shots === n} className={shots === n ? "on" : ""} onClick={() => setShots(n)}>{n}</button>
              ))}
            </div>
          )}
          <button className="shutter" disabled={busy || !camera.liveView} onClick={() => run(() => api.startShooting(shots))}>
            {booth ? "TAP TO START" : "Start"}
          </button>
        </div>
      )}

      {(s.state === "COUNTDOWN" || s.state === "CAPTURING") && (
        <div className="controls"><button onClick={() => run(() => api.cancelShooting())} disabled={busy}>Cancel</button></div>
      )}

      {s.state === "REVIEW" && (
        <div className="review">
          {s.warning && <div className="note warn">{s.warning}</div>}
          <h2>{booth ? "How do they look?" : "Review"}</h2>
          <div className={`thumbs n${s.shots}`}>
            {[...s.taken].sort((a, b) => a.slot - b.slot).map((t) => (
              <figure key={t.mediaId}>
                <img src={mediaUrl(t.mediaId, "preview")} alt={`Photo ${t.slot + 1}`} />
                <figcaption>
                  <span>{t.slot + 1}</span>
                  <button disabled={busy} onClick={() => run(() => api.retake(t.slot))}>Retake</button>
                </figcaption>
              </figure>
            ))}
          </div>
          <div className="controls">
            <button disabled={busy} onClick={() => run(() => api.cancelShooting())}>Start over</button>
            <button className="primary big" disabled={busy} onClick={() => run(() => api.accept())}>Looks good</button>
          </div>
        </div>
      )}

      {s.state === "SAVING" && <div className="center-msg"><div className="spinner" />Saving your photos…</div>}

      {s.state === "COMPLETED" && (
        <div className="done">
          <h2>{booth ? "All done!" : "Saved"}</h2>
          {s.warning && <div className="note warn">{s.warning}</div>}
          <div className="result">
            {s.resultId
              ? <img className="strip" src={mediaUrl(s.resultId, "preview")} alt="Your photo strip" />
              : <div className={`thumbs n${s.shots}`}>{s.taken.map((t) => <img key={t.mediaId} src={mediaUrl(t.mediaId, "preview")} alt="" />)}</div>}
          </div>
          <div className="controls">
            <button className="primary big" onClick={() => { api.resetShooting().then(() => onDone?.()); }}>
              {booth ? "Finish" : "New session"}
            </button>
          </div>
        </div>
      )}

      {s.state === "ERROR" && (
        <div className="center-msg">
          <div className="note err">
            <div><strong>{booth ? "Something went wrong with the camera." : s.error}</strong>
              <div className="action">{booth ? "Please ask the host for help." : s.errorAction}</div></div>
          </div>
          <button className="primary" onClick={() => api.resetShooting()}>Try again</button>
        </div>
      )}

      {s.state === "CANCELLED" && (
        <div className="center-msg"><p>Session cancelled.</p><button className="primary" onClick={() => api.resetShooting()}>Start again</button></div>
      )}
    </div>
  );
}
