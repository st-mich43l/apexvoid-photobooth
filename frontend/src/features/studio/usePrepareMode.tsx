import { useCallback, useRef, useState, type ReactNode } from "react";
import { api, toAppError } from "../../shared/api";
import { formatTranslation, translate } from "../../shared/i18n";
import { useLocale } from "../../shared/locale";
import { useApp } from "../../app/state";
import { LocalizedDiagnostic, Modal } from "../../shared/ui";
import type { CameraMode, ModeReadiness } from "../../shared/types";

/**
 * Makes the connected camera match the preference for Photo or Video mode.
 * If the preferred camera is missing it NEVER switches by itself: it shows
 * which camera it would use instead and waits for the operator to approve
 * (and to enter the kiosk PIN when one is set).
 */
export function usePrepareMode(preferredLanguage?: string): { prepare: (mode: CameraMode) => Promise<boolean>; dialog: ReactNode; error: string } {
  const { settings, toast } = useApp();
  const language = useLocale(preferredLanguage ?? settings.language);
  const t = (key: Parameters<typeof translate>[1]) => translate(language, key);
  const [ask, setAsk] = useState<ModeReadiness | null>(null);
  const [pin, setPin] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [errorIsLocalized, setErrorIsLocalized] = useState(false);
  const resolve = useRef<((ok: boolean) => void) | null>(null);
  const flight = useRef<Promise<boolean> | null>(null);

  const finish = (ok: boolean) => { resolve.current?.(ok); resolve.current = null; setAsk(null); setPin(""); setError(""); };

  // One preparation at a time: a second tap gets the first one's answer instead of racing it
  // (two overlapping camera switches, or a dialog whose promise is overwritten and never resolves).
  const prepare = useCallback((mode: CameraMode): Promise<boolean> => {
    if (flight.current) return flight.current;
    const run = prepareOnce(mode).finally(() => { flight.current = null; });
    flight.current = run;
    return run;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [toast, language]);

  const prepareOnce = async (mode: CameraMode): Promise<boolean> => {
    setError(""); setErrorIsLocalized(false);
    try {
      const r = await api.prepareMode(mode, "");
      if (r.ready) { if (r.switched) toast(t("cameraSwitchSuccess")); return true; }
      if (r.needsFallback && r.candidate) {
        setAsk(r);
        return await new Promise<boolean>((res) => { resolve.current = res; });
      }
      setError(r.message); return false;
    } catch (e) {
      const err = toAppError(e); setError(err.action ? `${err.message} ${err.action}` : err.message); return false;
    }
  };

  const approve = async () => {
    if (!ask?.candidate) return;
    setBusy(true); setError(""); setErrorIsLocalized(false);
    try {
      if (settings.kioskPinSet) await api.verifyKioskPin(pin);
      const r = await api.prepareMode(ask.mode, ask.candidate.id);
      if (!r.ready) { setError(r.message); return; }
      toast(t("cameraSwitchSuccess")); finish(true);
    } catch (e) {
      const err = toAppError(e);
      if (err.code?.startsWith("pin_")) setErrorIsLocalized(true);
      if (err.code === "pin_wrong") setError(t("pinWrong"));
      else if (err.code === "pin_locked") setError(formatTranslation(language, "pinLocked", { seconds: err.params?.seconds ?? 30 }));
      else if (err.code === "pin_unavailable") setError(t("pinUnavailable"));
      else setError(err.action ? `${err.message} ${err.action}` : err.message);
      setPin("");
    } finally { setBusy(false); }
  };

  const dialog = ask?.candidate ? (
    <Modal title={t("cameraFallbackTitle")} onClose={() => finish(false)}>
      <div className="modal-body">
        <p>{ask.preferred?.name ? formatTranslation(language, "cameraPreferredMissing", { name: ask.preferred.name }) : t("cameraFallbackReason")}</p>
        <p>{formatTranslation(language, "cameraFallbackQuestion", { name: ask.candidate.name, mode: t(ask.mode) })}</p>
        <p className="muted small">{t("cameraFallbackConfirm")}</p>
        {settings.kioskPinSet && (
          <label className="form-line">{t("operatorPin")}
            <input type="password" inputMode="numeric" autoComplete="off" maxLength={8} value={pin} aria-label={t("operatorPin")}
              onChange={(e) => setPin(e.target.value.replace(/\D/g, ""))} />
          </label>
        )}
        {error && <div className="note err" role="alert"><div>{language === "vi" && !errorIsLocalized ? <LocalizedDiagnostic text={error} summary="errorCamera" /> : error}</div></div>}
      </div>
      <footer>
        <button onClick={() => finish(false)} disabled={busy}>{t("cancel")}</button>
        <button className="primary" onClick={approve} disabled={busy || (settings.kioskPinSet && pin.length < 4)}>{formatTranslation(language, "useNamedCamera", { name: ask.candidate.name })}</button>
      </footer>
    </Modal>
  ) : null;

  return { prepare, dialog, error };
}
