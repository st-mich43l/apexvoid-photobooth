import { formatTranslation, photoCount, translate } from "../../../shared/i18n";
import { frameDisplayName } from "../../../shared/frameInfo";
import { useMemo, useState, type ReactNode } from "react";
import { Icon } from "../../../shared/icons";
import type { CameraStatus, FrameChoice, ShootSnapshot } from "../../../shared/types";
import { LivePreview } from "../../../shared/ui";
import { guideStyles, previewFovFor, resolveCaptureGuide } from "../captureGeometry";
import type { PhotoSelection } from "../photoSelection";
import { selectGridShape } from "../photoSelection";
import { Fit, GuestMessage, LayoutBadge, LoadingState, PrimaryAction, SecondaryAction } from "./parts";
import { PhotoSelectGrid } from "./PhotoSelectGrid";

/** "Photo 2 of 4" with one dot per photo: taken, being taken now, still to come. */
export function PhotoSteps({ total, taken, current, language }: { total: number; taken: number; current: number | null; language?: string }) {
  const n = current != null ? current + 1 : Math.min(taken, total);
  const text = current != null ? formatTranslation(language, "photoOf", { current: n, total }) : formatTranslation(language, "photosOf", { current: Math.min(taken, total), total });
  return (
    <div className="steps" role="progressbar" aria-label={translate(language, "photoProgress")} aria-valuemin={0} aria-valuemax={total} aria-valuenow={n}>
      <ol>{Array.from({ length: total }, (_, i) => <li key={i} className={current === i ? "now" : i < taken ? "done" : ""} aria-hidden="true" />)}</ol>
      <span>{text}</span>
    </div>
  );
}

export interface GuestShootProps {
  s: ShootSnapshot; camera: CameraStatus; frame: FrameChoice; countdownSeconds?: number; language?: string;
  busy: boolean; error: boolean; onClearError: () => void; slow: boolean; plannedShots?: number;
  mediaUrl: (id: string, kind: "preview") => string; frameUrl: (id: string, variantId: string, width: number, caption?: string) => string;
  resultId?: string; finishing: boolean; print: ReactNode;
  /** The guest's photo choice for this session (shared model, see photoSelection.ts). */
  selection: PhotoSelection; mirrored?: boolean;
  onStart: () => void; onChangeFrame?: () => void; onContinue: () => void; onCancel: () => void;
  onChangeDesign: () => void; onFinish: () => void; onRetryPreview: () => void; onReset: () => void;
}

/** The frame the guest chose: a thumbnail with its name and photo count. */
function FrameChip({ frame, frameUrl, onChange, busy, language }: { frame: FrameChoice; frameUrl: GuestShootProps["frameUrl"]; onChange?: () => void; busy?: boolean; language?: string }) {
  return (
    <div className="g-chip-frame">
      <figure aria-label={formatTranslation(language, "frameNamed", { name: frameDisplayName(frame, language) })}>
        <img src={frameUrl(frame.id, frame.variantId, 240, frame.caption)} alt="" decoding="async" draggable={false} />
        <figcaption><b>{frameDisplayName(frame, language)}</b><LayoutBadge format={frame.format} language={language} /><small>{photoCount(language, frame.photos)}</small></figcaption>
      </figure>
      {onChange && <SecondaryAction icon="swap" className="g-small" disabled={busy} onClick={onChange}>{translate(language, "changeFrame")}</SecondaryAction>}
    </div>
  );
}

/** Everything a guest sees between choosing a frame and finishing. Presentational: the workflow stays in ShootFlow / the Go engine. */
export function GuestShoot(p: GuestShootProps) {
  const { s, camera, frame } = p;
  const t = (key: Parameters<typeof translate>[1]) => translate(p.language, key);
  const warningKey = s.warningCode === "retake_failed" ? "warningRetake" : s.warningCode === "camera_reconnecting" ? "warningReconnect" : s.warningCode === "composition_failed" ? "warningComposition" : "warningGeneric";
  const live = s.state === "PREVIEW" || s.state === "COUNTDOWN" || s.state === "CAPTURING";
  // The capture guide comes from the frame's photo windows and the camera alone. It is fixed before the first photo and
  // does not look at which photo is being taken, so every candidate is framed the same way.
  const [liveAr, setLiveAr] = useState(() => (camera.mode && camera.mode.width > 0 && camera.mode.height > 0 ? camera.mode.width / camera.mode.height : 3 / 2));
  const guide = useMemo(() => resolveCaptureGuide(frame, { liveAspect: liveAr, mirrored: !!p.mirrored, fov: previewFovFor(camera.device?.driver, camera.device?.name) }),
    [frame, liveAr, p.mirrored, camera.device?.driver, camera.device?.name]);
  const guideCss = guide ? guideStyles(guide) : null;
  const ordered = [...s.taken].sort((a, b) => a.slot - b.slot);
  const sel = p.selection;
  const cd = p.countdownSeconds;
  const shape = { l: selectGridShape(Math.max(1, ordered.length), false), p: selectGridShape(Math.max(1, ordered.length), true) };
  const grid = { ["--cl" as string]: shape.l.cols, ["--cp" as string]: shape.p.cols, ["--rl" as string]: shape.l.rows, ["--rp" as string]: shape.p.rows };

  return (
    <div className="g-shoot" data-phase={s.state}>
      {camera.reconnecting && (
        <div className="g-reconnect" role="status">
          <div className="g-spinner" aria-hidden="true" />
          <div><strong>{t("reconnecting")}</strong><span>{t("safePhotos")}{p.slow ? t("slowReconnect") : ""}</span></div>
        </div>
      )}
      {p.error && <GuestMessage kind="warn" role="alert" action={<button type="button" className="g-btn g-secondary g-small" onClick={p.onClearError}>{t("ok")}</button>}>{t("errorGuest")}</GuestMessage>}
      {!camera.device && !camera.reconnecting && <GuestMessage kind="warn">{t("cameraNotReady")}</GuestMessage>}

      {live && (
        <>
          <div className="g-stage-wrap">
            <div className="g-stage">
              {guide && guideCss ? (
                <div className="g-shot-target" style={guideCss.box} aria-label={t("framingGuide")} data-guide={guide.mode}>
                  <LivePreview className="g-live" mediaStyle={guideCss.media} onAspect={(a) => { if (s.state === "PREVIEW") setLiveAr(a); }} />
                </div>
              ) : <LivePreview className="g-live" onAspect={(a) => { if (s.state === "PREVIEW") setLiveAr(a); }} />}
              {s.state !== "PREVIEW" && <PhotoSteps language={p.language} total={s.shots} taken={s.taken.length} current={s.slot} />}
              {s.state === "COUNTDOWN" && s.remaining > 0 && (
                <div className="g-count" key={`${s.slot}-${s.remaining}`} aria-live="assertive"><span>{s.remaining}</span><small>{t("getReady")}</small></div>
              )}
              {s.state === "CAPTURING" && (
                <>
                  <div className="g-flash" key={`flash-${s.slot}`} aria-hidden="true" />
                  <div className="g-smile" aria-live="polite"><span>{t("smile")}</span></div>
                </>
              )}
            </div>
            <FrameChip language={p.language} frame={frame} frameUrl={p.frameUrl} onChange={s.state === "PREVIEW" ? p.onChangeFrame : undefined} busy={p.busy} />
          </div>
          <div className="g-actions">
            {s.state === "PREVIEW" && (
              <>
                <div className="g-prompt"><b>{t("getReady")}</b><span>{formatTranslation(p.language, "countPrompt", { photos: p.plannedShots ?? s.shots, count: frame.photos })}{cd ? formatTranslation(p.language, "countdownPrompt", { seconds: cd }) : ""}.</span></div>
                <PrimaryAction icon="camera" className="g-start" disabled={p.busy || !camera.liveView || camera.reconnecting} onClick={p.onStart}>
                  {t("startShooting")}<small>{photoCount(p.language, p.plannedShots ?? s.shots)}</small>
                </PrimaryAction>
              </>
            )}
            {s.state === "COUNTDOWN" && <SecondaryAction disabled={p.busy} onClick={p.onCancel}>{t("cancel")}</SecondaryAction>}
            {s.state === "CAPTURING" && <p className="g-prompt"><b>{t("holdStill")}</b></p>}
          </div>
        </>
      )}

      {s.state === "IDLE" && !p.error && <LoadingState>{t("cameraPreparing")}</LoadingState>}
      {s.state === "IDLE" && p.error && (
        <div className="g-center"><p>{t("cameraStartError")}</p><PrimaryAction onClick={p.onRetryPreview}>{t("tryAgain")}</PrimaryAction></div>
      )}

      {s.state === "REVIEW" && (
        <>
          <div className="g-review g-pick-screen">
            {s.warning && <GuestMessage kind="warn">{t(warningKey)}</GuestMessage>}
            <div className="g-pick-head">
              <div className="g-pick-title"><h2>{t("selectHeading")}</h2><p>{t("selectHint")}</p></div>
              <div className={`g-pick-count ${sel.complete ? "done" : ""}`} role="status" aria-live="polite">
                <strong aria-hidden="true">{sel.count}<small>/{sel.required}</small></strong>
                <div>
                  <b>{formatTranslation(p.language, "selectedCount", { selected: sel.count, required: sel.required })}</b>
                  <span>{sel.notice === "full" ? formatTranslation(p.language, "selectionFull", { count: sel.required }) : sel.complete ? formatTranslation(p.language, "selectionReady", { count: sel.required }) : sel.remaining === 1 ? t("chooseMoreOne") : formatTranslation(p.language, "chooseMoreMany", { count: sel.remaining })}</span>
                </div>
              </div>
              <div className="g-pick-frame">
                <figure aria-label={formatTranslation(p.language, "frameNamed", { name: frameDisplayName(frame, p.language) })}>
                  <img src={p.frameUrl(frame.id, frame.variantId, 120, frame.caption)} alt="" decoding="async" draggable={false} />
                  <figcaption><b>{frameDisplayName(frame, p.language)}</b><small>{photoCount(p.language, frame.photos)}</small></figcaption>
                </figure>
                <SecondaryAction icon="swap" className="g-small" disabled={p.busy} onClick={p.onChangeDesign}>{t("changeDesign")}</SecondaryAction>
              </div>
            </div>
            <PhotoSelectGrid candidates={ordered} selection={sel} mediaUrl={p.mediaUrl} language={p.language} disabled={p.busy || sel.submitting || camera.reconnecting} />
          </div>
          <div className="g-actions">
            <PrimaryAction icon="arrow" disabled={p.busy || sel.submitting || !sel.complete} onClick={p.onContinue}>{t("continueLabel")}</PrimaryAction>
          </div>
        </>
      )}

      {s.state === "SAVING" && <LoadingState>{t("saving")}</LoadingState>}

      {s.state === "COMPLETED" && (
        <>
          <div className="g-result">
            <div className="g-result-media">
              <div className="g-confetti" aria-hidden="true">{Array.from({ length: 12 }, (_, i) => <i key={i} style={{ ["--i" as string]: i }} />)}</div>
              {p.resultId
                ? <Fit className="g-paper" src={p.mediaUrl(p.resultId, "preview")} alt={t("photoStripAlt")} />
                : <div className="g-photos loose" style={grid}>{ordered.map((t) => <img key={t.mediaId} src={p.mediaUrl(t.mediaId, "preview")} alt="" />)}</div>}
            </div>
            <div className="g-result-side">
              <span className="g-done-badge" aria-hidden="true"><Icon name="check" size={34} /></span>
              <h2>{t("allDone")}</h2>
              <p>{t("photosReady")}</p>
              {s.warning && <GuestMessage kind="warn">{t(warningKey)}</GuestMessage>}
              <div className="g-result-tools">
                {p.print}
                <SecondaryAction icon="swap" disabled={p.busy || p.finishing} onClick={p.onChangeDesign}>{t("changeDesign")}</SecondaryAction>
              </div>
            </div>
          </div>
          <div className="g-actions">
            <PrimaryAction icon="check" disabled={p.finishing} onClick={p.onFinish}>{t("finish")}</PrimaryAction>
          </div>
        </>
      )}

      {s.state === "ERROR" && (
        <div className="g-center">
          <GuestMessage kind="err"><strong>{t("photoError")}</strong><div>{t("safeOtherPhotos")}</div></GuestMessage>
          <PrimaryAction onClick={p.onReset}>{t("tryAgain")}</PrimaryAction>
        </div>
      )}
      {s.state === "CANCELLED" && <div className="g-center"><p>{t("sessionCancelled")}</p><PrimaryAction onClick={p.onReset}>{t("startAgain")}</PrimaryAction></div>}
    </div>
  );
}
