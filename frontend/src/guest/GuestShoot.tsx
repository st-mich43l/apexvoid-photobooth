import { useState, type ReactNode } from "react";
import { photosLabel } from "../frameInfo";
import { Icon } from "../icons";
import type { CameraStatus, FrameChoice, ShootSnapshot } from "../types";
import { LivePreview } from "../ui";
import { Fit, GuestMessage, LayoutBadge, LoadingState, PrimaryAction, SecondaryAction } from "./parts";

/** "Photo 2 of 4" with one dot per photo: taken, being taken now, still to come. */
export function PhotoSteps({ total, taken, current, retaking }: { total: number; taken: number; current: number | null; retaking?: boolean }) {
  const n = current != null ? current + 1 : Math.min(taken, total);
  const text = current != null ? (retaking ? `Retaking photo ${n}` : `Photo ${n} of ${total}`) : `${Math.min(taken, total)} of ${total} photos`;
  return (
    <div className="steps" role="progressbar" aria-label="Photo progress" aria-valuemin={0} aria-valuemax={total} aria-valuenow={n}>
      <ol>{Array.from({ length: total }, (_, i) => <li key={i} className={current === i ? "now" : i < taken ? "done" : ""} aria-hidden="true" />)}</ol>
      <span>{text}</span>
    </div>
  );
}

export interface GuestShootProps {
  s: ShootSnapshot; camera: CameraStatus; frame: FrameChoice; countdownSeconds?: number;
  busy: boolean; error: boolean; onClearError: () => void; slow: boolean;
  mediaUrl: (id: string, kind: "preview") => string; frameUrl: (id: string, variantId: string, width: number, caption?: string) => string;
  resultId?: string; finishing: boolean; print: ReactNode;
  onStart: () => void; onChangeFrame?: () => void; onRetake: (slot: number) => void; onAccept: () => void; onCancel: () => void;
  onChangeDesign: () => void; onFinish: () => void; onRetryPreview: () => void; onReset: () => void;
}

/** One photo of the session with its number and its own Retake: it is obvious which photo a tap will replace.
 *  The tile takes the photo's real aspect ratio (read once it has loaded), so nothing is cropped or stretched. */
function PhotoTile({ slot, src, disabled, onRetake }: { slot: number; src: string; disabled: boolean; onRetake: (slot: number) => void }) {
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
      <button type="button" className="g-btn g-secondary g-retake" aria-label={`Retake photo ${slot + 1}`} disabled={disabled} onClick={() => onRetake(slot)}>
        <Icon name="retake" size={22} /><span>Retake</span>
      </button>
    </figure>
  );
}

/** The frame the guest chose: a thumbnail with its name and photo count. */
function FrameChip({ frame, frameUrl, onChange, busy }: { frame: FrameChoice; frameUrl: GuestShootProps["frameUrl"]; onChange?: () => void; busy?: boolean }) {
  return (
    <div className="g-chip-frame">
      <figure aria-label={`Frame: ${frame.name}`}>
        <img src={frameUrl(frame.id, frame.variantId, 240, frame.caption)} alt="" decoding="async" draggable={false} />
        <figcaption><b>{frame.name}</b><LayoutBadge format={frame.format} /><small>{photosLabel(frame.photos)}</small></figcaption>
      </figure>
      {onChange && <SecondaryAction icon="swap" className="g-small" disabled={busy} onClick={onChange}>Change frame</SecondaryAction>}
    </div>
  );
}

/** Everything a guest sees between choosing a frame and finishing. Presentational: the workflow stays in ShootFlow / the Go engine. */
export function GuestShoot(p: GuestShootProps) {
  const { s, camera, frame } = p;
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
          <div><strong>Reconnecting the camera…</strong><span>Your photos are safe. This usually takes a few seconds.{p.slow ? " Still working — please ask the host if this takes much longer." : ""}</span></div>
        </div>
      )}
      {p.error && <GuestMessage kind="warn" role="alert" action={<button type="button" className="g-btn g-secondary g-small" onClick={p.onClearError}>OK</button>}>That didn’t work. Please try again, or ask the host.</GuestMessage>}
      {!camera.device && !camera.reconnecting && <GuestMessage kind="warn">The camera is not ready. Please ask the host.</GuestMessage>}

      {live && (
        <>
          <div className="g-stage-wrap">
            <div className="g-stage">
              <LivePreview className="g-live" />
              {s.state !== "PREVIEW" && <PhotoSteps total={s.shots} taken={s.taken.length} current={s.slot} retaking={retaking} />}
              {s.state === "COUNTDOWN" && s.remaining > 0 && (
                <div className="g-count" key={`${s.slot}-${s.remaining}`} aria-live="assertive"><span>{s.remaining}</span><small>Get ready!</small></div>
              )}
              {s.state === "CAPTURING" && (
                <>
                  <div className="g-flash" key={`flash-${s.slot}`} aria-hidden="true" />
                  <div className="g-smile" aria-live="polite"><span>Smile!</span></div>
                </>
              )}
            </div>
            <FrameChip frame={frame} frameUrl={p.frameUrl} onChange={s.state === "PREVIEW" ? p.onChangeFrame : undefined} busy={p.busy} />
          </div>
          <div className="g-actions">
            {s.state === "PREVIEW" && (
              <>
                <div className="g-prompt"><b>Get ready!</b><span>We’ll take {photosLabel(frame.photos)}{cd ? `, with a ${cd}-second countdown before each` : ""}.</span></div>
                <PrimaryAction icon="camera" className="g-start" disabled={p.busy || !camera.liveView || camera.reconnecting} onClick={p.onStart}>
                  Start shooting<small>{photosLabel(frame.photos)}</small>
                </PrimaryAction>
              </>
            )}
            {s.state === "COUNTDOWN" && <SecondaryAction disabled={p.busy} onClick={p.onCancel}>Cancel</SecondaryAction>}
            {s.state === "CAPTURING" && <p className="g-prompt"><b>Hold still…</b></p>}
          </div>
        </>
      )}

      {s.state === "IDLE" && !p.error && <LoadingState>Getting the camera ready…</LoadingState>}
      {s.state === "IDLE" && p.error && (
        <div className="g-center"><p>We couldn’t start the camera.</p><PrimaryAction onClick={p.onRetryPreview}>Try again</PrimaryAction></div>
      )}

      {s.state === "REVIEW" && (
        <>
          <div className="g-review">
            {s.warning && <GuestMessage kind="warn">{s.warning}</GuestMessage>}
            <div className="g-review-head">
              <div><h2>How do they look?</h2><p>Not happy with one? Tap Retake under that photo — the others stay.</p></div>
              <div className="g-review-frame">
                <FrameChip frame={frame} frameUrl={p.frameUrl} />
                <div className="g-review-tools">
                  <SecondaryAction icon="swap" className="g-small" disabled={p.busy} onClick={p.onChangeDesign}>Change design</SecondaryAction>
                  <SecondaryAction icon="retake" className="g-small" disabled={p.busy} onClick={p.onCancel}>Start over</SecondaryAction>
                </div>
              </div>
            </div>
            <div className="g-photos" style={grid}>
              {ordered.map((t) => (
                <PhotoTile key={t.mediaId} slot={t.slot} src={p.mediaUrl(t.mediaId, "preview")} disabled={p.busy || camera.reconnecting} onRetake={p.onRetake} />
              ))}
            </div>
          </div>
          <div className="g-actions">
            <PrimaryAction icon="check" disabled={p.busy} onClick={p.onAccept}>Looks good</PrimaryAction>
          </div>
        </>
      )}

      {s.state === "SAVING" && <LoadingState>Saving your photos…</LoadingState>}

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
              <h2>All done!</h2>
              <p>Your photos are ready.</p>
              {s.warning && <GuestMessage kind="warn">{s.warning}</GuestMessage>}
              <div className="g-result-tools">
                {p.print}
                <SecondaryAction icon="swap" disabled={p.busy || p.finishing} onClick={p.onChangeDesign}>Change design</SecondaryAction>
              </div>
            </div>
          </div>
          <div className="g-actions">
            <PrimaryAction icon="check" disabled={p.finishing} onClick={p.onFinish}>Finish</PrimaryAction>
          </div>
        </>
      )}

      {s.state === "ERROR" && (
        <div className="g-center">
          <GuestMessage kind="err"><strong>We couldn’t take that photo.</strong><div>Your other photos are safe. Tap below to try again, or ask the host.</div></GuestMessage>
          <PrimaryAction onClick={p.onReset}>Try again</PrimaryAction>
        </div>
      )}
      {s.state === "CANCELLED" && <div className="g-center"><p>Session cancelled.</p><PrimaryAction onClick={p.onReset}>Start again</PrimaryAction></div>}
    </div>
  );
}
