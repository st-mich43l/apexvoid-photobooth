import { useEffect, useRef, useState } from "react";
import { frameDisplayName } from "../../shared/frameInfo";
import { api } from "../../shared/api";
import { FrameSelection } from "../frames/FrameSelection";
import { GuestShoot, PhotoSteps } from "./guest/GuestShoot";
import { LeaveDialog } from "./LeaveDialog";
import { ResultPrint } from "./ResultPrint";
import { sessionGate } from "./sessionGate";
import { SourceChip } from "./SourceChip";
import { formatTranslation, photoCount, translate } from "../../shared/i18n";
import { useLocale } from "../../shared/locale";
import { useApp } from "../../app/state";
import { ErrorNote, LocalizedDiagnostic, LivePreview, SimulatedRibbon, useAction } from "../../shared/ui";
import type { FrameChoice } from "../../shared/types";

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
 *  component is shown, supplies the frame slots, and stays with the session:
 *  PREVIEW → COUNTDOWN → CAPTURING → REVIEW (retake / change design / accept) → SAVING → COMPLETED.
 *
 *  Leaving a session is never done here directly: everything goes through sessionGate, the one
 *  serialised, idempotent reset, and abandoning an unfinished session asks first. */
export function ShootFlow({ variant, frame: initialFrame, onChangeFrame, onDone }:
  { variant: "studio" | "booth"; frame: FrameChoice; onChangeFrame?: (current: FrameChoice) => void; onDone?: () => void }) {
  const { shoot, camera, mediaUrl, frameUrl, toast, settings } = useApp();
  const language = useLocale(settings?.language);
  const t = (key: Parameters<typeof translate>[1]) => translate(language, key);
  const [frame, setFrame] = useState(initialFrame);
  const [reviewFrameSlot, setReviewFrameSlot] = useState(0);
  const [reviewSelectedSlots, setReviewSelectedSlots] = useState<number[]>([]);
  const [changing, setChanging] = useState(false);
  const [composed, setComposed] = useState<{ session: string; mediaId: string } | null>(null);
  const [abandon, setAbandon] = useState(false);
  const [finishing, setFinishing] = useState(false);
  const [ready, setReady] = useState(false); // the previous session's reset is over
  const { busy, error, run, clear } = useAction();
  const booth = variant === "booth";

  useEffect(() => {
    setReviewSelectedSlots(Array.from({ length: frame.photos }, (_, i) => i));
    setReviewFrameSlot(0);
  }, [shoot.sessionId, frame.id, frame.photos]);

  const assignReviewPhoto = (captureSlot: number) => setReviewSelectedSlots((prev) => {
    const next = [...prev];
    const other = next.indexOf(captureSlot);
    if (other >= 0 && other !== reviewFrameSlot) next[other] = next[reviewFrameSlot];
    next[reviewFrameSlot] = captureSlot;
    return next;
  });
  const selectionsComplete = reviewSelectedSlots.length === frame.photos && new Set(reviewSelectedSlots).size === frame.photos && reviewSelectedSlots.every((slot) => shoot.taken.some((shot) => shot.slot === slot));

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
    <figure className="frame-side" aria-label={formatTranslation(language, "frameAlt", { name: frameDisplayName(frame, language) })}>
      <img src={frameUrl(frame.id, frame.variantId, 200, frame.caption)} alt="" />
      <figcaption><b>{frameDisplayName(frame, language)}</b><small>{photoCount(language, frame.photos)}</small></figcaption>
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
      toast(formatTranslation(language, "designChanged", { name: c.name }));
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
        <LeaveDialog language={language} state={s.state} taken={s.taken.length} busy={false} confirmLabel={t("startOver")} onKeep={() => setAbandon(false)} onLeave={() => { void startOver(); }} />
      )}
      {changing && (
        <div className="design-picker" role="dialog" aria-modal="true" aria-label={t("changeDesign")}>
          <FrameSelection variant={variant} takenPhotos={s.taken.length} initial={{ id: frame.id, variantId: frame.variantId, caption: frame.caption }}
            title={t("designTitle")} backLabel={t("keepDesign")} confirmLabel={t("useDesign")} busy={busy} onConfirm={applyDesign} onBack={() => setChanging(false)} />
        </div>
      )}
    </>
  );

  if (booth) {
    return (
      <div className="shoot booth g-screen">
        <GuestShoot language={language} s={s} camera={camera} frame={frame} countdownSeconds={settings?.countdownSeconds} plannedShots={Math.max(settings?.shots ?? 8, frame.photos)} busy={busy} error={!!error} onClearError={clear} slow={slow}
          mediaUrl={mediaUrl} frameUrl={frameUrl} resultId={resultId} finishing={finishing}
          print={resultId ? <ResultPrint mediaId={resultId} booth /> : null}
          onStart={() => run(() => api.startShooting(frame.id, frame.variantId, frame.caption))}
          onChangeFrame={onChangeFrame ? () => onChangeFrame(frame) : undefined}
          onRetake={(slot) => run(() => api.retake(slot))} onAccept={(slots) => run(() => api.accept(slots))} onCancel={() => setAbandon(true)}
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
        ? <div className="reconnect-card" role="status"><div className="spinner" /><div><strong>{t("reconnecting")}</strong><span>{t("safePhotos")}{slow ? t("slowReconnect") : ""}</span></div></div>
        : <div className="note warn">{t("studioReconnect")}</div>)}
      {!booth && <ErrorNote error={error} onClose={clear} />}
      {booth && error && <div className="note warn" role="alert"><div>{t("errorGuest")}</div><button onClick={clear}>{t("ok")}</button></div>}

      {!camera.device && !camera.reconnecting && (
        <div className="note warn">{booth ? "The camera is not ready. Please ask the host." : t("studioNoCamera")}</div>
      )}

      {showPreview && (
        <div className="stage-row">
          <div className="stage">
            <LivePreview className="stage-preview" />
            {s.state === "COUNTDOWN" && s.remaining > 0 && <div className="count" key={`${s.slot}-${s.remaining}`} aria-live="assertive"><span>{s.remaining}</span></div>}
            {s.state === "CAPTURING" && <div className="flash" aria-live="polite"><span>{t("smile")}</span></div>}
            {s.state !== "PREVIEW" && <PhotoSteps language={language} total={s.shots} taken={s.taken.length} current={s.slot} />}
          </div>
          {chip}
        </div>
      )}

      {s.state === "IDLE" && !error && <div className="center-msg"><div className="spinner" />{t("cameraPreparing")}</div>}
      {s.state === "IDLE" && error && booth && (
        <div className="center-msg"><p>{t("cameraStartError")}</p><button className="primary big" onClick={() => { clear(); started.current = false; run(() => api.startPreview()); }}>{t("tryAgain")}</button></div>
      )}

      {s.state === "PREVIEW" && (
        <div className="controls">
          {onChangeFrame && <button disabled={busy} onClick={() => onChangeFrame(frame)}>{t("changeFrame")}</button>}
          <button className="shutter" disabled={busy || !camera.liveView || camera.reconnecting} onClick={() => run(() => api.startShooting(frame.id, frame.variantId, frame.caption))}>
            {booth ? t("startShooting") : t("start")} · {photoCount(language, Math.max(settings?.shots ?? 8, frame.photos))}
          </button>
          <p className="muted">{formatTranslation(language, "countPrompt", { photos: Math.max(settings?.shots ?? 8, frame.photos), count: frame.photos })}</p>
        </div>
      )}

      {s.state === "COUNTDOWN" && (
        <div className="controls"><button onClick={() => setAbandon(true)} disabled={busy}>{t("cancel")}</button></div>
      )}
      {s.state === "CAPTURING" && <div className="controls"><p className="muted">{t("holdStill")}</p></div>}

      {s.state === "REVIEW" && (
        <div className="review">
          {s.warning && <div className="note warn"><LocalizedDiagnostic text={s.warning} summary="captureWarning" /></div>}
          <h2>{booth ? t("reviewHeading") : t("review")}</h2>
          <PhotoSteps language={language} total={s.shots} taken={s.taken.length} current={null} />
          <section className="photo-picker">
            <h3>{formatTranslation(language, "chooseFramePhotos", { count: frame.photos })}</h3>
            <p className="muted">{t("chooseFramePhotosHint")}</p>
            <div className="frame-slots" role="group" aria-label={formatTranslation(language, "chooseFramePhotos", { count: frame.photos })}>
              {Array.from({ length: frame.photos }, (_, i) => (
                <button key={i} className={reviewFrameSlot === i ? "primary" : ""} aria-pressed={reviewFrameSlot === i} disabled={busy} onClick={() => setReviewFrameSlot(i)}>
                  {formatTranslation(language, "framePhotoSlot", { number: i + 1 })}
                  <small>{reviewSelectedSlots[i] === undefined ? "—" : formatTranslation(language, "photoNumber", { number: reviewSelectedSlots[i] + 1 })}</small>
                </button>
              ))}
            </div>
          </section>
          <div className={`thumbs n${s.shots}`}>
            {[...s.taken].sort((a, b) => a.slot - b.slot).map((t) => (
              <figure className={reviewSelectedSlots.includes(t.slot) ? "selected" : ""} key={t.mediaId}>
                <img src={mediaUrl(t.mediaId, "preview")} alt={formatTranslation(language, "photoNumber", { number: t.slot + 1 })} />
                <figcaption>
                  <span>{formatTranslation(language, "photoNumber", { number: t.slot + 1 })}</span>
                  <button className={reviewSelectedSlots[reviewFrameSlot] === t.slot ? "primary" : ""} aria-pressed={reviewSelectedSlots[reviewFrameSlot] === t.slot} disabled={busy} onClick={() => assignReviewPhoto(t.slot)}>
                    {reviewSelectedSlots.includes(t.slot) ? formatTranslation(language, "selectedForFrameSlot", { number: reviewSelectedSlots.indexOf(t.slot) + 1 }) : formatTranslation(language, "useForFrameSlot", { number: reviewFrameSlot + 1 })}
                  </button>
                  <button disabled={busy || camera.reconnecting} onClick={() => run(() => api.retake(t.slot))}>{translate(language, "retake")}</button>
                </figcaption>
              </figure>
            ))}
          </div>
          <div className="review-frame">{chip}<button disabled={busy} onClick={() => setChanging(true)}>{t("changeDesign")}</button></div>
          <div className="controls">
            <button disabled={busy} onClick={() => setAbandon(true)}>{t("startOver")}</button>
            <button className="primary big" disabled={busy || !selectionsComplete} onClick={() => run(() => api.accept(reviewSelectedSlots))}>{t("looksGood")}</button>
          </div>
        </div>
      )}

      {s.state === "SAVING" && <div className="center-msg"><div className="spinner" />{t("saving")}</div>}

      {s.state === "COMPLETED" && (
        <div className="done">
          <h2>{booth ? t("allDone") : t("saved")}</h2>
          {booth && <p className="muted">{t("photosReady")}</p>}
          {s.warning && <div className="note warn"><LocalizedDiagnostic text={s.warning} summary="captureWarning" /></div>}
          <div className="result">
            {resultId
              ? <img className="strip" src={mediaUrl(resultId, "preview")} alt={t("photoStripAlt")} />
              : <div className={`thumbs n${s.shots}`}>{s.taken.map((t) => <img key={t.mediaId} src={mediaUrl(t.mediaId, "preview")} alt="" />)}</div>}
          </div>
          <div className="controls">
            <button disabled={busy || finishing} onClick={() => setChanging(true)}>{t("changeDesign")}</button>
            {resultId && <ResultPrint mediaId={resultId} booth={booth} />}
            <button className="primary big" disabled={finishing} onClick={finish}>{booth ? t("finish") : t("newSession")}</button>
          </div>
        </div>
      )}

      {s.state === "ERROR" && (
        <div className="center-msg">
          <div className="note err">
            <div><strong>{booth ? "We couldn’t take that photo." : s.error}</strong>
              <div className="action">{booth ? "Your other photos are safe. Tap below to try again, or ask the host." : s.errorAction}</div></div>
          </div>
          <button className="primary big" onClick={() => { void sessionGate.reset(); }}>{t("tryAgain")}</button>
        </div>
      )}

      {s.state === "CANCELLED" && (
        <div className="center-msg"><p>{t("sessionCancelled")}</p><button className="primary big" onClick={() => { void sessionGate.reset(); }}>{t("startAgain")}</button></div>
      )}

      {dialogs}
    </div>
  );
}
