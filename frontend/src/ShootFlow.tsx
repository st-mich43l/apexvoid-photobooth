import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { FrameSelection } from "./FrameSelection";
import { photosLabel } from "./frameInfo";
import { useApp } from "./state";
import { ResultPrint } from "./ResultPrint";
import { SourceChip } from "./SourceChip";
import { ErrorNote, LivePreview, SimulatedRibbon, useAction } from "./ui";
import type { FrameChoice } from "./types";

/** The photo workflow, shared by Studio and Booth. The frame is chosen BEFORE this
 *  component is shown, decides the number of photos, and stays with the session:
 *  PREVIEW → COUNTDOWN → CAPTURING → REVIEW (retake / change design / accept) → SAVING → COMPLETED. */
export function ShootFlow({ variant, frame: initialFrame, onChangeFrame, onDone }:
  { variant: "studio" | "booth"; frame: FrameChoice; onChangeFrame?: (current: FrameChoice) => void; onDone?: () => void }) {
  const { shoot, camera, mediaUrl, frameUrl, toast } = useApp();
  const [frame, setFrame] = useState(initialFrame);
  const [changing, setChanging] = useState(false);
  const [composed, setComposed] = useState<{ session: string; mediaId: string } | null>(null);
  const { busy, error, run, clear } = useAction();
  const booth = variant === "booth";
  const stateRef0 = useRef(shoot.state); // the state this screen was opened in

  // enter: bring live view up as soon as a camera is connected
  const started = useRef(false);
  useEffect(() => {
    if (shoot.state === "IDLE" && camera.device && camera.state !== "error" && !camera.reconnecting && !started.current) {
      started.current = true;
      run(() => api.startPreview()).finally(() => { started.current = false; });
    }
  }, [shoot.state, camera.device, camera.state, camera.reconnecting]); // eslint-disable-line react-hooks/exhaustive-deps

  // a new session never starts on top of the previous one's "All done" / error screen: clear what is left over
  useEffect(() => {
    const st = stateRef0.current;
    if (st !== "IDLE" && st !== "PREVIEW") api.cancelShooting().catch(() => {}).finally(() => api.resetShooting().catch(() => {}));
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

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
  const resultId = composed && composed.session === s.sessionId ? composed.mediaId : s.resultId;

  const chip = (
    <figure className="frame-side" aria-label={`Frame: ${frame.name}`}>
      <img src={frameUrl(frame.id, frame.variantId, 200, frame.caption)} alt="" />
      <figcaption><b>{frame.name}</b><small>{photosLabel(frame.photos)}</small></figcaption>
    </figure>
  );

  // a different design for the photos that exist: no retake unless the guest picks a frame that needs more photos
  const applyDesign = (c: FrameChoice, d: { useFirst: boolean; restart: boolean }) => {
    setChanging(false);
    if (d.restart) {
      run(async () => { if (s.state !== "COMPLETED") await api.cancelShooting(); await api.resetShooting(); setFrame(c); });
      return;
    }
    run(async () => {
      if (s.state === "COMPLETED") {
        const m = await api.composeSession(s.sessionId, c.id, c.variantId, c.caption, d.useFirst);
        setComposed({ session: s.sessionId, mediaId: m.id });
      } else {
        await api.setSessionFrame(s.sessionId, c.id, c.variantId, c.caption, d.useFirst);
      }
      setFrame(c);
      toast(`Design changed to ${c.name}`);
    });
  };

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
       <div className="stage-row">
        <div className="stage">
          <LivePreview className="stage-preview" />
          {s.state === "COUNTDOWN" && s.remaining > 0 && <div className="count" key={`${s.slot}-${s.remaining}`}>{s.remaining}</div>}
          {s.state === "CAPTURING" && <div className="flash">Smile!</div>}
          {s.state !== "PREVIEW" && <div className="progress-pill">Photo {progress}</div>}
        </div>
        {chip}
       </div>
      )}

      {s.state === "IDLE" && <div className="center-msg"><div className="spinner" />Getting the camera ready…</div>}

      {s.state === "PREVIEW" && (
        <div className="controls">
          {onChangeFrame && <button disabled={busy} onClick={() => onChangeFrame(frame)}>Change frame</button>}
          <button className="shutter" disabled={busy || !camera.liveView} onClick={() => run(() => api.startShooting(frame.id, frame.variantId, frame.caption))}>
            {booth ? "TAP TO START" : "Start"} · {photosLabel(frame.photos)}
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
          <div className="review-frame">{chip}<button disabled={busy} onClick={() => setChanging(true)}>Change design</button></div>
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
            {resultId
              ? <img className="strip" src={mediaUrl(resultId, "preview")} alt="Your photo strip" />
              : <div className={`thumbs n${s.shots}`}>{s.taken.map((t) => <img key={t.mediaId} src={mediaUrl(t.mediaId, "preview")} alt="" />)}</div>}
          </div>
          <div className="controls">
            <button disabled={busy} onClick={() => setChanging(true)}>Change design</button>
            {resultId && <ResultPrint mediaId={resultId} booth={booth} />}
            <button className="primary big" onClick={() => { onDone?.(); api.resetShooting().catch(() => {}); }}>
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
      {changing && (
        <div className="design-picker" role="dialog" aria-modal="true" aria-label="Change design">
          <FrameSelection variant={variant} takenPhotos={s.taken.length} initial={{ id: frame.id, variantId: frame.variantId, caption: frame.caption }}
            title="Change the design" backLabel="Keep this design" confirmLabel="Use this design" busy={busy} onConfirm={applyDesign} onBack={() => setChanging(false)} />
        </div>
      )}
    </div>
  );
}
