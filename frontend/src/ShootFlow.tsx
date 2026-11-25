import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { FrameSelection } from "./FrameSelection";
import { photosLabel } from "./frameInfo";
import { GuestShoot, PhotoSteps } from "./guest/GuestShoot";
import { LeaveDialog } from "./LeaveDialog";
import { ResultPrint } from "./ResultPrint";
import { sessionGate } from "./sessionGate";
import { SourceChip } from "./SourceChip";
import { useLocale } from "./locale";
import { useApp } from "./state";
import { ErrorNote, LivePreview, SimulatedRibbon, useAction } from "./ui";
import type { FrameChoice } from "./types";

export { PhotoSteps };

/** True after `flag` has been continuously set for `ms` (used to add "ask the host" to a long reconnect). */
function useAfter(flag: boolean, ms: number): boolean {
  const [late, setLate] = useState(false);
  useEffect(() => {
    if (!flag) { setLate(false); return; }
    const t = window.setTimeout(() => setLate(true), ms);
    return () => window.clearTimeout(t);
  }, [flag, ms]);
  return late;
}

/** The photo workflow, shared by Studio and Booth. The frame is chosen BEFORE this
 *  component is shown, decides the number of photos, and stays with the session:
 *  PREVIEW → COUNTDOWN → CAPTURING → REVIEW (retake / change design / accept) → SAVING → COMPLETED.
 *
 *  Leaving a session is never done here directly: everything goes through sessionGate, the one
 *  serialised, idempotent reset, and abandoning an unfinished session asks first. */
export function ShootFlow({ variant, frame: initialFrame, onChangeFrame, onDone }:
  { variant: "studio" | "booth"; frame: FrameChoice; onChangeFrame?: (current: FrameChoice) => void; onDone?: () => void }) {
  const { shoot, camera, mediaUrl, frameUrl, toast, settings } = useApp();
  const language = useLocale(settings?.language);
  const [frame, setFrame] = useState(initialFrame);
  const [changing, setChanging] = useState(false);
  const [composed, setComposed] = useState<{ session: string; mediaId: string } | null>(null);
  const [abandon, setAbandon] = useState(false);
  const [finishing, setFinishing] = useState(false);
  const [ready, setReady] = useState(false); // the previous session's reset is over
  const { busy, error, run, clear } = useAction();
  const booth = variant === "booth";

  const mountState = useRef(shoot.state); // the state this screen was opened in (read synchronously, before any await)

  // enter: wait for any reset still running, clear a leftover finished/failed session (through the same gate), then go
  useEffect(() => {
    let alive = true;
    (async () => {
      await sessionGate.ready();
      const st = mountState.current;
      if (st !== "IDLE" && st !== "PREVIEW") await sessionGate.reset();
      if (alive) setReady(true);
    })();
    return () => { alive = false; };
  }, []);

  // bring live view up as soon as a camera is connected
  const started = useRef(false);
  useEffect(() => {
    if (ready && shoot.state === "IDLE" && camera.device && camera.state !== "error" && !camera.reconnecting && !started.current) {
      started.current = true;
      run(() => api.startPreview()).finally(() => { started.current = false; });
    }
  }, [ready, shoot.state, camera.device, camera.state, camera.reconnecting]); // eslint-disable-line react-hooks/exhaustive-deps

  // leave: release live view and close what is left (idempotent; Booth has already done it for Home/Finish, so this is the same reset)
  useEffect(() => () => { void sessionGate.reset(); }, []);

  const s = shoot;
  const showPreview = ["PREVIEW", "COUNTDOWN", "CAPTURING"].includes(s.state);
  const resultId = composed && composed.session === s.sessionId ? composed.mediaId : s.resultId;
  const slow = useAfter(camera.reconnecting, 30_000);

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
      run(async () => { await sessionGate.reset(); setFrame(c); });
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

  const finish = () => {
    if (finishing) return; // a second tap on Finish does nothing
    setFinishing(true);
    if (onDone) onDone(); else void sessionGate.reset().finally(() => setFinishing(false));
  };
  const startOver = async () => { await sessionGate.reset(); setAbandon(false); };

  const dialogs = (
    <>
      {abandon && (
        <LeaveDialog language={language} state={s.state} taken={s.taken.length} busy={false} confirmLabel="Start over" onKeep={() => setAbandon(false)} onLeave={() => { void startOver(); }} />
      )}
      {changing && (
        <div className="design-picker" role="dialog" aria-modal="true" aria-label="Change design">
          <FrameSelection variant={variant} takenPhotos={s.taken.length} initial={{ id: frame.id, variantId: frame.variantId, caption: frame.caption }}
            title="Change the design" backLabel="Keep this design" confirmLabel="Use this design" busy={busy} onConfirm={applyDesign} onBack={() => setChanging(false)} />
        </div>
      )}
    </>
  );

  if (booth) {
    return (
      <div className="shoot booth g-screen">
        <GuestShoot language={language} s={s} camera={camera} frame={frame} countdownSeconds={settings?.countdownSeconds} busy={busy} error={!!error} onClearError={clear} slow={slow}
          mediaUrl={mediaUrl} frameUrl={frameUrl} resultId={resultId} finishing={finishing}
          print={resultId ? <ResultPrint mediaId={resultId} booth /> : null}
          onStart={() => run(() => api.startShooting(frame.id, frame.variantId, frame.caption))}
          onChangeFrame={onChangeFrame ? () => onChangeFrame(frame) : undefined}
          onRetake={(slot) => run(() => api.retake(slot))} onAccept={() => run(() => api.accept())} onCancel={() => setAbandon(true)}
          onChangeDesign={() => setChanging(true)} onFinish={finish}
          onRetryPreview={() => { clear(); started.current = false; run(() => api.startPreview()); }} onReset={() => { void sessionGate.reset(); }} />
        {dialogs}
      </div>
    );
  }

  return (
    <div className="shoot">
      <SimulatedRibbon />
      <SourceChip />
      {camera.reconnecting && (booth
        ? <div className="reconnect-card" role="status"><div className="spinner" /><div><strong>Reconnecting the camera…</strong><span>Your photos are safe. This usually takes a few seconds.{slow ? " Still working — please ask the host if this takes much longer." : ""}</span></div></div>
        : <div className="note warn">Camera disconnected — reconnecting automatically… Photos already taken are kept.</div>)}
      {!booth && <ErrorNote error={error} onClose={clear} />}
      {booth && error && <div className="note warn" role="alert"><div>That didn’t work. Please try again, or ask the host.</div><button onClick={clear}>OK</button></div>}

      {!camera.device && !camera.reconnecting && (
        <div className="note warn">{booth ? "The camera is not ready. Please ask the host." : "No camera connected. Connect one on the Camera page."}</div>
      )}

      {showPreview && (
        <div className="stage-row">
          <div className="stage">
            <LivePreview className="stage-preview" />
            {s.state === "COUNTDOWN" && s.remaining > 0 && <div className="count" key={`${s.slot}-${s.remaining}`} aria-live="assertive"><span>{s.remaining}</span></div>}
            {s.state === "CAPTURING" && <div className="flash" aria-live="polite"><span>Smile!</span></div>}
            {s.state !== "PREVIEW" && <PhotoSteps total={s.shots} taken={s.taken.length} current={s.slot} />}
          </div>
          {chip}
        </div>
      )}

      {s.state === "IDLE" && !error && <div className="center-msg"><div className="spinner" />Getting the camera ready…</div>}
      {s.state === "IDLE" && error && booth && (
        <div className="center-msg"><p>We couldn’t start the camera.</p><button className="primary big" onClick={() => { clear(); started.current = false; run(() => api.startPreview()); }}>Try again</button></div>
      )}

      {s.state === "PREVIEW" && (
        <div className="controls">
          {onChangeFrame && <button disabled={busy} onClick={() => onChangeFrame(frame)}>Change frame</button>}
          <button className="shutter" disabled={busy || !camera.liveView || camera.reconnecting} onClick={() => run(() => api.startShooting(frame.id, frame.variantId, frame.caption))}>
            {booth ? "TAP TO START" : "Start"} · {photosLabel(frame.photos)}
          </button>
        </div>
      )}

      {s.state === "COUNTDOWN" && (
        <div className="controls"><button onClick={() => setAbandon(true)} disabled={busy}>Cancel</button></div>
      )}
      {s.state === "CAPTURING" && <div className="controls"><p className="muted">Hold still…</p></div>}

      {s.state === "REVIEW" && (
        <div className="review">
          {s.warning && <div className="note warn">{s.warning}</div>}
          <h2>{booth ? "How do they look?" : "Review"}</h2>
          <PhotoSteps total={s.shots} taken={s.taken.length} current={null} />
          <div className={`thumbs n${s.shots}`}>
            {[...s.taken].sort((a, b) => a.slot - b.slot).map((t) => (
              <figure key={t.mediaId}>
                <img src={mediaUrl(t.mediaId, "preview")} alt={`Photo ${t.slot + 1}`} />
                <figcaption>
                  <span>Photo {t.slot + 1}</span>
                  <button disabled={busy || camera.reconnecting} onClick={() => run(() => api.retake(t.slot))}>Retake</button>
                </figcaption>
              </figure>
            ))}
          </div>
          <div className="review-frame">{chip}<button disabled={busy} onClick={() => setChanging(true)}>Change design</button></div>
          <div className="controls">
            <button disabled={busy} onClick={() => setAbandon(true)}>Start over</button>
            <button className="primary big" disabled={busy} onClick={() => run(() => api.accept())}>Looks good</button>
          </div>
        </div>
      )}

      {s.state === "SAVING" && <div className="center-msg"><div className="spinner" />Saving your photos…</div>}

      {s.state === "COMPLETED" && (
        <div className="done">
          <h2>{booth ? "All done!" : "Saved"}</h2>
          {booth && <p className="muted">Your photos are ready.</p>}
          {s.warning && <div className="note warn">{s.warning}</div>}
          <div className="result">
            {resultId
              ? <img className="strip" src={mediaUrl(resultId, "preview")} alt="Your photo strip" />
              : <div className={`thumbs n${s.shots}`}>{s.taken.map((t) => <img key={t.mediaId} src={mediaUrl(t.mediaId, "preview")} alt="" />)}</div>}
          </div>
          <div className="controls">
            <button disabled={busy || finishing} onClick={() => setChanging(true)}>Change design</button>
            {resultId && <ResultPrint mediaId={resultId} booth={booth} />}
            <button className="primary big" disabled={finishing} onClick={finish}>{booth ? "Finish" : "New session"}</button>
          </div>
        </div>
      )}

      {s.state === "ERROR" && (
        <div className="center-msg">
          <div className="note err">
            <div><strong>{booth ? "We couldn’t take that photo." : s.error}</strong>
              <div className="action">{booth ? "Your other photos are safe. Tap below to try again, or ask the host." : s.errorAction}</div></div>
          </div>
          <button className="primary big" onClick={() => { void sessionGate.reset(); }}>Try again</button>
        </div>
      )}

      {s.state === "CANCELLED" && (
        <div className="center-msg"><p>Session cancelled.</p><button className="primary big" onClick={() => { void sessionGate.reset(); }}>Start again</button></div>
      )}

      {dialogs}
    </div>
  );
}
