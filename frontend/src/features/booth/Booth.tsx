import { translate } from "../../shared/i18n";
import { useCallback, useEffect, useRef, useState } from "react";
import { ExitDialog } from "./ExitDialog";
import { FrameSelection } from "../frames/FrameSelection";
import { enterFullscreen } from "../../shared/fullscreen";
import { GuestHeader, GuestMessage, type Step } from "./guest/parts";
import { Welcome } from "./guest/Welcome";
import { GuestLocale, type Language } from "../../shared/locale";
import { Icon } from "../../shared/icons";
import { LeaveDialog } from "./LeaveDialog";
import { sessionGate, useGate } from "./sessionGate";
import { ShootFlow } from "./ShootFlow";
import { useApp } from "../../app/state";
import { SimulatedRibbon } from "../../shared/ui";
import type { FrameChoice } from "../../shared/types";
import { useOrientation } from "../../shared/useOrientation";
import { usePrepareMode } from "../studio/usePrepareMode";

type Screen = "welcome" | "frames" | "photo";

/** Idle limits. They only ever apply where nothing is at stake (see `safeToReset`). */
export const IDLE_FRAMES_MS = 90_000;
export const IDLE_DONE_MS = 120_000;

/** Booth Mode — the fullscreen guest interface. Touch-first, large targets,
 *  portrait and landscape. The operator leaves through the visible "Exit Booth"
 *  button, the Esc key, or by holding the top-left corner for 3 s — all three
 *  open the same confirmation (and the PIN prompt when kiosk protection is on).
 *
 *  Session safety: a guest can never lose a photo session by accident.
 *  - COUNTDOWN / CAPTURING / SAVING / camera reconnection are never abandoned by a timer;
 *  - REVIEW is never discarded by a timer;
 *  - Home / Start over during COUNTDOWN or REVIEW asks first and says what happens to the photos;
 *  - Home is ignored while a photo is being taken or saved;
 *  - every way out goes through one coordinated reset (sessionGate), so repeated taps cannot overlap. */
export function Booth({ onExited }: { onExited: () => void }) {
  const { camera, shoot, readiness, settings } = useApp();
  const gate = useGate();
  const [guestLanguage, setGuestLanguage] = useState<Language>(() => settings.guestLanguage === "vi" ? "vi" : "en");
  const orientation = useOrientation();
  // #booth/photo deep-links into the frame choice (support, screenshots)
  const [screen, setScreen] = useState<Screen>(() => (window.location.hash === "#booth/photo" ? "frames" : "welcome"));
  const [asking, setAsking] = useState(false);
  const [fsError, setFsError] = useState("");
  const { prepare, dialog: modeDialog, error: modeError } = usePrepareMode(guestLanguage);
  // the frame is chosen first (no camera, no live view, no session yet). Booth Mode is photo-only; video recording is a separate flow (Studio)
  const [frame, setFrame] = useState<FrameChoice | null>(null);
  const [preparing, setPreparing] = useState(false);
  const [confirmLeave, setConfirmLeave] = useState(false);
  const [leaveError, setLeaveError] = useState("");
  const [leaving, setLeaving] = useState(false);
  const [touch, setTouch] = useState(0);

  const st = shoot.state;
  const stateRef = useRef(st);
  stateRef.current = st;
  const busy = useRef(false);     // one leave/finish at a time
  const starting = useRef(false); // one "choose frame → prepare camera" at a time

  const goWelcome = useCallback(() => { setFrame(null); setScreen("welcome"); setGuestLanguage(settings.guestLanguage === "vi" ? "vi" : "en"); }, [settings.guestLanguage]);

  /** Waits until no photo is being taken or saved (a shutter in flight must reach disk), then resets once. */
  const closeSession = useCallback(async (): Promise<boolean> => {
    for (let i = 0; i < 300 && (stateRef.current === "CAPTURING" || stateRef.current === "SAVING"); i++) await new Promise((r) => setTimeout(r, 50));
    return sessionGate.reset();
  }, []);

  /** Leave the photo screens for Welcome. `confirmed` skips nothing: COUNTDOWN/REVIEW must have been confirmed by the caller. */
  const leave = useCallback(async () => {
    if (busy.current) return;
    busy.current = true; setLeaving(true); setLeaveError("");
    try {
      if (screen === "photo") {
        if (!(await closeSession())) { setLeaveError(translate(guestLanguage, "sessionCloseFailed")); return; }
      }
      setConfirmLeave(false);
      goWelcome();
    } finally { busy.current = false; setLeaving(false); }
  }, [screen, closeSession, goWelcome, guestLanguage]);

  const requestHome = () => {
    if (busy.current) return;
    if (screen !== "photo") { goWelcome(); return; }
    if (st === "CAPTURING" || st === "SAVING") return; // never in the middle of a photo
    if (st === "COUNTDOWN" || st === "REVIEW") { setLeaveError(""); setConfirmLeave(true); return; }
    void leave();
  };

  /** Finish: show Welcome at once, tidy up behind it; the next session waits for that tidy-up. */
  const finishing = useRef(false);
  const finish = useCallback(() => {
    if (finishing.current) return;
    finishing.current = true;
    goWelcome();
    void sessionGate.reset().finally(() => { finishing.current = false; });
  }, [goWelcome]);

  const confirmFrame = async (c: FrameChoice) => {
    if (starting.current) return;
    starting.current = true; setPreparing(true);
    try {
      await sessionGate.ready(); // never start on top of a reset that is still running
      if (await prepare("photo")) { setFrame(c); setScreen("photo"); }
    } finally { starting.current = false; setPreparing(false); }
  };

  const goFullscreen = useCallback(() => {
    setFsError("");
    enterFullscreen().catch((e) => setFsError(e instanceof Error ? e.message : String(e)));
  }, []);
  useEffect(() => { goFullscreen(); }, [goFullscreen]);

  // Esc opens the exit confirmation (a dialog that is already open handles Esc itself)
  useEffect(() => {
    if (asking) return;
    const f = (e: KeyboardEvent) => {
      if (e.key !== "Escape" || document.querySelector(".modal")) return;
      e.preventDefault(); setAsking(true);
    };
    window.addEventListener("keydown", f);
    return () => window.removeEventListener("keydown", f);
  }, [asking]);

  // Inactivity — only where nothing is at stake:
  //   frame choice (no session exists yet) · live preview before the first shot · a session that is already saved.
  // Never during COUNTDOWN, CAPTURING, REVIEW, SAVING, a camera reconnection, or while a dialog is open.
  const idle = useRef<number | undefined>(undefined);
  useEffect(() => {
    window.clearTimeout(idle.current);
    if (asking || confirmLeave || leaving || preparing || camera.reconnecting) return;
    if (screen === "frames") idle.current = window.setTimeout(goWelcome, IDLE_FRAMES_MS);
    else if (screen === "photo" && (st === "IDLE" || st === "PREVIEW" || st === "CANCELLED" || st === "ERROR")) idle.current = window.setTimeout(() => { void leave(); }, IDLE_FRAMES_MS);
    else if (screen === "photo" && st === "COMPLETED") idle.current = window.setTimeout(finish, IDLE_DONE_MS);
    return () => window.clearTimeout(idle.current);
  }, [screen, st, asking, confirmLeave, leaving, preparing, camera.reconnecting, touch, goWelcome, leave, finish]);

  // secondary shortcut: hold the top-left corner for 3 s
  const hold = useRef<number | undefined>(undefined);
  const [holding, setHolding] = useState(false);
  const startHold = () => { setHolding(true); hold.current = window.setTimeout(() => { setHolding(false); setAsking(true); }, 3000); };
  const endHold = () => { setHolding(false); window.clearTimeout(hold.current); };
  useEffect(() => () => window.clearTimeout(hold.current), []);

  // A session that is still being closed (right after Finish) is not a setup problem: the guest-facing gate ignores it,
  // the gate in front of the next session (sessionGate) already waits for that reset.
  const hard = (readiness?.items ?? []).filter((i) => i.required && i.state === "problem");
  const onlyASession = hard.length > 0 && hard.every((i) => i.key === "session");
  const blocked = readiness ? !readiness.canStart && !onlyASession : !(camera.device && camera.state !== "error" && !camera.reconnecting);
  const guestText = translate(guestLanguage, readiness?.status === "reconnecting" ? "guestReconnecting" : "guestNotReady");
  const resetting = gate.phase === "resetting";
  // where the guest is in Frame → Shoot → Review → Done (informational only)
  const step: Step = screen === "frames" ? 1 : st === "REVIEW" || st === "SAVING" ? 3 : st === "COMPLETED" ? 4 : 2;

  return (
    <GuestLocale language={guestLanguage} operatorLanguage={settings.language}><div className="booth-root" data-orientation={orientation} data-screen={screen} data-state={screen === "photo" ? st : undefined} onPointerDown={() => setTouch((n) => n + 1)}>
      <SimulatedRibbon />
      <div className={`exit-corner ${holding ? "holding" : ""}`} data-testid="exit-corner" onPointerDown={startHold} onPointerUp={endHold} onPointerLeave={endHold} onPointerCancel={endHold} aria-hidden />
      <button className="exit-btn" onClick={() => setAsking(true)} aria-label={translate(guestLanguage, "exitBoothMode")} title={translate(guestLanguage, "exitBoothHint")}>
        <Icon name="exit" size={18} /> {translate(guestLanguage, "exitBooth")}
      </button>
      {fsError && (
        <div className="note warn fs-note" role="alert">
          <div>{guestLanguage === "en" ? `Fullscreen is not active: ${fsError} Booth Mode keeps working in a window.` : translate(guestLanguage, "fullscreenInactive")}</div>
          <button onClick={goFullscreen}>{translate(guestLanguage, "retry")}</button>
        </div>
      )}
      {screen !== "welcome" && <GuestHeader step={step} language={guestLanguage} />}
      {screen === "welcome" && (
        <Welcome onLanguageChange={setGuestLanguage} language={guestLanguage} blocked={blocked} resetting={resetting} guestText={guestText} readiness={readiness} resetFailed={gate.phase === "failed"} resetError={gate.error}
          modeError={modeError ? translate(guestLanguage, "cameraNotReady") : ""} onStart={() => setScreen("frames")} onRetryReset={() => { void sessionGate.reset(); }} />
      )}
      {screen === "frames" && (
        <>
          {modeError && <GuestMessage kind="err">{translate(guestLanguage, "cameraNotReady")}</GuestMessage>}
          <FrameSelection variant="booth" busy={preparing} initial={frame ? { id: frame.id, variantId: frame.variantId, caption: frame.caption } : undefined}
            onConfirm={confirmFrame} onBack={goWelcome} backLabel={translate(guestLanguage, "home")} />
        </>
      )}
      {screen === "photo" && frame && (
        <ShootFlow variant="booth" frame={frame} onChangeFrame={() => { void (async () => { if (busy.current) return; busy.current = true; try { if (await sessionGate.reset()) setScreen("frames"); } finally { busy.current = false; } })(); }}
          onDone={finish} />
      )}
      {screen === "photo" && (
        <button type="button" className="home-btn g-btn g-secondary" disabled={leaving || st === "CAPTURING" || st === "SAVING"} onClick={requestHome}><Icon name="back" size={24} /><span>{translate(guestLanguage, "home")}</span></button>
      )}
      {modeDialog}
      {confirmLeave && (
        <LeaveDialog language={guestLanguage} state={st} taken={shoot.taken.length} busy={leaving} error={leaveError} onKeep={() => setConfirmLeave(false)} onLeave={() => { void leave(); }} />
      )}
      {asking && <ExitDialog onClose={() => setAsking(false)} onExited={onExited} />}
    </div></GuestLocale>
  );
}
