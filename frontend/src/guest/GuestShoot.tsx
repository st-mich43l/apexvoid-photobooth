import { formatTranslation, photoCount, translate } from "../i18n";
import { useState, type ReactNode } from "react";
import { photosLabel } from "../frameInfo";
import { Icon } from "../icons";
import type { CameraStatus, FrameChoice, ShootSnapshot } from "../types";
import { LivePreview } from "../ui";
import { Fit, GuestMessage, LayoutBadge, LoadingState, PrimaryAction, SecondaryAction } from "./parts";

/** "Photo 2 of 4" with one dot per photo: taken, being taken now, still to come. */
export function PhotoSteps({ total, taken, current, retaking, language }: { total: number; taken: number; current: number | null; retaking?: boolean; language?: string }) {
  const n = current != null ? current + 1 : Math.min(taken, total);
  const text = current != null ? (retaking ? formatTranslation(language, "retakingPhoto", { current: n }) : formatTranslation(language, "photoOf", { current: n, total })) : formatTranslation(language, "photosOf", { current: Math.min(taken, total), total });
  return (
    <div className="steps" role="progressbar" aria-label="Photo progress" aria-valuemin={0} aria-valuemax={total} aria-valuenow={n}>
      <ol>{Array.from({ length: total }, (_, i) => <li key={i} className={current === i ? "now" : i < taken ? "done" : ""} aria-hidden="true" />)}</ol>
      <span>{text}</span>
    </div>
  );
}

export interface GuestShootProps {
  s: ShootSnapshot; camera: CameraStatus; frame: FrameChoice; countdownSeconds?: number; language?: string;
  busy: boolean; error: boolean; onClearError: () => void; slow: boolean;
  mediaUrl: (id: string, kind: "preview") => string; frameUrl: (id: string, variantId: string, width: number, caption?: string) => string;
  resultId?: string; finishing: boolean; print: ReactNode;
  onStart: () => void; onChangeFrame?: () => void; onRetake: (slot: number) => void; onAccept: () => void; onCancel: () => void;
  onChangeDesign: () => void; onFinish: () => void; onRetryPreview: () => void; onReset: () => void;
}

/** One photo of the session with its number and its own Retake: it is obvious which photo a tap will replace.
 *  The tile takes the photo's real aspect ratio (read once it has loaded), so nothing is cropped or stretched. */
function PhotoTile({ slot, src, disabled, onRetake, language }: { slot: number; src: string; disabled: boolean; onRetake: (slot: number) => void; language?: string }) {
  const [ar, setAr] = useState(1.5);
  return (
    <figure className="g-photo">
      <div className="g-photo-cell">
        <div className="g-photo-img" style={{ ["--ar" as string]: ar }}>
          <img src={src} alt={`Photo ${slot + 1}`} decoding="async" draggable={false}
            onLoad={(e) => { const i = e.currentTarget; if (i.naturalWidth && i.naturalHeight) setAr(i.naturalWidth / i.naturalHeight); }} />
          <span className="g-photo-num" aria-hidden="true">{slot + 1}</span>
        </div>
      </div>
      <button type="button" className="g-btn g-secondary g-retake" aria-label={`${translate(language, "retakePhoto")} ${slot + 1}`} disabled={disabled} onClick={() => onRetake(slot)}>
        <Icon name="retake" size={22} /><span>{translate(language, "retake")}</span>
      </button>
    </figure>
  );
}

/** The frame the guest chose: a thumbnail with its name and photo count. */
function FrameChip({ frame, frameUrl, onChange, busy, language }: { frame: FrameChoice; frameUrl: GuestShootProps["frameUrl"]; onChange?: () => void; busy?: boolean; language?: string }) {
  return (
    <div className="g-chip-frame">
      <figure aria-label={`Frame: ${frame.name}`}>
        <img src={frameUrl(frame.id, frame.variantId, 240, frame.caption)} alt="" decoding="async" draggable={false} />
        <figcaption><b>{frame.name}</b><LayoutBadge format={frame.format} /><small>{photosLabel(frame.photos)}</small></figcaption>
      </figure>
      {onChange && <SecondaryAction icon="swap" className="g-small" disabled={busy} onClick={onChange}>{translate(language, "changeFrame")}</SecondaryAction>}
    </div>
  );
}

/** Everything a guest sees between choosing a frame and finishing. Presentational: the workflow stays in ShootFlow / the Go engine. */
export function GuestShoot(p: GuestShootProps) {
  const { s, camera, frame } = p;
  const t = (key: Parameters<typeof translate>[1]) => translate(p.language, key);
  const live = s.state === "PREVIEW" || s.state === "COUNTDOWN" || s.state === "CAPTURING";
  const retaking = (s.state === "COUNTDOWN" || s.state === "CAPTURING") && s.taken.some((t) => t.slot === s.slot);
  const ordered = [...s.taken].sort((a, b) => a.slot - b.slot);
  const cd = p.countdownSeconds;
  const photoN = Math.max(1, s.shots);
  const colsL = photoN <= 4 ? photoN : 3, colsP = photoN <= 2 ? 1 : 2;
  const grid = { ["--cl" as string]: colsL, ["--cp" as string]: colsP, ["--rl" as string]: Math.ceil(photoN / colsL), ["--rp" as string]: Math.ceil(photoN / colsP) };

  return (
    <div className="g-shoot" data-phase={s.state}>
      {camera.reconnecting && (
        <div className="g-reconnect" role="status">
          <div className="g-spinner" aria-hidden="true" />
          <div><strong>{t("reconnecting")}</strong><span>{t("safePhotos")}{p.slow ? t("slowReconnect") : ""}</span></div>
        </div>
      )}
      {p.error && <GuestMessage kind="warn" role="alert" action={<button type="button" className="g-btn g-secondary g-small" onClick={p.onClearError}>OK</button>}>{t("errorGuest")}</GuestMessage>}
      {!camera.device && !camera.reconnecting && <GuestMessage kind="warn">{t("cameraNotReady")}</GuestMessage>}

      {live && (
        <>
          <div className="g-stage-wrap">
            <div className="g-stage">
              <LivePreview className="g-live" />
              {s.state !== "PREVIEW" && <PhotoSteps language={p.language} total={s.shots} taken={s.taken.length} current={s.slot} retaking={retaking} />}
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
                <div className="g-prompt"><b>{t("getReady")}</b><span>{formatTranslation(p.language, "countPrompt", { photos: photoCount(p.language, frame.photos) })}{cd ? formatTranslation(p.language, "countdownPrompt", { seconds: cd }) : ""}.</span></div>
                <PrimaryAction icon="camera" className="g-start" disabled={p.busy || !camera.liveView || camera.reconnecting} onClick={p.onStart}>
                  {t("startShooting")}<small>{photoCount(p.language, frame.photos)}</small>
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
          <div className="g-review">
            {s.warning && <GuestMessage kind="warn">{s.warning}</GuestMessage>}
            <div className="g-review-head">
              <div><h2>{t("reviewHeading")}</h2><p>{t("reviewHint")}</p></div>
              <div className="g-review-frame">
                <FrameChip language={p.language} frame={frame} frameUrl={p.frameUrl} />
                <div className="g-review-tools">
                  <SecondaryAction icon="swap" className="g-small" disabled={p.busy} onClick={p.onChangeDesign}>{t("changeDesign")}</SecondaryAction>
                  <SecondaryAction icon="retake" className="g-small" disabled={p.busy} onClick={p.onCancel}>{t("startOver")}</SecondaryAction>
                </div>
              </div>
            </div>
            <div className="g-photos" style={grid}>
              {ordered.map((t) => (
                <PhotoTile language={p.language} key={t.mediaId} slot={t.slot} src={p.mediaUrl(t.mediaId, "preview")} disabled={p.busy || camera.reconnecting} onRetake={p.onRetake} />
              ))}
            </div>
          </div>
          <div className="g-actions">
            <PrimaryAction icon="check" disabled={p.busy} onClick={p.onAccept}>{t("looksGood")}</PrimaryAction>
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
                ? <Fit className="g-paper" src={p.mediaUrl(p.resultId, "preview")} alt="Your photo strip" />
                : <div className="g-photos loose" style={grid}>{ordered.map((t) => <img key={t.mediaId} src={p.mediaUrl(t.mediaId, "preview")} alt="" />)}</div>}
            </div>
            <div className="g-result-side">
              <span className="g-done-badge" aria-hidden="true"><Icon name="check" size={34} /></span>
              <h2>{t("allDone")}</h2>
              <p>{t("photosReady")}</p>
              {s.warning && <GuestMessage kind="warn">{s.warning}</GuestMessage>}
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
