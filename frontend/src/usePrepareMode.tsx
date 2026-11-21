import { useCallback, useRef, useState, type ReactNode } from "react";
import { api, toAppError } from "./api";
import { useApp } from "./state";
import { Modal } from "./ui";
import type { CameraMode, ModeReadiness } from "./types";

/**
 * Makes the connected camera match the preference for Photo or Video mode.
 * If the preferred camera is missing it NEVER switches by itself: it shows
 * which camera it would use instead and waits for the operator to approve
 * (and to enter the kiosk PIN when one is set).
 */
export function usePrepareMode(): { prepare: (mode: CameraMode) => Promise<boolean>; dialog: ReactNode; error: string } {
  const { settings, toast } = useApp();
  const [ask, setAsk] = useState<ModeReadiness | null>(null);
  const [pin, setPin] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const resolve = useRef<((ok: boolean) => void) | null>(null);

  const finish = (ok: boolean) => { resolve.current?.(ok); resolve.current = null; setAsk(null); setPin(""); setError(""); };

  const prepare = useCallback(async (mode: CameraMode): Promise<boolean> => {
    setError("");
    try {
      const r = await api.prepareMode(mode, "");
      if (r.ready) { if (r.switched) toast(r.message); return true; }
      if (r.needsFallback && r.candidate) {
        setAsk(r);
        return await new Promise<boolean>((res) => { resolve.current = res; });
      }
      setError(r.message); return false;
    } catch (e) {
      const err = toAppError(e); setError(err.action ? `${err.message} ${err.action}` : err.message); return false;
    }
  }, [toast]);

  const approve = async () => {
    if (!ask?.candidate) return;
    setBusy(true); setError("");
    try {
      if (settings.kioskPinSet) await api.verifyKioskPin(pin);
      const r = await api.prepareMode(ask.mode, ask.candidate.id);
      toast(r.message); finish(true);
    } catch (e) {
      const err = toAppError(e); setError(err.action ? `${err.message} ${err.action}` : err.message); setPin("");
    } finally { setBusy(false); }
  };

  const dialog = ask?.candidate ? (
    <Modal title="Use a different camera?" onClose={() => finish(false)}>
      <div className="modal-body">
        <p>{ask.message}</p>
        <p>Use <strong>{ask.candidate.name}</strong> for {ask.mode} mode instead?</p>
        <p className="muted small">Nothing is switched unless you confirm.</p>
        {settings.kioskPinSet && (
          <label className="form-line">Operator PIN
            <input type="password" inputMode="numeric" autoComplete="off" maxLength={8} value={pin} aria-label="Operator PIN"
              onChange={(e) => setPin(e.target.value.replace(/\D/g, ""))} />
          </label>
        )}
        {error && <div className="note err" role="alert"><div>{error}</div></div>}
      </div>
      <footer>
        <button onClick={() => finish(false)} disabled={busy}>Cancel</button>
        <button className="primary" onClick={approve} disabled={busy || (settings.kioskPinSet && pin.length < 4)}>Use {ask.candidate.name}</button>
      </footer>
    </Modal>
  ) : null;

  return { prepare, dialog, error };
}
