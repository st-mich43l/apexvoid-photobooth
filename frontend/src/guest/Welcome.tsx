import type { BoothReadiness } from "../types";
import { BrandMark, GuestMessage, PrimaryAction } from "./parts";

/** Decorative photo strips. Purely visual (aria-hidden, no pointer events), animated with transforms only. */
function WelcomeArt() {
  return (
    <div className="g-art" aria-hidden="true">
      <span className="g-orb a" /><span className="g-orb b" />
      <span className="g-strip s1"><i /><i /><i /><i /></span>
      <span className="g-strip s2"><i /><i /><i /><i /></span>
      <span className="g-strip s3"><i /><i /><i /><i /></span>
      <span className="g-spark k1" /><span className="g-spark k2" /><span className="g-spark k3" />
    </div>
  );
}

export interface WelcomeProps {
  blocked: boolean; resetting: boolean; guestText: string; readiness: BoothReadiness | null;
  resetFailed: boolean; resetError: string; modeError?: string;
  onStart: () => void; onRetryReset: () => void;
}

/** The first thing a guest sees. One big action; any reason it cannot start is stated plainly, never hidden by the artwork. */
export function Welcome(p: WelcomeProps) {
  const sub = p.blocked ? "We will be right with you" : "Pick a frame, strike a pose, and get your photo strip.";
  return (
    <div className="g-welcome">
      <WelcomeArt />
      <div className="g-welcome-card">
        <div className="g-rise r1"><BrandMark /></div>
        <h1 className="g-rise r2">Say cheese!</h1>
        <p className="g-sub g-rise r3">{sub}</p>
        {p.blocked && (
          <GuestMessage kind="warn" hint={p.readiness ? `Host: ${p.readiness.status === "attention" ? "Operator attention required" : p.readiness.headline}` : undefined}>
            {p.guestText}
          </GuestMessage>
        )}
        {p.resetFailed && !p.resetting && (
          <GuestMessage kind="warn" role="alert" hint={`Host: ${p.resetError}`} action={<button type="button" className="g-btn g-secondary" onClick={p.onRetryReset}>Try again</button>}>
            We are still tidying up the last session.
          </GuestMessage>
        )}
        {p.modeError && <GuestMessage kind="err">{p.modeError}</GuestMessage>}
        <div className="g-rise r4">
          <PrimaryAction icon="camera" className="g-start" disabled={p.blocked || p.resetting} onClick={p.onStart}>
            {p.resetting ? "One moment…" : "Take photos"}
          </PrimaryAction>
        </div>
        <p className="g-hint g-rise r5">{p.resetting ? "Getting ready for you" : "Choose a frame to begin"}</p>
      </div>
    </div>
  );
}
