import { useEffect, useRef, useState } from "react";
import { api, toAppError } from "./api";
import { leaveFullscreen } from "./fullscreen";
import { exitImpact, safeExit, ExitWindowError } from "./safeExit";
import { sessionGate } from "./sessionGate";
import { useLocale } from "./locale";
import { translate } from "./i18n";
import { useApp } from "./state";
import { Modal } from "./ui";

/** "Exit Booth Mode and return to Studio?" — opened by the Exit button, Esc, or the corner long-press. */
export function ExitDialog({ onClose, onExited, leave = leaveFullscreen }: { onClose: () => void; onExited: () => void; leave?: () => Promise<void> }) {
  const { video, shoot, settings } = useApp();
  const language = useLocale(settings.language);
  const t = (key: Parameters<typeof translate>[1]) => translate(language, key);
  const [pin, setPin] = useState("");
  const [busy, setBusy] = useState(false);
  const [step, setStep] = useState("");
  const [error, setError] = useState("");
  const [windowStuck, setWindowStuck] = useState(false);

  // safeExit polls the *latest* state, not the value captured when it started
  const v = useRef(video); v.current = video;
  const s = useRef(shoot); s.current = shoot;

  const impact = exitImpact(video, shoot);
  const needPin = settings.kioskPinSet;
  const stay = useRef<HTMLButtonElement>(null);
  useEffect(() => { if (!needPin) stay.current?.focus(); }, [needPin]);

  const go = async () => {
    setError(""); setBusy(true);
    try {
      if (needPin) {
        try { await api.verifyKioskPin(pin); } catch (e) { setPin(""); throw e; }
      }
      await safeExit({
        getVideo: () => v.current, getShoot: () => s.current,
        stopRecording: () => api.stopRecording(),
        // the same coordinated reset every other way out uses, so Exit during a Finish cannot overlap it
        cancelShooting: async () => { if (!(await sessionGate.reset())) throw new Error("The photo session could not be closed. Try again in a moment."); },
        leaveFullscreen: leave, onStep: setStep,
      });
      onExited();
    } catch (e) {
      const err = toAppError(e);
      setError(err.action ? `${err.message} ${err.action}` : err.message);
      // the data was saved/cancelled safely; only the window change failed
      if (e instanceof ExitWindowError) setWindowStuck(true);
    } finally { setBusy(false); setStep(""); }
  };

  const label = impact.recording ? t("stopRecordExit") : t("exitBooth");
  return (
    <Modal title={t("exitTitle")} onClose={busy ? () => {} : onClose}>
      <div className="modal-body exit-dialog">
        {impact.recording && <div className="note warn"><div>{t("exitRecording")}</div></div>}
        {impact.session && !impact.recording && <div className="note warn"><div>{t("exitSession")}</div></div>}
        {impact.saving && <div className="note warn"><div>{t("exitSaving")}</div></div>}
        {needPin && (
          <label className="form-line">{t("operatorPin")}
            <input type="password" inputMode="numeric" autoComplete="off" autoFocus maxLength={8} value={pin} disabled={busy}
              onChange={(e) => setPin(e.target.value.replace(/\D/g, ""))}
              onKeyDown={(e) => { if (e.key === "Enter" && pin.length >= 4 && !busy) go(); }} aria-label={t("operatorPin")} />
          </label>
        )}
        {step && <p className="muted" role="status">{step}</p>}
        {error && <div className="note err" role="alert"><div>{error}</div></div>}
      </div>
      <footer>
        <button ref={stay} onClick={onClose} disabled={busy}>{t("stayBooth")}</button>
        {windowStuck && <button onClick={() => onExited()} disabled={busy}>{t("continueStudio")}</button>}
        <button className="primary" onClick={go} disabled={busy || (needPin && pin.length < 4)}>{busy ? t("working") : windowStuck ? t("tryAgain") : label}</button>
      </footer>
    </Modal>
  );
}
