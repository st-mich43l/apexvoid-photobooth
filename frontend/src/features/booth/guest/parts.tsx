import { formatTranslation, photoCount, translate } from "../../../shared/i18n";
import type { ButtonHTMLAttributes, ReactNode } from "react";
import { Icon } from "../../../shared/icons";
import { frameDisplayName, swatch } from "../../../shared/frameInfo";
import type { Frame } from "../../../shared/types";

/** Small, composable pieces of the guest (Booth) interface. Colours, sizes and motion live in guest.css as tokens. */

export function BrandMark({ compact }: { compact?: boolean }) {
  return (
    <span className={`g-brand ${compact ? "compact" : ""}`}>
      <span className="g-brand-mark" aria-hidden="true"><Icon name="camera" size={compact ? 18 : 26} /></span>
      <span className="g-brand-text"><b>ApexVoid</b> Photobooth</span>
    </span>
  );
}

export const STEPS = ["Frame", "Shoot", "Review", "Done"] as const;
export type Step = 1 | 2 | 3 | 4;

/** Where the guest is in the journey. Purely informational: never a button. */
export function StepIndicator({ step, language }: { step: Step; language?: string }) {
  const labels = (["frameStep", "shootStep", "reviewStep", "doneStep"] as const).map((key) => translate(language, key));
  return (
    <ol className="g-steps" aria-label={formatTranslation(language, "stepDescription", { step, total: STEPS.length, label: labels[step - 1] })}>
      {labels.map((label, i) => (
        <li key={label} className={i + 1 === step ? "now" : i + 1 < step ? "done" : ""} aria-current={i + 1 === step ? "step" : undefined}>
          <span className="g-step-dot">{i + 1 < step ? <Icon name="check" size={14} /> : i + 1}</span>
          <span className="g-step-label">{label}</span>
        </li>
      ))}
    </ol>
  );
}

/** Top bar of every guest screen after Welcome. The right-hand column stays empty: the operator's Exit button lives there. */
export function GuestHeader({ step, language }: { step: Step; language?: string }) {
  return (
    <header className="g-header">
      <BrandMark compact />
      <StepIndicator step={step} language={language} />
      <span aria-hidden="true" />
    </header>
  );
}

type BtnProps = ButtonHTMLAttributes<HTMLButtonElement> & { icon?: string; busy?: boolean };
/** THE action of a screen: one per screen, always bottom-right. */
export function PrimaryAction({ icon, busy, children, className = "", ...rest }: BtnProps) {
  return (
    <button type="button" {...rest} className={`g-btn g-primary ${busy ? "busy" : ""} ${className}`}>
      <span>{children}</span>{icon && <Icon name={icon} size={28} />}
    </button>
  );
}
export function SecondaryAction({ icon, children, className = "", ...rest }: BtnProps) {
  return (
    <button type="button" {...rest} className={`g-btn g-secondary ${className}`}>
      {icon && <Icon name={icon} size={24} />}<span>{children}</span>
    </button>
  );
}

/** A calm message with an optional action. `alert` is announced at once; `status` politely. */
export function GuestMessage({ kind = "info", children, action, hint, role }: { kind?: "info" | "warn" | "err"; children: ReactNode; action?: ReactNode; hint?: ReactNode; role?: "alert" | "status" }) {
  return (
    <div className={`g-message ${kind}`} role={role ?? (kind === "err" ? "alert" : "status")}>
      <div className="g-message-body"><div>{children}</div>{hint && <small className="host-hint">{hint}</small>}</div>
      {action}
    </div>
  );
}

export function LoadingState({ children }: { children: ReactNode }) {
  return <div className="g-center" role="status"><div className="g-spinner" aria-hidden="true" />{children}</div>;
}

/** "2×6 strip" → big "2×6" + small "strip". */
export function LayoutBadge({ format, language }: { format: string; language?: string }) {
  const [size, ...rest] = format.split(" ");
  return <span className="g-layout"><b>{size}</b>{rest.length > 0 && <span>{language === "vi" && rest.join(" ") === "strip" ? translate(language, "layoutStrip") : language === "vi" && rest.join(" ") === "sheet" ? translate(language, "layoutSheet") : rest.join(" ")}</span>}</span>;
}

/** One small rectangle per photo the frame takes. */
export function PhotoDots({ n }: { n: number }) {
  return <span className="g-photodots" aria-hidden="true">{Array.from({ length: Math.min(n, 8) }, (_, i) => <i key={i} />)}</span>;
}

/** The colour variants a frame has, as small dots (the first is the frame's own colour). */
export function ColorDots({ frame, max = 5, language }: { frame: Frame; max?: number; language?: string }) {
  const vs = frame.variants ?? [];
  if (vs.length < 2) return null;
  const shown = vs.slice(0, max);
  return (
    <span className="g-colordots" title={formatTranslation(language, "colorCount", { count: vs.length })}>
      {shown.map((v, i) => <i key={v.id} style={{ background: i === 0 ? frame.background : swatch(v, frame) }} />)}
      {vs.length > max && <em>+{vs.length - max}</em>}
    </span>
  );
}

export function FrameThumb({ frame, src, selected, mismatch, note, onSelect, language }:
  { frame: Frame; src: string; selected: boolean; mismatch?: boolean; note?: string; onSelect: () => void; language?: string }) {
  return (
    <button type="button" role="radio" aria-checked={selected} data-frame={frame.id} className={`g-frame ${selected ? "on" : ""} ${mismatch ? "mismatch" : ""}`} onClick={onSelect}>
      <span className="g-frame-img">
        <img src={src} alt={formatTranslation(language, "frameAlt", { name: frameDisplayName(frame, language) })} loading="lazy" decoding="async" draggable={false} />
        {selected && <span className="g-frame-check" aria-hidden="true"><Icon name="check" size={20} /></span>}
      </span>
      <span className="g-frame-name">{frameDisplayName(frame, language)}</span>
      <span className="g-frame-meta"><LayoutBadge format={frame.format} language={language} /><PhotoDots n={frame.photos} /><span>{photoCount(language, frame.photos)}</span></span>
      <span className="g-frame-foot"><ColorDots frame={frame} language={language} />{mismatch && note && <span className="g-frame-warn">{note}</span>}</span>
    </button>
  );
}

/** An image that fills the space it is given without ever being stretched: the picture is centred and contained. */
export function Fit({ src, alt, className = "" }: { src: string; alt: string; className?: string }) {
  return <div className={`g-fit ${className}`}><img src={src} alt={alt} decoding="async" draggable={false} /></div>;
}
